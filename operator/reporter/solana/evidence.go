package solana

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"

	"github.com/gagliardetto/solana-go"
	"golang.org/x/sync/singleflight"

	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

// entryOverhead approximates the memory an index entry takes besides its owner bytes and signature text.
const entryOverhead = 192

var (
	// ErrBlockMissing reports a resolution block that the owned node cannot serve.
	ErrBlockMissing = errors.New("resolution evidence unavailable: block missing from owned node history")
	// ErrNotInBlock reports that no transaction of the resolution block proves the resolution.
	ErrNotInBlock = errors.New("resolution evidence unavailable in its block")
	// ErrMultipleInBlock reports more than one transaction proving the resolution; the status account is created once.
	ErrMultipleInBlock = errors.New("multiple resolution transactions in block")

	errSlotMismatch = errors.New("resolution slot mismatch")
	errSignature    = errors.New("resolution transaction signature is not a non-zero 64 byte signature")
)

// writes reports whether tx lists address as a writable account.
func writes(tx Tx, address solana.PublicKey) bool {
	for i, k := range tx.Keys {
		if k == address && i < len(tx.Writable) && tx.Writable[i] {
			return true
		}
	}

	return false
}

// Resolves reports whether tx proves a resolution: it succeeded, has logs, writes address and one of the
// TransferResolved events of its program frames satisfies match. Logs that cannot be attributed give no evidence.
func Resolves(tx Tx, program, address solana.PublicKey, match func(TransferResolved) bool) bool {
	if tx.Failed || !tx.HasLogs || !writes(tx, address) {
		return false
	}

	events, err := ProgramEvents(tx.Logs, program)
	if err != nil {
		return false
	}

	return slices.ContainsFunc(events, match)
}

// MatchesStatus reports whether ev carries the identity, proof height, slot and outcome of st.
func MatchesStatus(ev TransferResolved, st TransferStatus) bool {
	if !bytes.Equal(ev.ID.Owner, st.ID.Owner) ||
		ev.ID.Height != st.ID.Height ||
		ev.ID.Sequence != st.ID.Sequence ||
		ev.ProofHeight != st.ProofHeight ||
		ev.ResolvedAtSlot != st.ResolvedAtSlot {
		return false
	}

	switch {
	case ev.Outcome.Minted != nil && st.Outcome.Minted != nil:
		return *ev.Outcome.Minted == *st.Outcome.Minted
	case ev.Outcome.Voided != nil && st.Outcome.Voided != nil:
		return *ev.Outcome.Voided == *st.Outcome.Voided
	default:
		return false
	}
}

// entry is one candidate event of a successful transaction of a block.
type entry struct {
	event     TransferResolved
	signature string
	tx        int
	status    solana.PublicKey
	writes    bool
}

func (e entry) size() int64 {
	return int64(entryOverhead + len(e.event.ID.Owner) + len(e.signature))
}

type slotEntries struct {
	entries []entry
	size    int64
}

// EvidenceIndex finds the transaction that resolved a transfer in the block of its resolution slot. Blocks are
// finalized, so the entries of a slot never change; slots are evicted lowest number first, not by use.
type EvidenceIndex struct {
	rpc      *RPC
	program  solana.PublicKey
	maxSlots int
	maxBytes int64

	fetches singleflight.Group

	mu    sync.Mutex
	slots map[uint64]slotEntries
	size  int64
}

// NewEvidenceIndex returns an index over the blocks rpc serves. It keeps at most maxSlots slots and about
// maxBytes of entries.
func NewEvidenceIndex(rpc *RPC, program solana.PublicKey, maxSlots int, maxBytes int64) *EvidenceIndex {
	return &EvidenceIndex{
		rpc:      rpc,
		program:  program,
		maxSlots: maxSlots,
		maxBytes: maxBytes,
		slots:    make(map[uint64]slotEntries),
	}
}

// Lookup returns the signature of the one transaction of slot that resolves st at address.
//
// ErrBlockMissing means the block could not be read; ErrNotInBlock and ErrMultipleInBlock mean the block holds no
// resolution or several. A cancelled ctx gives ctx.Err() and caches nothing.
func (e *EvidenceIndex) Lookup(
	ctx context.Context,
	slot uint64,
	address solana.PublicKey,
	st TransferStatus,
) (solana.Signature, error) {
	entries, err := e.entries(ctx, slot)
	if err != nil {
		return solana.Signature{}, err
	}

	if st.ResolvedAtSlot != slot {
		return solana.Signature{}, fmt.Errorf("%w: status resolved at %d, looked up %d", errSlotMismatch, st.ResolvedAtSlot, slot)
	}

	var (
		found     int
		last      = -1
		signature string
	)

	for _, en := range entries {
		if !en.writes || en.status != address || !MatchesStatus(en.event, st) || en.tx == last {
			continue
		}

		last = en.tx
		signature = en.signature
		found++
	}

	switch {
	case found > 1:
		return solana.Signature{}, ErrMultipleInBlock
	case found == 0:
		return solana.Signature{}, ErrNotInBlock
	}

	sig, err := solana.SignatureFromBase58(signature)
	if err != nil || sig == (solana.Signature{}) {
		return solana.Signature{}, errSignature
	}

	return sig, nil
}

// entries returns the entries of slot, fetching the block unless it is cached. Concurrent callers share one fetch.
func (e *EvidenceIndex) entries(ctx context.Context, slot uint64) ([]entry, error) {
	key := strconv.FormatUint(slot, 10)

	for {
		if entries, ok := e.cached(slot); ok {
			return entries, nil
		}

		res := e.fetches.DoChan(key, func() (any, error) { return e.fetch(ctx, slot) })

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case r := <-res:
			if r.Err == nil {
				return r.Val.([]entry), nil //nolint:forcetypeassert // fetch returns []entry
			}

			// The fetch ran under another caller's context; that caller giving up is not this caller's error.
			if ctx.Err() == nil && (errors.Is(r.Err, context.Canceled) || errors.Is(r.Err, context.DeadlineExceeded)) {
				continue
			}

			return nil, r.Err
		}
	}
}

func (e *EvidenceIndex) cached(slot uint64) ([]entry, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, ok := e.slots[slot]

	return s.entries, ok
}

// fetch streams the block of slot into a local slice and publishes it only when the whole block was read.
func (e *EvidenceIndex) fetch(ctx context.Context, slot uint64) ([]entry, error) {
	// A flight that finished after the caller looked at the cache has already published the slot.
	if entries, ok := e.cached(slot); ok {
		return entries, nil
	}

	var (
		entries []entry
		txs     int
	)

	err := e.rpc.StreamBlock(ctx, slot, func(tx Tx) error {
		entries = e.appendEntries(entries, tx, txs)
		txs++

		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}

		return nil, &causeError{sentinel: ErrBlockMissing, cause: err}
	}

	e.publish(slot, entries)

	return entries, nil
}

// appendEntries adds an entry for every TransferResolved event of a transaction that could be evidence. A transaction
// whose logs cannot be attributed has no entries.
func (e *EvidenceIndex) appendEntries(dst []entry, tx Tx, index int) []entry {
	if tx.Failed || !tx.HasLogs || !slices.Contains(tx.Writable, true) {
		return dst
	}

	events, err := ProgramEvents(tx.Logs, e.program)
	if err != nil {
		return dst
	}

	var signature string
	if len(tx.Signatures) > 0 {
		signature = tx.Signatures[0]
	}

	for _, ev := range events {
		id := sinkid.ID{
			OwnerBytes: ev.ID.Owner,
			Height:     int64(ev.ID.Height), //nolint:gosec // the seed uses the same 8 bytes
			Sequence:   ev.ID.Sequence,
		}
		status := StatusAddress(e.program, id)

		dst = append(dst, entry{
			event:     ev,
			signature: signature,
			tx:        index,
			status:    status,
			writes:    writes(tx, status),
		})
	}

	return dst
}

// publish stores the entries of slot, then evicts the numerically lowest slots until the budgets hold.
func (e *EvidenceIndex) publish(slot uint64, entries []entry) {
	var size int64
	for _, en := range entries {
		size += en.size()
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if old, ok := e.slots[slot]; ok {
		e.size -= old.size
	}

	e.slots[slot] = slotEntries{entries: entries, size: size}
	e.size += size

	for len(e.slots) > 0 && (len(e.slots) > e.maxSlots || e.size > e.maxBytes) {
		lowest, first := uint64(0), true

		for s := range e.slots {
			if first || s < lowest {
				lowest, first = s, false
			}
		}

		e.size -= e.slots[lowest].size
		delete(e.slots, lowest)
	}
}
