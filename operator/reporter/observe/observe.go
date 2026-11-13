// Package observe ports SolanaObserver of contracts/solana/reporter/solana.ts: it turns finalized Solana state into
// the observation the policy plans from. The owned RPC node is a trust boundary; finalized commitment cannot make an
// untrusted node truthful.
package observe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/internal/clock"
	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

const (
	// futureSlack is how far a finalized block time may lie ahead of the local clock.
	futureSlack   = 10 * time.Second
	launchVersion = 2
	mintDecimals  = 6
	signatureSize = 64
)

// Error texts are the TS texts; checks TS asserted without a message have descriptive texts.
var (
	errCluster        = errors.New("wrong Solana cluster")
	errNoSlot         = errors.New("Solana node has no finalized positive slot") //nolint:staticcheck // verbatim TS text
	errStale          = errors.New("stale Solana finalized state")
	errNotExecutable  = errors.New("mint program is not executable")
	errConfigOwner    = errors.New("wrong configuration owner")
	errLaunchVersion  = errors.New("unsupported launch configuration")
	errConfiguredMint = errors.New("wrong configured mint")
	errOriginChain    = errors.New("wrong bridge origin chain")
	errMintMissing    = errors.New("AKT mint account is missing")
	errMintOwner      = errors.New("AKT mint is not owned by the SPL Token program")
	errMintConfig     = errors.New("wrong AKT mint configuration")
	errMintAuthority  = errors.New("wrong mint authority")
	errNotInChunk     = errors.New("transfer status is not in the chunk snapshot")
	errStatusOwner    = errors.New("wrong transfer status owner")
	errStatusID       = errors.New("wrong status transfer ID")
	errProofHeight    = errors.New("proof predates deposit commitment")
	errNotFinalized   = errors.New("status is not finalized")
	errMintAmount     = errors.New("mint amount differs from Akash deposit")
	errMintRecipient  = errors.New("mint recipient differs from Akash deposit")
	errUnknownOutcome = errors.New("unknown transfer outcome")
	errBehind         = errors.New("Solana node is behind reported evidence; absence cannot authorize a veto") //nolint:staticcheck // verbatim TS text
	errContradicts    = errors.New("reported Solana transaction contradicts missing status")
	errSlotMismatch   = errors.New("resolution slot mismatch")
)

// Options configures an Observer.
type Options struct {
	GenesisHash string
	ProgramID   solana.PublicKey
	Mint        solana.PublicKey
	// LightClientPin, when set, fails every snapshot whose bridge names another light client. Unset follows the bridge.
	LightClientPin *solana.PublicKey
	ChainID        string
	MaxAge         time.Duration
	// SlotLead is how far beyond this node's finalized slot another node's finalized slot can plausibly be.
	SlotLead uint64
	Clock    clock.Clock
}

// Change is a change of the followed light client, seen first at ContextSlot.
type Change struct {
	Previous    solana.PublicKey
	Current     solana.PublicKey
	ContextSlot uint64
}

// MismatchError reports a bridge account that names a light client other than the configured pin.
type MismatchError struct {
	Pinned  solana.PublicKey
	Current solana.PublicKey
}

func (e *MismatchError) Error() string {
	return fmt.Sprintf("bridge light client %s differs from pinned %s", e.Current, e.Pinned)
}

// Deployment is the result of a deployment snapshot without status accounts.
type Deployment struct {
	ContextSlot   uint64
	BridgeEnabled bool
	LightClient   solana.PublicKey
}

// Chunk is one finalized snapshot of the deployment accounts and the status accounts of a chunk of transfers.
type Chunk struct {
	contextSlot uint64
	statuses    map[solana.PublicKey]*aktsolana.Account
}

// Finalized is the finalized status of one transfer. Address, Status, Outcome and Slot are set for KindResolved only.
type Finalized struct {
	Kind        policy.Kind
	ContextSlot uint64
	Address     solana.PublicKey
	Status      *aktsolana.TransferStatus
	Outcome     sinkv1.Outcome
	Slot        uint64
}

// Observer reads finalized Solana state for the reporter.
type Observer struct {
	rpc       *aktsolana.RPC
	index     *aktsolana.EvidenceIndex
	opts      Options
	onChange  func(Change)
	config    solana.PublicKey
	bridge    solana.PublicKey
	authority solana.PublicKey

	// follow guards the followed light client and the highest snapshot slot it was seen at; onChange runs under it.
	follow    sync.Mutex
	following bool
	followed  solana.PublicKey
	highest   uint64
}

// New returns an observer. onChange, which may be nil, is called with each change of the followed light client
// while the follow lock is held. New panics on a mint that is not the program's "mint" PDA or a zero slot lead,
// which configuration validation rules out.
func New(rpc *aktsolana.RPC, idx *aktsolana.EvidenceIndex, o Options, onChange func(Change)) *Observer {
	if o.SlotLead == 0 {
		panic("observe: invalid slot lead")
	}
	if o.Mint != aktsolana.PDA(o.ProgramID, []byte("mint")) {
		panic("observe: noncanonical mint")
	}
	if o.Clock == nil {
		o.Clock = clock.Real()
	}

	return &Observer{
		rpc:       rpc,
		index:     idx,
		opts:      o,
		onChange:  onChange,
		config:    aktsolana.PDA(o.ProgramID, []byte("config")),
		bridge:    aktsolana.PDA(o.ProgramID, []byte("bridge")),
		authority: aktsolana.PDA(o.ProgramID, []byte("mint_authority")),
	}
}

type snapshot struct {
	contextSlot   uint64
	bridgeEnabled bool
	lightClient   solana.PublicKey
	statuses      []*aktsolana.Account
}

// finalizedAccounts reads the deployment accounts and the given status accounts in one call and checks the
// cluster, freshness and deployment, in the TS order.
func (o *Observer) finalizedAccounts(ctx context.Context, statuses []solana.PublicKey) (snapshot, error) {
	genesis, err := o.rpc.GenesisHash(ctx)
	if err != nil {
		return snapshot{}, err
	}
	if genesis != o.opts.GenesisHash {
		return snapshot{}, errCluster
	}

	keys := append([]solana.PublicKey{o.opts.ProgramID, o.config, o.bridge, o.opts.Mint}, statuses...)
	slot, accounts, err := o.rpc.MultipleAccounts(ctx, keys)
	if err != nil {
		return snapshot{}, err
	}
	if slot == 0 {
		return snapshot{}, errNoSlot
	}

	if err := o.checkFresh(ctx, slot); err != nil {
		return snapshot{}, err
	}

	program, launchAccount, bridgeAccount, mintAccount := accounts[0], accounts[1], accounts[2], accounts[3]
	if program == nil || !program.Executable {
		return snapshot{}, errNotExecutable
	}
	for _, a := range []*aktsolana.Account{launchAccount, bridgeAccount} {
		if a == nil || a.Executable || a.Owner != o.opts.ProgramID {
			return snapshot{}, errConfigOwner
		}
	}

	launch, err := aktsolana.DecodeLaunchConfig(launchAccount.Data)
	if err != nil {
		return snapshot{}, err
	}
	bridge, err := aktsolana.DecodeBridgeConfig(bridgeAccount.Data)
	if err != nil {
		return snapshot{}, err
	}
	if launch.Version != launchVersion {
		return snapshot{}, errLaunchVersion
	}
	if launch.Mint != o.opts.Mint {
		return snapshot{}, errConfiguredMint
	}
	if bridge.ChainID != o.opts.ChainID {
		return snapshot{}, errOriginChain
	}
	if err := o.checkLightClient(bridge.LightClientProgram, slot); err != nil {
		return snapshot{}, err
	}
	if err := o.checkMint(mintAccount); err != nil {
		return snapshot{}, err
	}

	return snapshot{
		contextSlot:   slot,
		bridgeEnabled: launch.BridgeEnabled,
		lightClient:   bridge.LightClientProgram,
		statuses:      accounts[4:],
	}, nil
}

// checkFresh requires the block time of slot to lie within [-10 s, MaxAge] of now. A slot without a block time is stale.
func (o *Observer) checkFresh(ctx context.Context, slot uint64) error {
	t, err := o.rpc.BlockTime(ctx, slot)
	if err != nil {
		return err
	}
	if t == nil {
		return errStale
	}

	age := o.opts.Clock.Now().Sub(time.Unix(*t, 0))
	if age < -futureSlack || age > o.opts.MaxAge {
		return errStale
	}

	return nil
}

// checkMint ports unpackMint and the TS mint checks.
func (o *Observer) checkMint(a *aktsolana.Account) error {
	if a == nil {
		return errMintMissing
	}
	if a.Owner != solana.TokenProgramID {
		return errMintOwner
	}

	mint, err := aktsolana.DecodeMint(a.Data)
	if err != nil {
		return fmt.Errorf("%w: %w", errMintConfig, err)
	}
	if !mint.Initialized || mint.Decimals != mintDecimals || mint.FreezeAuthority != nil {
		return errMintConfig
	}
	if mint.MintAuthority == nil || *mint.MintAuthority != o.authority {
		return errMintAuthority
	}

	return nil
}

// checkLightClient fails closed on a pin mismatch. Unpinned, it follows the bridge: an older snapshot is ignored, and
// a different program at the same or a newer slot is a change reported under the follow lock.
func (o *Observer) checkLightClient(current solana.PublicKey, slot uint64) error {
	if pinned := o.opts.LightClientPin; pinned != nil {
		if current != *pinned {
			return &MismatchError{Pinned: *pinned, Current: current}
		}

		return nil
	}

	o.follow.Lock()
	defer o.follow.Unlock()

	if o.following && slot < o.highest {
		return nil
	}
	if o.following && o.followed != current && o.onChange != nil {
		o.onChange(Change{Previous: o.followed, Current: current, ContextSlot: slot})
	}
	o.following, o.followed, o.highest = true, current, slot

	return nil
}

// Deployment takes a snapshot of the deployment accounts. It fails on a light-client pin mismatch.
func (o *Observer) Deployment(ctx context.Context) (Deployment, error) {
	s, err := o.finalizedAccounts(ctx, nil)
	if err != nil {
		return Deployment{}, err
	}

	return Deployment{ContextSlot: s.contextSlot, BridgeEnabled: s.bridgeEnabled, LightClient: s.lightClient}, nil
}

// Chunk takes one snapshot of the deployment accounts and the status accounts of ids. With the four deployment
// accounts, the RPC's 100-key limit allows 96 IDs.
func (o *Observer) Chunk(ctx context.Context, ids []sinkid.ID) (*Chunk, error) {
	addresses := make([]solana.PublicKey, len(ids))
	for i, id := range ids {
		addresses[i] = aktsolana.StatusAddress(o.opts.ProgramID, id)
	}

	s, err := o.finalizedAccounts(ctx, addresses)
	if err != nil {
		return nil, err
	}

	statuses := make(map[solana.PublicKey]*aktsolana.Account, len(addresses))
	for i, address := range addresses {
		statuses[address] = s.statuses[i]
	}

	return &Chunk{contextSlot: s.contextSlot, statuses: statuses}, nil
}

// Finalized checks the finalized status of rec in the chunk snapshot against the Akash record.
func (o *Observer) Finalized(c *Chunk, rec sinkid.Record) (Finalized, error) {
	address := aktsolana.StatusAddress(o.opts.ProgramID, rec.ID)

	account, ok := c.statuses[address]
	if !ok {
		return Finalized{}, errNotInChunk
	}
	if account == nil {
		return Finalized{Kind: policy.KindAbsent, ContextSlot: c.contextSlot}, nil
	}
	if account.Executable || account.Owner != o.opts.ProgramID {
		return Finalized{}, errStatusOwner
	}

	status, err := aktsolana.DecodeTransferStatus(account.Data)
	if err != nil {
		return Finalized{}, err
	}
	if !matchesID(status.ID, rec.ID) {
		return Finalized{}, errStatusID
	}
	if status.ProofHeight <= height(rec.ID) {
		return Finalized{}, errProofHeight
	}

	slot := status.ResolvedAtSlot
	if slot == 0 || slot > c.contextSlot {
		return Finalized{}, errNotFinalized
	}

	var outcome sinkv1.Outcome
	switch {
	case status.Outcome.Minted != nil:
		if status.Outcome.Minted.Amount != rec.Amount {
			return Finalized{}, errMintAmount
		}
		if status.Outcome.Minted.Recipient != rec.Recipient {
			return Finalized{}, errMintRecipient
		}
		outcome = sinkv1.OutcomeMinted
	case status.Outcome.Voided != nil:
		outcome = sinkv1.OutcomeVoided
	default:
		return Finalized{}, errUnknownOutcome
	}

	return Finalized{
		Kind:        policy.KindResolved,
		ContextSlot: c.contextSlot,
		Address:     address,
		Status:      &status,
		Outcome:     outcome,
		Slot:        slot,
	}, nil
}

// Observe returns what this node observes for rec in the chunk snapshot. reported holds the Akash votes and the
// governance observation of the transfer; own, when set, is this reporter's authenticated stored vote. Its evidence is
// reused after the immutable finalized status is rechecked, so pruned transaction history does not break transfers
// already observed.
func (o *Observer) Observe(
	ctx context.Context,
	c *Chunk,
	rec sinkid.Record,
	reported []sinkv1.Observation,
	own *sinkv1.Observation,
) (policy.Observed, error) {
	f, err := o.Finalized(c, rec)
	if err != nil {
		return policy.Observed{}, err
	}
	if f.Kind == policy.KindAbsent {
		return o.absent(ctx, rec, f.ContextSlot, reported)
	}

	var signature []byte
	if own != nil && own.Outcome == f.Outcome && own.Slot == f.Slot && validSignature(own.TransactionSignature) {
		signature = slices.Clone(own.TransactionSignature)
	} else {
		sig, err := o.index.Lookup(ctx, f.Slot, f.Address, *f.Status)
		if err != nil {
			return policy.Observed{}, lookupError(err, *f.Status, f.Slot)
		}
		signature = sig[:]
	}

	return policy.Observed{
		Kind:        policy.KindResolved,
		ContextSlot: f.ContextSlot,
		Observation: sinkv1.Observation{Outcome: f.Outcome, Slot: f.Slot, TransactionSignature: signature},
	}, nil
}

// lookupError gives the TS text for an evidence index error. Missing evidence and the caller's own cancellation pass
// through; the sentinels are checked first because a missing block wraps the RPC's request timeout.
func lookupError(err error, st aktsolana.TransferStatus, slot uint64) error {
	switch {
	case errors.Is(err, aktsolana.ErrBlockMissing),
		errors.Is(err, aktsolana.ErrNotInBlock),
		errors.Is(err, aktsolana.ErrMultipleInBlock),
		errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded):
		return err
	case st.ResolvedAtSlot != slot:
		return errSlotMismatch
	default:
		// The resolution's signature is not 64 non-zero bytes; TS asserted that without a message.
		return err
	}
}

// absent supports a veto only when every plausible reported slot is at or below this node's finalized slot. Slots
// beyond the limit cannot be finalized and are left for the policy to exclude and alert on.
func (o *Observer) absent(
	ctx context.Context,
	rec sinkid.Record,
	contextSlot uint64,
	reported []sinkv1.Observation,
) (policy.Observed, error) {
	limit := contextSlot + o.opts.SlotLead
	if limit < contextSlot {
		limit = math.MaxUint64
	}

	for _, r := range reported {
		if r.Slot > contextSlot && r.Slot <= limit {
			return policy.Observed{}, errBehind
		}
	}

	// The status is permanent, so its absence already disproves evidence at or below contextSlot. A finalized
	// transaction that still shows it means this node is inconsistent and must not support a veto.
	for _, r := range reported {
		if r.Slot > contextSlot {
			continue
		}

		shows, err := o.showsReported(ctx, rec, r)
		if err != nil {
			return policy.Observed{}, err
		}
		if shows {
			return policy.Observed{}, errContradicts
		}
	}

	return policy.Observed{Kind: policy.KindAbsent, ContextSlot: contextSlot, SlotLimit: &limit}, nil
}

// showsReported reports whether the reported transaction is finalized at the reported slot under the reported first
// signature and resolves rec with the reported outcome.
func (o *Observer) showsReported(ctx context.Context, rec sinkid.Record, r sinkv1.Observation) (bool, error) {
	mint := r.Outcome == sinkv1.OutcomeMinted
	if (!mint && r.Outcome != sinkv1.OutcomeVoided) || len(r.TransactionSignature) != signatureSize {
		return false, nil
	}

	signature := solana.SignatureFromBytes(r.TransactionSignature).String()
	tx, err := o.rpc.Transaction(ctx, signature)
	if err != nil || tx == nil {
		return false, err
	}
	if tx.Slot != r.Slot || len(tx.Signatures) == 0 || tx.Signatures[0] != signature {
		return false, nil
	}

	address := aktsolana.StatusAddress(o.opts.ProgramID, rec.ID)

	return aktsolana.Resolves(*tx, o.opts.ProgramID, address, func(ev aktsolana.TransferResolved) bool {
		if !matchesID(ev.ID, rec.ID) || ev.ProofHeight <= height(rec.ID) || ev.ResolvedAtSlot != tx.Slot {
			return false
		}
		if !mint {
			return ev.Outcome.Voided != nil
		}

		m := ev.Outcome.Minted

		return m != nil && m.Recipient == rec.Recipient && m.Amount == rec.Amount
	}), nil
}

// matchesID reports whether a program transfer ID is the Akash record ID.
func matchesID(id aktsolana.TransferID, want sinkid.ID) bool {
	return bytes.Equal(id.Owner, want.OwnerBytes) && id.Height == height(want) && id.Sequence == want.Sequence
}

func height(id sinkid.ID) uint64 {
	return uint64(id.Height) //nolint:gosec // sink heights are positive int64
}

// validSignature reports a 64-byte signature that is not all zero.
func validSignature(sig []byte) bool {
	return len(sig) == signatureSize && slices.ContainsFunc(sig, func(b byte) bool { return b != 0 })
}
