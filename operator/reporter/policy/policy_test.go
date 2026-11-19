package policy_test

import (
	"bytes"
	"math"
	"slices"
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

const (
	uakt        = "uakt"
	reportMsg   = "/akash.sink.v1.MsgReportOutcome"
	vetoMsg     = "/akash.sink.v1.MsgVetoVoid"
	refundMsg   = "/akash.sink.v1.MsgFinalizeRefund"
	reporter    = "reporter"
	baseTime    = int64(86410)
	baseEpoch   = uint64(57)
	startRound  = uint64(1)
	startPolicy = uint64(1)
)

var members = []string{reporter, "b", "c", "d"}

func dec(t *testing.T, s string) sdkmath.LegacyDec {
	t.Helper()

	d, err := sdkmath.LegacyNewDecFromStr(s)
	require.NoError(t, err)

	return d
}

func testID(t *testing.T, sequence uint64) sinkid.ID {
	t.Helper()

	owner, err := bech32.ConvertAndEncode("akash", bytes.Repeat([]byte{42}, 20))
	require.NoError(t, err)

	id, err := sinkid.Parse(owner + "/1/" + sdkmath.NewIntFromUint64(sequence).String())
	require.NoError(t, err)

	return id
}

func testSnapshot(t *testing.T) policy.Snapshot {
	t.Helper()

	id := testID(t, 1)

	return policy.Snapshot{
		Params: sinkv1.Params{
			Reporters:          slices.Clone(members),
			ReportersThreshold: dec(t, "0.75"),
			RefundsEnabled:     true,
			MaxEpochRefund:     sdk.NewInt64Coin(uakt, 100),
		},
		Policy: sinkv1.ReporterPolicy{
			Version:   startPolicy,
			Members:   slices.Clone(members),
			Threshold: dec(t, "0.75"),
		},
		RefundUsage: sinkv1.EpochUsage{Epoch: baseEpoch, Amount: sdk.NewInt64Coin(uakt, 0)},
		Epoch:       baseEpoch,
		Time:        baseTime,
		RefundHead:  &id,
		Transfer: sinkv1.TransferState{
			ID: id.Proto(),
			Data: sinkv1.RecordData{
				Dest:   solana.NewWallet().PublicKey().String(),
				Amount: sdk.NewInt64Coin(uakt, 60),
			},
			Status: sinkv1.StatusTransferPending,
			Settlement: sinkv1.SettlementData{
				Round: startRound,
				Policy: sinkv1.ReporterPolicy{
					Version:   startPolicy,
					Members:   slices.Clone(members),
					Threshold: dec(t, "0.75"),
				},
			},
		},
	}
}

func observation(outcome sinkv1.Outcome) sinkv1.Observation {
	return sinkv1.Observation{Outcome: outcome, Slot: 100, TransactionSignature: bytes.Repeat([]byte{7}, 64)}
}

func resolved(outcome sinkv1.Outcome) policy.Observed {
	return policy.Observed{Kind: policy.KindResolved, ContextSlot: 110, Observation: observation(outcome)}
}

func absent() policy.Observed {
	return policy.Observed{Kind: policy.KindAbsent, ContextSlot: 110}
}

func addVote(s *policy.Snapshot, voter string, version uint64, outcome sinkv1.Outcome) {
	st := &s.Transfer.Settlement
	st.Votes = append(slices.Clone(st.Votes), sinkv1.Vote{Reporter: voter, PolicyVersion: version, Observation: observation(outcome)})
}

// voided is a refund-pending transfer whose challenge window ends at the snapshot time.
func voided(t *testing.T) policy.Snapshot {
	t.Helper()

	s := testSnapshot(t)
	s.Transfer.Status = sinkv1.StatusRefundPending
	s.Transfer.Settlement.ChallengeEnd = s.Time

	for _, voter := range []string{reporter, "b", "c"} {
		addVote(&s, voter, startPolicy, sinkv1.OutcomeVoided)
	}

	return s
}

func plan(t *testing.T, who string, s policy.Snapshot, o policy.Observed) policy.Plan {
	t.Helper()

	p, err := policy.PlanOutcome(who, s, o)
	require.NoError(t, err)

	return p
}

func urls(p policy.Plan) []string {
	out := make([]string, 0, len(p.Messages))
	for _, m := range p.Messages {
		out = append(out, sdk.MsgTypeURL(m))
	}

	return out
}

func requireMessages(t *testing.T, want []string, p policy.Plan) {
	t.Helper()
	require.Equal(t, want, urls(p))
}

func reportOf(t *testing.T, p policy.Plan) *sinkv1.MsgReportOutcome {
	t.Helper()
	require.NotEmpty(t, p.Messages)

	msg, ok := p.Messages[0].(*sinkv1.MsgReportOutcome)
	require.True(t, ok, "first message is %T", p.Messages[0])

	return msg
}

func TestPolicy(t *testing.T) {
	t.Run("reports exact composite identity, current policy and round; dedupes only same outcome/version", func(t *testing.T) {
		s := testSnapshot(t)
		observed := resolved(sinkv1.OutcomeMinted)

		msg := reportOf(t, plan(t, reporter, s, observed))
		require.Equal(t, s.Transfer.ID, msg.ID)
		require.Equal(t, observed.Observation, msg.Observation)
		require.Equal(t, uint64(1), msg.PolicyVersion)
		require.Equal(t, uint64(1), msg.Round)
		require.Equal(t, reporter, msg.Reporter)

		addVote(&s, reporter, 1, observed.Observation.Outcome)
		require.Empty(t, plan(t, reporter, s, observed).Messages)

		s.Policy.Version = 9007199254740993
		next := reportOf(t, plan(t, reporter, s, observed))
		require.Equal(t, s.Policy.Version, next.PolicyVersion)
		require.Len(t, plan(t, reporter, s, resolved(sinkv1.OutcomeVoided)).Messages, 1)
	})

	t.Run("absence never creates a void or deadline refund", func(t *testing.T) {
		s := testSnapshot(t)
		s.Transfer.Status = sinkv1.StatusVoided
		s.Transfer.Settlement.VoidAuthorization = &sinkv1.VoidAuthorization{RequestedHeight: 1, RequestedRound: 1}
		require.Equal(t, policy.Plan{Messages: []sdk.Msg{}, Alerts: []string{}}, plan(t, reporter, s, absent()))

		unpaid := voided(t)
		unpaid.Time += 86400
		require.Empty(t, plan(t, reporter, unpaid, absent()).Messages)
	})

	t.Run("fresh absence vetoes only during challenge and never behind plausible evidence", func(t *testing.T) {
		s := voided(t)
		s.Time--
		require.Equal(t, uint64(100), policy.MinimumSlot(s.Transfer, nil))

		_, err := policy.PlanOutcome(reporter, s, policy.Observed{Kind: policy.KindAbsent, ContextSlot: 99})
		require.ErrorContains(t, err, "behind")

		requireMessages(t, []string{vetoMsg}, plan(t, reporter, s, absent()))

		fabricated := append([]sinkv1.Vote(nil), s.Transfer.Settlement.Votes...)
		fabricated[0].Observation = sinkv1.Observation{
			Outcome:              sinkv1.OutcomeVoided,
			Slot:                 math.MaxUint64,
			TransactionSignature: bytes.Repeat([]byte{8}, 64),
		}
		s.Transfer.Settlement.Votes = fabricated

		limit := uint64(710)
		limited := policy.Observed{Kind: policy.KindAbsent, ContextSlot: 110, SlotLimit: &limit}
		p := plan(t, reporter, s, limited)
		require.Contains(t, p.Alerts, "Akash reports a Solana slot beyond any plausible finalized slot")
		require.Len(t, p.Messages, 1)

		s.Transfer.Settlement.Hold = sinkv1.HoldVeto
		require.Empty(t, plan(t, reporter, s, limited).Messages)
	})

	t.Run("refund requires finalized void, queue head, authorization, challenge, cap, enablement and no hold", func(t *testing.T) {
		requireMessages(t, []string{refundMsg}, plan(t, reporter, voided(t), resolved(sinkv1.OutcomeVoided)))

		other := testID(t, 9)

		for name, change := range map[string]func(s *policy.Snapshot){
			"challenge still open": func(s *policy.Snapshot) { s.Time-- },
			"refunds disabled":     func(s *policy.Snapshot) { s.Params.RefundsEnabled = false },
			"cap reached":          func(s *policy.Snapshot) { s.RefundUsage.Amount = sdk.NewInt64Coin(uakt, 41) },
			"stale policy":         func(s *policy.Snapshot) { s.Policy.Version++ },
			"not the queue head":   func(s *policy.Snapshot) { s.RefundHead = &other },
			"empty queue":          func(s *policy.Snapshot) { s.RefundHead = nil },
			"conflict hold":        func(s *policy.Snapshot) { s.Transfer.Settlement.Hold = sinkv1.HoldConflict },
		} {
			s := voided(t)
			change(&s)

			p := plan(t, reporter, s, resolved(sinkv1.OutcomeVoided))
			require.NotContains(t, urls(p), refundMsg, name)
		}
	})

	t.Run("refund capacity follows the native epoch without shortening the challenge", func(t *testing.T) {
		s := voided(t)
		s.RefundUsage.Amount = sdk.NewInt64Coin(uakt, 100)
		s.Time += 86400
		require.Empty(t, plan(t, reporter, s, resolved(sinkv1.OutcomeVoided)).Messages)

		s.Epoch++
		s.RefundUsage = sinkv1.EpochUsage{Epoch: s.Epoch, Amount: sdk.NewInt64Coin(uakt, 0)}
		s.Transfer.Settlement.ChallengeEnd = s.Time + 1
		require.Empty(t, plan(t, reporter, s, resolved(sinkv1.OutcomeVoided)).Messages)

		s.Time++
		requireMessages(t, []string{refundMsg}, plan(t, reporter, s, resolved(sinkv1.OutcomeVoided)))

		s.Epoch = 0
		s.RefundUsage.Epoch = 0
		require.Empty(t, plan(t, reporter, s, resolved(sinkv1.OutcomeVoided)).Messages)
	})

	t.Run("governance refund authorization survives policy rotation; paid and minted authorization is retained", func(t *testing.T) {
		s := voided(t)
		governance := observation(sinkv1.OutcomeVoided)
		s.Transfer.Settlement.GovernanceObservation = &governance
		s.Transfer.Settlement.Votes = nil
		s.Policy.Version++
		requireMessages(t, []string{refundMsg}, plan(t, reporter, s, resolved(sinkv1.OutcomeVoided)))

		s.Transfer.Status = sinkv1.StatusRefunded
		s.Transfer.Settlement.ChallengeEnd = 0
		require.Empty(t, plan(t, reporter, s, resolved(sinkv1.OutcomeVoided)).Messages)

		conflict := plan(t, reporter, s, resolved(sinkv1.OutcomeMinted))
		require.Contains(t, conflict.Alerts, "Akash settlement contradicts finalized Solana outcome")
		requireMessages(t, []string{reportMsg}, conflict)
		require.Equal(t, uint64(1), s.Transfer.Settlement.Policy.Version)
		require.NotNil(t, s.Transfer.Settlement.GovernanceObservation)
	})

	t.Run("decimal quorum is exact and financial terminal state does not conceal holds or opposing votes", func(t *testing.T) {
		for _, tc := range []struct {
			ratio   string
			members int
			want    int
		}{
			{"0.75", 4, 3},
			{"0.6", 4, 3},
			{"0.666666666666666666", 4, 3},
			{"0.666666666666666667", 3, 3},
		} {
			got, err := policy.ReporterThreshold(dec(t, tc.ratio), tc.members)
			require.NoError(t, err, tc.ratio)
			require.Equal(t, tc.want, got, tc.ratio)
		}

		_, err := policy.ReporterThreshold(dec(t, "0.666666666666666666"), 3)
		require.Error(t, err)

		_, err = policy.ReporterThreshold(dec(t, "1.01"), 0)
		require.Error(t, err)

		for _, ratio := range []sdkmath.LegacyDec{dec(t, "0.5"), dec(t, "1.01"), {}} {
			_, err = policy.ReporterThreshold(ratio, 4)
			require.Error(t, err)
		}

		s := testSnapshot(t)
		s.Policy.Threshold = dec(t, "0.750000000000000000")
		require.Len(t, plan(t, reporter, s, resolved(sinkv1.OutcomeVoided)).Messages, 1)

		s.Params.ReportersThreshold = dec(t, "0.74")
		_, err = policy.PlanOutcome(reporter, s, resolved(sinkv1.OutcomeVoided))
		require.ErrorContains(t, err, "threshold mismatch")

		s.Policy.Threshold = dec(t, "0.5")
		s.Params.ReportersThreshold = dec(t, "0.5")
		_, err = policy.PlanOutcome(reporter, s, resolved(sinkv1.OutcomeVoided))
		require.ErrorContains(t, err, "quorum")

		transfer := sinkv1.TransferState{
			Status:     sinkv1.StatusMinted,
			Settlement: sinkv1.SettlementData{Policy: sinkv1.ReporterPolicy{Version: 1}},
		}
		require.True(t, policy.SettledConsistently(transfer, sinkv1.OutcomeMinted))

		transfer.Settlement.Hold = sinkv1.HoldConflict
		require.False(t, policy.SettledConsistently(transfer, sinkv1.OutcomeMinted))

		transfer.Settlement.Hold = sinkv1.HoldNone
		transfer.Settlement.Votes = []sinkv1.Vote{{Reporter: "b", PolicyVersion: 1, Observation: observation(sinkv1.OutcomeVoided)}}
		require.False(t, policy.SettledConsistently(transfer, sinkv1.OutcomeMinted))
	})

	t.Run("a frozen certificate does not buy repeated same-outcome votes from the remaining reporter", func(t *testing.T) {
		s := voided(t)
		s.Time--
		require.Empty(t, plan(t, "d", s, resolved(sinkv1.OutcomeVoided)).Messages)
		requireMessages(t, []string{reportMsg}, plan(t, "d", s, resolved(sinkv1.OutcomeMinted)))

		s.Transfer.Settlement.Hold = sinkv1.HoldConflict
		addVote(&s, "d", s.Policy.Version, sinkv1.OutcomeMinted)

		for _, outcome := range []sinkv1.Outcome{sinkv1.OutcomeMinted, sinkv1.OutcomeVoided} {
			p := plan(t, reporter, s, resolved(outcome))
			require.Empty(t, p.Messages)
			require.Contains(t, p.Alerts, "Transfer requires governance review")
		}
	})

	t.Run("policy rotation refreshes only unheld unpaid attempts and retains held evidence", func(t *testing.T) {
		s := voided(t)
		s.Policy.Version++
		requireMessages(t, []string{reportMsg}, plan(t, "d", s, resolved(sinkv1.OutcomeVoided)))

		s.Transfer.Settlement.Hold = sinkv1.HoldVeto
		require.Empty(t, plan(t, "d", s, resolved(sinkv1.OutcomeVoided)).Messages)
		requireMessages(t, []string{reportMsg}, plan(t, "d", s, resolved(sinkv1.OutcomeMinted)))

		s.Transfer.Settlement.Hold = sinkv1.HoldConflict
		require.Empty(t, plan(t, "d", s, resolved(sinkv1.OutcomeMinted)).Messages)
	})
}

func TestPlanErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		change func(s *policy.Snapshot)
		who    string
		o      policy.Observed
		err    string
	}{
		"settlement without round": {
			change: func(s *policy.Snapshot) { s.Transfer.Settlement.Round = 0 },
			err:    "invalid sink settlement",
		},
		"settlement without policy": {
			change: func(s *policy.Snapshot) { s.Transfer.Settlement.Policy.Version = 0 },
			err:    "invalid sink settlement",
		},
		"not a member": {
			who: "stranger",
			err: "account is not a current sink reporter",
		},
		"weak quorum": {
			change: func(s *policy.Snapshot) {
				s.Policy.Threshold = dec(t, "0.5")
				s.Params.ReportersThreshold = dec(t, "0.5")
			},
			err: "reporter quorum must exceed two thirds of the members",
		},
		"threshold mismatch": {
			change: func(s *policy.Snapshot) { s.Params.ReportersThreshold = dec(t, "0.74") },
			err:    "current policy threshold mismatch",
		},
		"slot limit below context": {
			o:   policy.Observed{Kind: policy.KindAbsent, ContextSlot: 110, SlotLimit: ptr(109)},
			err: "invalid plausible slot limit",
		},
		"behind evidence": {
			change: func(s *policy.Snapshot) { addVote(s, "b", 1, sinkv1.OutcomeVoided) },
			o:      policy.Observed{Kind: policy.KindAbsent, ContextSlot: 99},
			err:    "observation is behind reported evidence",
		},
		"negative challenge end": {
			change: func(s *policy.Snapshot) { s.Transfer.Settlement.ChallengeEnd = -1 },
			err:    "invalid challenge deadline",
		},
		"unknown outcome": {
			o:   policy.Observed{Kind: policy.KindResolved, ContextSlot: 110, Observation: observation(sinkv1.OutcomeUnspecified)},
			err: "invalid observation outcome",
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := testSnapshot(t)
			if tc.change != nil {
				tc.change(&s)
			}

			who, o := tc.who, tc.o
			if who == "" {
				who = reporter
			}

			if o.Kind == 0 {
				o = resolved(sinkv1.OutcomeVoided)
			}

			_, err := policy.PlanOutcome(who, s, o)
			require.EqualError(t, err, tc.err)
		})
	}

	t.Run("ratio", func(t *testing.T) {
		_, err := policy.ReporterThreshold(sdkmath.LegacyDec{}, 4)
		require.EqualError(t, err, "invalid reporter ratio")

		_, err = policy.ReporterThreshold(dec(t, "-0.1"), 4)
		require.EqualError(t, err, "invalid reporter ratio")

		_, err = policy.ReporterThreshold(dec(t, "1.01"), 4)
		require.EqualError(t, err, "reporter ratio must be at most one")
	})

	t.Run("challenge deadline", func(t *testing.T) {
		got, err := policy.ChallengeSeconds(sinkv1.SettlementData{ChallengeEnd: 5})
		require.NoError(t, err)
		require.Equal(t, int64(5), got)
	})
}

func ptr(v uint64) *uint64 { return &v }

func TestVetoReason(t *testing.T) {
	s := voided(t)
	s.Time--

	p := plan(t, reporter, s, policy.Observed{Kind: policy.KindAbsent, ContextSlot: 123})
	require.Len(t, p.Messages, 1)

	msg, ok := p.Messages[0].(*sinkv1.MsgVetoVoid)
	require.True(t, ok)
	require.Equal(t, "No Solana status at finalized slot 123", msg.Reason)
	require.Equal(t, reporter, msg.Signer)
	require.Equal(t, s.Transfer.ID, msg.ID)
}

func TestAlerts(t *testing.T) {
	require.Equal(t, "Akash reports a Solana slot beyond any plausible finalized slot", policy.AlertFabricatedSlot)
	require.Equal(t, "Akash reports an outcome without a finalized Solana status", policy.AlertNoStatus)
	require.Equal(t, "Akash settlement contradicts finalized Solana outcome", policy.AlertContradiction)
	require.Equal(t, "Transfer requires governance review", policy.AlertGovernanceReview)
}

func TestKeyOf(t *testing.T) {
	id := testID(t, 3)

	vote := &sinkv1.MsgReportOutcome{
		Reporter:      reporter,
		ID:            id.Proto(),
		PolicyVersion: 4,
		Round:         2,
		Observation:   observation(sinkv1.OutcomeMinted),
	}
	require.Equal(t,
		policy.Key{ID: id.String(), Kind: policy.MsgVote, Outcome: sinkv1.OutcomeMinted, Round: 2, PolicyVersion: 4},
		policy.KeyOf(id, vote))

	other := *vote
	other.Round = 3
	require.NotEqual(t, policy.KeyOf(id, vote), policy.KeyOf(id, &other))

	other = *vote
	other.PolicyVersion = 5
	require.NotEqual(t, policy.KeyOf(id, vote), policy.KeyOf(id, &other))

	other = *vote
	other.Observation.Outcome = sinkv1.OutcomeVoided
	require.NotEqual(t, policy.KeyOf(id, vote), policy.KeyOf(id, &other))

	other = *vote
	other.Observation.Slot++
	require.Equal(t, policy.KeyOf(id, vote), policy.KeyOf(id, &other))

	require.Equal(t,
		policy.Key{ID: id.String(), Kind: policy.MsgVeto},
		policy.KeyOf(id, &sinkv1.MsgVetoVoid{Signer: reporter, ID: id.Proto(), Reason: "x"}))
	require.Equal(t,
		policy.Key{ID: id.String(), Kind: policy.MsgRefund},
		policy.KeyOf(id, &sinkv1.MsgFinalizeRefund{Executor: reporter, ID: id.Proto()}))

	require.Equal(t, 1, int(policy.MsgVote))
	require.Equal(t, 2, int(policy.MsgVeto))
	require.Equal(t, 3, int(policy.MsgRefund))
}

// keysOf returns the keys of the messages the reporter plans for the observation in the state.
func keysOf(t *testing.T, s policy.Snapshot, o policy.Observed) []policy.Key {
	t.Helper()

	id, err := sinkid.FromProto(s.Transfer.ID)
	require.NoError(t, err)

	messages := plan(t, reporter, s, o).Messages
	keys := make([]policy.Key, 0, len(messages))

	for _, m := range messages {
		keys = append(keys, policy.KeyOf(id, m))
	}

	return keys
}

func allObserved() map[string]policy.Observed {
	return map[string]policy.Observed{
		"absent":   absent(),
		"minted":   resolved(sinkv1.OutcomeMinted),
		"voided":   resolved(sinkv1.OutcomeVoided),
		"absent+":  {Kind: policy.KindAbsent, ContextSlot: 5000},
		"minted+":  {Kind: policy.KindResolved, ContextSlot: 5000, Observation: observation(sinkv1.OutcomeMinted)},
		"voided++": {Kind: policy.KindResolved, ContextSlot: 9000, Observation: observation(sinkv1.OutcomeVoided)},
	}
}

func requirePlans(t *testing.T, key policy.Key, s policy.Snapshot, o policy.Observed) {
	t.Helper()
	require.Contains(t, keysOf(t, s, o), key, "the state before execution must plan the message")
}

func requireNoKey(t *testing.T, key policy.Key, s policy.Snapshot, name string) {
	t.Helper()

	for label, o := range allObserved() {
		require.NotContains(t, keysOf(t, s, o), key, "%s: %s plans an executed message", name, label)
	}
}

func TestNoReplanAfterExecution(t *testing.T) {
	id := testID(t, 1).String()
	voteKey := func(outcome sinkv1.Outcome, version uint64) policy.Key {
		return policy.Key{ID: id, Kind: policy.MsgVote, Outcome: outcome, Round: startRound, PolicyVersion: version}
	}

	t.Run("recorded vote", func(t *testing.T) {
		key := voteKey(sinkv1.OutcomeMinted, 1)
		pre := testSnapshot(t)
		requirePlans(t, key, pre, resolved(sinkv1.OutcomeMinted))

		post := testSnapshot(t)
		addVote(&post, reporter, 1, sinkv1.OutcomeMinted)
		requireNoKey(t, key, post, "recorded")

		minted := post
		addVote(&minted, "b", 1, sinkv1.OutcomeMinted)
		addVote(&minted, "c", 1, sinkv1.OutcomeMinted)
		minted.Transfer.Status = sinkv1.StatusMinted
		requireNoKey(t, key, minted, "minted")

		conflict := post
		addVote(&conflict, "b", 1, sinkv1.OutcomeVoided)
		conflict.Transfer.Settlement.Hold = sinkv1.HoldConflict
		requireNoKey(t, key, conflict, "conflict")
	})

	t.Run("vote without effect under HOLD_CONFLICT", func(t *testing.T) {
		key := voteKey(sinkv1.OutcomeMinted, 1)
		pre := testSnapshot(t)
		addVote(&pre, "b", 1, sinkv1.OutcomeVoided)
		requirePlans(t, key, pre, resolved(sinkv1.OutcomeMinted))

		post := pre
		addVote(&post, "c", 1, sinkv1.OutcomeMinted)
		post.Transfer.Settlement.Hold = sinkv1.HoldConflict
		requireNoKey(t, key, post, "conflict")
	})

	t.Run("vote without effect on a minted transfer", func(t *testing.T) {
		key := voteKey(sinkv1.OutcomeMinted, 1)
		pre := testSnapshot(t)
		addVote(&pre, "b", 1, sinkv1.OutcomeMinted)
		addVote(&pre, "c", 1, sinkv1.OutcomeMinted)
		requirePlans(t, key, pre, resolved(sinkv1.OutcomeMinted))

		post := pre
		addVote(&post, "d", 1, sinkv1.OutcomeMinted)
		post.Transfer.Status = sinkv1.StatusMinted
		requireNoKey(t, key, post, "minted")
	})

	t.Run("vote without effect on a refund-pending transfer", func(t *testing.T) {
		key := voteKey(sinkv1.OutcomeVoided, 1)
		pre := testSnapshot(t)
		addVote(&pre, "b", 1, sinkv1.OutcomeVoided)
		addVote(&pre, "c", 1, sinkv1.OutcomeVoided)
		requirePlans(t, key, pre, resolved(sinkv1.OutcomeVoided))

		post := pre
		addVote(&post, "d", 1, sinkv1.OutcomeVoided)
		post.Transfer.Status = sinkv1.StatusRefundPending
		post.Transfer.Settlement.ChallengeEnd = post.Time + 100
		requireNoKey(t, key, post, "refund pending")

		later := post
		later.Time += 1000
		requireNoKey(t, key, later, "refund pending after the window")
	})

	t.Run("vote without effect on a held transfer", func(t *testing.T) {
		key := voteKey(sinkv1.OutcomeVoided, 1)
		pre := testSnapshot(t)
		addVote(&pre, "b", 1, sinkv1.OutcomeVoided)
		requirePlans(t, key, pre, resolved(sinkv1.OutcomeVoided))

		post := pre
		addVote(&post, "c", 1, sinkv1.OutcomeVoided)
		addVote(&post, "d", 1, sinkv1.OutcomeVoided)
		post.Transfer.Status = sinkv1.StatusRefundPending
		post.Transfer.Settlement.ChallengeEnd = post.Time + 100
		post.Transfer.Settlement.Hold = sinkv1.HoldVeto
		post.Transfer.Settlement.VetoBy = "d"
		post.Transfer.Settlement.VetoPolicyVersion = 1
		requireNoKey(t, key, post, "held")
	})

	t.Run("refresh vote after which the settlement policy version is current", func(t *testing.T) {
		key := voteKey(sinkv1.OutcomeVoided, 2)
		pre := voided(t)
		pre.Policy.Version = 2
		requirePlans(t, key, pre, resolved(sinkv1.OutcomeVoided))

		post := pre
		post.Transfer.Status = sinkv1.StatusTransferPending
		post.Transfer.Settlement.Policy = post.Policy
		post.Transfer.Settlement.Votes = nil
		post.Transfer.Settlement.ChallengeEnd = 0
		addVote(&post, reporter, 2, sinkv1.OutcomeVoided)
		requireNoKey(t, key, post, "refreshed")

		later := post
		addVote(&later, "b", 2, sinkv1.OutcomeVoided)
		addVote(&later, "c", 2, sinkv1.OutcomeVoided)
		later.Transfer.Status = sinkv1.StatusRefundPending
		later.Transfer.Settlement.ChallengeEnd = later.Time + 100
		requireNoKey(t, key, later, "refresh then quorum")
	})

	t.Run("executed veto", func(t *testing.T) {
		key := policy.Key{ID: id, Kind: policy.MsgVeto}
		pre := testSnapshot(t)
		pre.Transfer.Status = sinkv1.StatusRefundPending
		pre.Transfer.Settlement.ChallengeEnd = pre.Time + 100
		requirePlans(t, key, pre, absent())

		post := pre
		post.Transfer.Settlement.Hold = sinkv1.HoldVeto
		post.Transfer.Settlement.VetoBy = reporter
		post.Transfer.Settlement.VetoReason = "No Solana status at finalized slot 110"
		post.Transfer.Settlement.VetoPolicyVersion = 1
		requireNoKey(t, key, post, "held by veto")

		later := post
		later.Time += 1000
		requireNoKey(t, key, later, "veto after the window")

		conflict := post
		conflict.Transfer.Settlement.Hold = sinkv1.HoldConflict
		requireNoKey(t, key, conflict, "veto under conflict")
	})

	t.Run("executed refund", func(t *testing.T) {
		key := policy.Key{ID: id, Kind: policy.MsgRefund}
		pre := voided(t)
		requirePlans(t, key, pre, resolved(sinkv1.OutcomeVoided))

		post := pre
		post.Transfer.Status = sinkv1.StatusRefunded
		post.Transfer.Settlement.ChallengeEnd = 0
		post.RefundUsage.Amount = sdk.NewInt64Coin(uakt, 60)
		requireNoKey(t, key, post, "refunded")

		headless := post
		headless.RefundHead = nil
		requireNoKey(t, key, headless, "refunded without head")

		later := post
		later.Time += 86400
		later.Epoch++
		later.RefundUsage = sinkv1.EpochUsage{Epoch: later.Epoch, Amount: sdk.NewInt64Coin(uakt, 0)}
		requireNoKey(t, key, later, "refunded in a later epoch")
	})
}

func TestVetoAfterGovernanceResolution(t *testing.T) {
	round1 := testSnapshot(t)
	round1.Transfer.Status = sinkv1.StatusRefundPending
	round1.Transfer.Settlement.ChallengeEnd = round1.Time + 100

	// The reporter's veto in round 1.
	first := plan(t, reporter, round1, absent())
	requireMessages(t, []string{vetoMsg}, first)

	id, err := sinkid.FromProto(round1.Transfer.ID)
	require.NoError(t, err)

	round1Key := policy.KeyOf(id, first.Messages[0])
	require.Equal(t, policy.Key{ID: id.String(), Kind: policy.MsgVeto}, round1Key)

	// The veto executes.
	vetoed := round1
	vetoed.Transfer.Settlement.Hold = sinkv1.HoldVeto
	vetoed.Transfer.Settlement.VetoBy = reporter
	vetoed.Transfer.Settlement.VetoReason = "No Solana status at finalized slot 110"
	vetoed.Transfer.Settlement.VetoPolicyVersion = 1
	require.Empty(t, plan(t, reporter, vetoed, absent()).Messages)

	// Governance resolves the transfer: round 2, no hold, refund-pending with a new challenge end.
	round2 := vetoed
	round2.Time += 500
	round2.Transfer.Status = sinkv1.StatusRefundPending
	round2.Transfer.Settlement.Round = 2
	round2.Transfer.Settlement.Hold = sinkv1.HoldNone
	round2.Transfer.Settlement.Votes = nil
	// MsgConfirmVoid is the resolution that leaves a transfer refund-pending, and it always carries evidence.
	governance := observation(sinkv1.OutcomeVoided)
	round2.Transfer.Settlement.GovernanceObservation = &governance
	round2.Transfer.Settlement.VetoBy, round2.Transfer.Settlement.VetoReason, round2.Transfer.Settlement.VetoPolicyVersion = "", "", 0
	round2.Transfer.Settlement.ChallengeEnd = round2.Time + 100
	require.Greater(t, round2.Transfer.Settlement.ChallengeEnd, round2.Time)

	second := plan(t, reporter, round2, absent())
	requireMessages(t, []string{vetoMsg}, second)
	require.Contains(t, second.Alerts, policy.AlertNoStatus)
	require.Equal(t, round1Key, policy.KeyOf(id, second.Messages[0]))
}
