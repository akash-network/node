package relayer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"

	"github.com/gagliardetto/solana-go"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
)

type relayPlan struct {
	id                   sink.ID
	transfer             sink.TransferState
	height, resumeHeight headerHeight
	retained             bool
	unbacked             string
}

func (s *service) plan(ctx context.Context, id sink.ID, height stateHeight, latest headerHeight, known *sink.TransferState) (relayPlan, error) {
	var transfer sink.TransferState
	if known != nil {
		transfer = *known
	} else {
		var err error
		transfer, err = s.native.Transfer(ctx, id, int64(height))
		if err != nil {
			return relayPlan{}, err
		}
	}
	if transfer.ID.Height >= maxSafeInteger {
		return relayPlan{}, fmt.Errorf("Akash RPC cannot represent this height exactly")
	}
	saved, err := s.store.loadProof(id)
	if err != nil {
		return relayPlan{}, err
	}
	planned := relayPlan{
		id:       id,
		transfer: transfer,
		retained: saved != nil,
		unbacked: unbackedMint(transfer),
	}
	minimum := headerHeight(transfer.ID.Height + 1)
	resumeHeight := headerHeight(0)
	if saved != nil {
		resumeHeight = headerHeight(saved.ProofHeight)
		key, err := hex.DecodeString(saved.Key)
		if err != nil {
			return relayPlan{}, err
		}
		hash, err := hex.DecodeString(saved.ValueHash)
		if err != nil {
			return relayPlan{}, err
		}
		retained, err := s.native.Retained(ctx, key, saved.ProofHeight, [32]byte(hash))
		if err != nil {
			return relayPlan{}, err
		}
		if !retained {
			resumeHeight = 0
			minimum = headerHeight(saved.ProofHeight + 1)
		}
	}
	if transfer.Settlement.VoidAuthorization == nil {
		planned.height = minimum
		if resumeHeight != 0 {
			planned.height = resumeHeight
		}
		planned.resumeHeight = resumeHeight
		return planned, nil
	}

	// Header H proves state at H-1; a void needs the complete current settlement.
	seen := make(map[headerHeight]bool)
	for _, candidate := range []headerHeight{resumeHeight, latest, headerHeight(height - 1)} {
		if seen[candidate] || candidate == 0 || candidate > headerHeight(height-1) || candidate < minimum {
			continue
		}
		seen[candidate] = true
		snapshot, err := s.native.Transfer(ctx, id, int64(candidate-1))
		if err != nil {
			return relayPlan{}, err
		}
		if snapshot.Settlement.VoidAuthorization != nil && finalCheck(snapshot, transfer) == "" {
			planned.height = candidate
			if resumeHeight == candidate {
				planned.resumeHeight = candidate
			}
			return planned, nil
		}
	}
	planned.height = max(headerHeight(height), minimum)
	return planned, nil
}

func unbackedMint(transfer sink.TransferState) string {
	if transfer.Settlement.VoidAuthorization != nil {
		return ""
	}
	switch transfer.Status {
	case sink.StatusVoided:
		return "akash-void-request-without-authorization"
	case sink.StatusRefundPending:
		return "akash-voided-without-solana-status"
	case sink.StatusRefunded:
		return "akash-refunded-without-solana-status"
	default:
		return ""
	}
}

func finalCheck(before, after sink.TransferState) string {
	planned, latest := before.Settlement.VoidAuthorization, after.Settlement.VoidAuthorization
	if (planned == nil) != (latest == nil) {
		return "void-authorization-changed"
	}
	if planned != nil {
		if planned.RequestedHeight != latest.RequestedHeight || planned.RequestedRound != latest.RequestedRound {
			return "void-authorization-changed"
		}
		left, leftErr := before.Settlement.Marshal()
		right, rightErr := after.Settlement.Marshal()
		if before.Status != after.Status || leftErr != nil || rightErr != nil || !bytes.Equal(left, right) {
			return "settlement-changed"
		}
	}
	return unbackedMint(after)
}

func relayOrder(entries []relayPlan, payer solana.PublicKey) []relayPlan {
	fresh := func(entry relayPlan) bool {
		return entry.transfer.Status == sink.StatusTransferPending && entry.transfer.Settlement.Hold == sink.HoldNone && entry.transfer.Settlement.VoidAuthorization == nil && !entry.retained
	}
	type ranked struct {
		entry relayPlan
		rank  [32]byte
	}
	ranks := make([]ranked, 0)
	for _, entry := range entries {
		if fresh(entry) {
			digest := sha256.New()
			digest.Write(payer[:])
			digest.Write([]byte(entry.id.String()))
			ranks = append(ranks, ranked{
				entry: entry,
				rank:  [32]byte(digest.Sum(nil)),
			})
		}
	}
	sort.SliceStable(ranks, func(i, j int) bool { return bytes.Compare(ranks[i].rank[:], ranks[j].rank[:]) < 0 })
	result := append([]relayPlan(nil), entries...)
	next := 0
	for i, entry := range entries {
		if fresh(entry) {
			result[i] = ranks[next].entry
			next++
		}
	}
	return result
}

type refreshInput struct {
	Latest, Target                        headerHeight
	Required                              []headerHeight
	AgeSeconds, TrustingSeconds, Fraction float64
}

func headerRefresh(input refreshInput) string {
	if input.Target <= input.Latest {
		return ""
	}
	for _, height := range input.Required {
		if height > input.Latest && height <= input.Target {
			return "proof"
		}
	}
	if input.AgeSeconds >= input.TrustingSeconds*input.Fraction {
		return "age"
	}
	return ""
}

func mintWait(view bridge.MintView, amount *big.Int, mints bool, reserved *big.Int) string {
	if !view.Enabled {
		return "bridge-paused"
	}
	if !mints {
		return ""
	}
	day := big.NewInt(view.NowSeconds / 86_400)
	mintDay := new(big.Int).SetUint64(view.MintDay)
	if day.Cmp(mintDay) < 0 {
		return "solana-clock-behind"
	}
	if amount.Cmp(new(big.Int).SetUint64(view.PerTransferMax)) > 0 {
		return "per-transfer-limit"
	}
	used := new(big.Int).Set(reserved)
	if day.Cmp(mintDay) == 0 {
		used.Add(used, new(big.Int).SetUint64(view.MintedToday))
	}
	if used.Add(used, amount).Cmp(new(big.Int).SetUint64(view.DailyMintCap)) > 0 {
		return "daily-limit"
	}
	return ""
}

func (s *service) payable(ctx context.Context, plans []relayPlan, checkpoint func(context.Context) error) ([]relayPlan, error) {
	records := make([]sink.RecordData, 0, len(plans))
	for _, entry := range plans {
		if entry.transfer.Settlement.VoidAuthorization == nil {
			records = append(records, entry.transfer.Data)
		}
	}
	view, err := s.reader.MintView(ctx, records, checkpoint)
	if err != nil {
		return nil, err
	}
	recipients := make(map[sink.ID]bool, len(records))
	next := 0
	for _, entry := range plans {
		if entry.transfer.Settlement.VoidAuthorization == nil {
			recipients[entry.id] = view.Recipients[next].Mints
			next++
		}
	}
	reserved := new(big.Int)
	ready := make([]relayPlan, 0, len(plans))
	for _, entry := range plans {
		if checkpoint != nil {
			if err := checkpoint(ctx); err != nil {
				return nil, err
			}
		}
		mints := recipients[entry.id]
		authorization := entry.transfer.Settlement.VoidAuthorization
		reason := ""
		if authorization == nil {
			reason = mintWait(view, entry.transfer.Data.Amount.Amount.BigInt(), mints, reserved)
		}
		if reason != "" {
			if s.waiting[entry.id] != reason {
				s.emit(false, map[string]any{
					"event":  "relay-wait",
					"id":     entry.id.String(),
					"reason": reason,
				})
			}
			if s.waiting == nil {
				s.waiting = make(map[sink.ID]string)
			}
			s.waiting[entry.id] = reason
			continue
		}
		delete(s.waiting, entry.id)
		if mints && authorization == nil {
			reserved.Add(reserved, entry.transfer.Data.Amount.Amount.BigInt())
		}
		ready = append(ready, entry)
	}
	return ready, nil
}
