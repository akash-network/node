// Package policy ports contracts/solana/reporter/policy.ts: it decides which sink messages the reporter sends.
package policy

import (
	"errors"
	"fmt"
	"slices"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

const (
	AlertFabricatedSlot   = "Akash reports a Solana slot beyond any plausible finalized slot"
	AlertNoStatus         = "Akash reports an outcome without a finalized Solana status"
	AlertContradiction    = "Akash settlement contradicts finalized Solana outcome"
	AlertGovernanceReview = "Transfer requires governance review"
)

var (
	errChallengeDeadline = errors.New("invalid challenge deadline")
	errRatio             = errors.New("invalid reporter ratio")
	errRatioAboveOne     = errors.New("reporter ratio must be at most one")
	errQuorum            = errors.New("reporter quorum must exceed two thirds of the members")
	errSettlement        = errors.New("invalid sink settlement")
	errNotReporter       = errors.New("account is not a current sink reporter")
	errThresholdMismatch = errors.New("current policy threshold mismatch")
	errSlotLimit         = errors.New("invalid plausible slot limit")
	errBehindEvidence    = errors.New("observation is behind reported evidence")
	errOutcome           = errors.New("invalid observation outcome")
	errKind              = errors.New("invalid observed kind")
)

// Kind tells whether Solana finalized a status for a transfer.
type Kind int

const (
	KindAbsent Kind = iota + 1
	KindResolved
)

// Observed is what a node saw on Solana. Observation is set for KindResolved only. SlotLimit, when set, is the highest slot
// that any node could have finalized at ContextSlot.
type Observed struct {
	Kind        Kind
	ContextSlot uint64
	SlotLimit   *uint64
	Observation sinkv1.Observation
}

// Snapshot is the chain state a decision is made from. Time is the block time in Unix seconds.
// RefundHead is the head of the refund queue: x/sink pays refunds oldest first.
type Snapshot struct {
	Params      sinkv1.Params
	Policy      sinkv1.ReporterPolicy
	RefundUsage sinkv1.EpochUsage
	Epoch       uint64
	Time        int64
	RefundHead  *sinkid.ID
	Transfer    sinkv1.TransferState
}

// Plan is the messages to send and the alerts to raise for one transfer.
type Plan struct {
	Messages []sdk.Msg
	Alerts   []string
}

// ChallengeSeconds returns the end of the challenge window in Unix seconds.
func ChallengeSeconds(settlement sinkv1.SettlementData) (int64, error) {
	if settlement.ChallengeEnd < 0 {
		return 0, errChallengeDeadline
	}

	return settlement.ChallengeEnd, nil
}

func checkRatio(ratio sdkmath.LegacyDec) error {
	if ratio.IsNil() || ratio.IsNegative() {
		return errRatio
	}

	if ratio.GT(sdkmath.LegacyOneDec()) {
		return errRatioAboveOne
	}

	return nil
}

// ReporterThreshold returns ceil(ratio × members), which must exceed two thirds of the members unless there are none.
func ReporterThreshold(ratio sdkmath.LegacyDec, members int) (int, error) {
	if err := checkRatio(ratio); err != nil {
		return 0, err
	}

	threshold := ratio.MulInt64(int64(members)).Ceil().TruncateInt64()
	if members != 0 && 3*threshold <= 2*int64(members) {
		return 0, errQuorum
	}

	return int(threshold), nil
}

// ReportedObservations returns the governance observation, then the observation of every vote.
func ReportedObservations(transfer sinkv1.TransferState) []sinkv1.Observation {
	settlement := transfer.Settlement
	out := make([]sinkv1.Observation, 0, len(settlement.Votes)+1)

	if settlement.GovernanceObservation != nil {
		out = append(out, *settlement.GovernanceObservation)
	}

	for _, vote := range settlement.Votes {
		out = append(out, vote.Observation)
	}

	return out
}

// MinimumSlot returns the highest reported slot, ignoring slots above slotLimit when one is given.
func MinimumSlot(transfer sinkv1.TransferState, slotLimit *uint64) uint64 {
	var slot uint64

	for _, observation := range ReportedObservations(transfer) {
		if observation.Slot > slot && (slotLimit == nil || observation.Slot <= *slotLimit) {
			slot = observation.Slot
		}
	}

	return slot
}

// CompareOutcome tells how a finalized Solana outcome relates to the Akash settlement and its reported observations.
func CompareOutcome(transfer sinkv1.TransferState, outcome sinkv1.Outcome) (contradicts, terminalMatches bool) {
	mint := outcome == sinkv1.OutcomeMinted

	contradicts = (transfer.Status == sinkv1.StatusMinted && !mint) ||
		((transfer.Status == sinkv1.StatusRefundPending || transfer.Status == sinkv1.StatusRefunded) && mint) ||
		slices.ContainsFunc(ReportedObservations(transfer), func(observation sinkv1.Observation) bool {
			return observation.Outcome != outcome
		})
	terminalMatches = (mint && transfer.Status == sinkv1.StatusMinted) ||
		(!mint && transfer.Status == sinkv1.StatusRefunded)

	return contradicts, terminalMatches
}

// SettledConsistently reports a terminal, unheld settlement that agrees with the finalized outcome and every report.
// PlanOutcome would return no message and no alert for it.
func SettledConsistently(transfer sinkv1.TransferState, outcome sinkv1.Outcome) bool {
	contradicts, terminalMatches := CompareOutcome(transfer, outcome)

	return terminalMatches && !contradicts && transfer.Settlement.Hold == sinkv1.HoldNone
}

// PlanOutcome is the pure policy. The chain remains authoritative for membership, rounds, caps, challenge deadlines and
// exact-once payment when the planned messages execute.
func PlanOutcome(reporter string, s Snapshot, o Observed) (Plan, error) {
	transfer := s.Transfer
	settlement := transfer.Settlement

	if settlement.Policy.Version == 0 || settlement.Round == 0 {
		return Plan{}, errSettlement
	}

	if !slices.Contains(s.Policy.Members, reporter) {
		return Plan{}, errNotReporter
	}

	if _, err := ReporterThreshold(s.Policy.Threshold, len(s.Policy.Members)); err != nil {
		return Plan{}, err
	}

	if err := checkRatio(s.Params.ReportersThreshold); err != nil {
		return Plan{}, err
	}

	if !s.Policy.Threshold.Equal(s.Params.ReportersThreshold) {
		return Plan{}, errThresholdMismatch
	}

	record, err := sinkid.DecodeRecord(transfer)
	if err != nil {
		return Plan{}, err
	}

	challengeEnd, err := ChallengeSeconds(settlement)
	if err != nil {
		return Plan{}, err
	}

	result := Plan{Messages: []sdk.Msg{}, Alerts: []string{}}

	switch o.Kind {
	case KindAbsent:
		return planAbsent(reporter, s, o, record, challengeEnd, result)
	case KindResolved:
		return planResolved(reporter, s, o, record, challengeEnd, result)
	default:
		return Plan{}, errKind
	}
}

func planAbsent(reporter string, s Snapshot, o Observed, record sinkid.Record, challengeEnd int64, result Plan) (Plan, error) {
	transfer := s.Transfer
	settlement := transfer.Settlement

	// A slot beyond what any node could have finalized is false evidence. It must not stop a fresh node
	// from proving absence and vetoing.
	if o.SlotLimit != nil && *o.SlotLimit < o.ContextSlot {
		return Plan{}, errSlotLimit
	}

	if o.ContextSlot < MinimumSlot(transfer, o.SlotLimit) {
		return Plan{}, errBehindEvidence
	}

	if o.SlotLimit != nil && MinimumSlot(transfer, nil) > *o.SlotLimit {
		result.Alerts = append(result.Alerts, AlertFabricatedSlot)
	}

	if (transfer.Status != sinkv1.StatusTransferPending && transfer.Status != sinkv1.StatusVoided) ||
		len(ReportedObservations(transfer)) > 0 {
		result.Alerts = append(result.Alerts, AlertNoStatus)

		if transfer.Status == sinkv1.StatusRefundPending && settlement.Hold == sinkv1.HoldNone && s.Time < challengeEnd {
			result.Messages = append(result.Messages, &sinkv1.MsgVetoVoid{
				Signer: reporter,
				ID:     record.ID.Proto(),
				Reason: fmt.Sprintf("No Solana status at finalized slot %d", o.ContextSlot),
			})
		}
	}

	return result, nil
}

func planResolved(reporter string, s Snapshot, o Observed, record sinkid.Record, challengeEnd int64, result Plan) (Plan, error) {
	transfer := s.Transfer
	settlement := transfer.Settlement
	observation := o.Observation

	// A permanent finalized outcome remains evidence even if another reporter invents a future slot.
	// Only an absence check must wait for reported slots.
	if observation.Outcome != sinkv1.OutcomeMinted && observation.Outcome != sinkv1.OutcomeVoided {
		return Plan{}, errOutcome
	}

	mint := observation.Outcome == sinkv1.OutcomeMinted

	contradicts, _ := CompareOutcome(transfer, observation.Outcome)
	if contradicts {
		result.Alerts = append(result.Alerts, AlertContradiction)
	}

	if settlement.Hold != sinkv1.HoldNone {
		result.Alerts = append(result.Alerts, AlertGovernanceReview)
	}

	currentPolicy := settlement.Policy.Version == s.Policy.Version
	alreadyReported := slices.ContainsFunc(settlement.Votes, func(vote sinkv1.Vote) bool {
		return vote.Reporter == reporter &&
			vote.PolicyVersion == s.Policy.Version &&
			vote.Observation.Outcome == observation.Outcome
	})
	active := settlement.Hold == sinkv1.HoldNone &&
		(transfer.Status == sinkv1.StatusTransferPending || transfer.Status == sinkv1.StatusVoided)
	refresh := !currentPolicy &&
		settlement.Hold == sinkv1.HoldNone &&
		settlement.GovernanceObservation == nil &&
		(active || transfer.Status == sinkv1.StatusRefundPending)

	if settlement.Hold != sinkv1.HoldConflict && (refresh || (!alreadyReported && (contradicts || active))) {
		result.Messages = append(result.Messages, &sinkv1.MsgReportOutcome{
			Reporter:      reporter,
			ID:            record.ID.Proto(),
			PolicyVersion: s.Policy.Version,
			Round:         settlement.Round,
			Observation:   observation,
		})

		return result, nil // Re-read after voting before considering any refund.
	}

	spent, err := sinkid.CoinAmount(s.RefundUsage.Amount, false)
	if err != nil {
		return Plan{}, err
	}

	if mint ||
		transfer.Status != sinkv1.StatusRefundPending ||
		settlement.Hold != sinkv1.HoldNone ||
		contradicts ||
		(!currentPolicy && settlement.GovernanceObservation == nil) ||
		!s.Params.RefundsEnabled ||
		s.Epoch == 0 ||
		s.Time < challengeEnd ||
		challengeEnd <= 0 {
		return result, nil
	}

	limit, err := sinkid.CoinAmount(s.Params.MaxEpochRefund, true)
	if err != nil {
		return Plan{}, err
	}

	if spent.Add(sdkmath.NewIntFromUint64(record.Amount)).LTE(limit) &&
		s.RefundHead != nil && sinkid.Compare(*s.RefundHead, record.ID) == 0 {
		result.Messages = append(result.Messages, &sinkv1.MsgFinalizeRefund{
			Executor: reporter,
			ID:       record.ID.Proto(),
		})
	}

	return result, nil
}
