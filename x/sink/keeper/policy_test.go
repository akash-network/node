package keeper

import (
	"math"
	"slices"
	"strings"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

func TestDecimalThresholdSettlement(t *testing.T) {
	for _, tc := range []struct {
		threshold string
		required  int
	}{
		{
			threshold: "0.7",
			required:  5,
		},
		{
			threshold: "0.75",
			required:  6,
		},
		{
			threshold: "0.714285714285714285",
			required:  5,
		},
		{
			threshold: "0.714285714285714286",
			required:  6,
		},
		{
			threshold: "1",
			required:  7,
		},
	} {
		for _, outcome := range []types.Outcome{types.OutcomeMinted, types.OutcomeVoided} {
			t.Run(tc.threshold+"/"+outcome.String(), func(t *testing.T) {
				k, ctx, bank := testStore(t)
				g := types.DefaultGenesis()
				for n := byte(1); n <= 7; n++ {
					g.Params.Reporters = append(g.Params.Reporters, testOwner(n).String())
				}

				g.Params.ReportersThreshold = sdkmath.LegacyMustNewDecFromStr(tc.threshold)
				g.PolicySequence = 1
				policy := types.ReporterPolicy{
					Version:   1,
					Members:   g.Params.Reporters,
					Threshold: g.Params.ReportersThreshold,
				}

				g.Policies = []types.ReporterPolicy{policy}
				id := types.ID{
					Owner:    testOwner(8).String(),
					Height:   7,
					Sequence: 1,
				}

				g.Records = []types.GenesisRecord{
					{
						ID:     id,
						Status: types.StatusTransferPending,
						Data: types.RecordData{
							Dest:   types.SolanaAddress{1}.String(),
							Amount: testCoin(10),
						},
						Settlement: types.SettlementData{
							Round:  1,
							Policy: policy,
						},
					},
				}
				g.Totals.PendingAmount = testCoin(10)
				g.Totals.PendingCount = 1
				g.SendUsage = types.EpochUsage{
					Epoch:  19,
					Amount: testCoin(10),
				}
				g.Accounts = []types.GenesisAccount{
					{
						Owner: id.Owner,
						Usage: types.AccountUsage{
							TotalSent: testCoin(10),
							EpochSent: testCoin(10),
							LastEpoch: 19,
						},
					},
				}

				votes := quorum(policy, outcome)
				require.Len(t, votes, tc.required)
				importTestGenesis(t, k, ctx, bank, g)
				for i, vote := range votes {
					msg := &types.MsgReportOutcome{
						ID:            id,
						Round:         1,
						Reporter:      vote.Reporter,
						PolicyVersion: vote.PolicyVersion,
						Observation:   vote.Observation,
					}
					require.NoError(t, msg.ValidateBasic())
					require.NoError(t, k.ReportOutcome(ctx, msg))
					got, err := k.GetTransfer(ctx, id)
					require.NoError(t, err)
					require.Len(t, got.Settlement.Votes, i+1)
					switch {
					case i+1 < tc.required:
						require.Equal(t, types.StatusTransferPending, got.Status)
						require.Zero(t, got.Settlement.ChallengeEnd)
					case outcome == types.OutcomeMinted:
						require.Equal(t, types.StatusMinted, got.Status)
						require.Zero(t, got.Settlement.ChallengeEnd)
					default:
						require.Equal(t, types.StatusRefundPending, got.Status)
						require.Equal(t, ctx.BlockTime().Add(g.Params.ReporterChallengePeriod).Unix(), got.Settlement.ChallengeEnd)
					}
				}

				before, err := k.ExportGenesis(ctx)
				require.NoError(t, err)
				events := ctx.EventManager().Events()
				vote := votes[0]
				require.NoError(t, k.ReportOutcome(ctx, &types.MsgReportOutcome{
					ID:            id,
					Round:         1,
					Reporter:      policy.Members[len(policy.Members)-1],
					PolicyVersion: policy.Version,
					Observation:   vote.Observation,
				}))
				after, err := k.ExportGenesis(ctx)
				require.NoError(t, err)
				require.Equal(t, before, after)
				require.Equal(t, events, ctx.EventManager().Events())

				restored, restoreCtx, restoreBank := testStore(t)
				importTestGenesis(t, restored, restoreCtx, restoreBank, after)
				roundTrip, err := restored.ExportGenesis(restoreCtx)
				require.NoError(t, err)
				require.Equal(t, after, roundTrip)
				require.Equal(t, bank.GetAllBalances(ctx, Address()), restoreBank.GetAllBalances(restoreCtx, Address()))
			})
		}
	}
}

func TestDecimalPolicyRotation(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := populatedGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	p := g.Params
	p.Reporters = slices.Clone(p.Reporters)
	for _, tc := range []struct {
		name    string
		change  func()
		version uint64
	}{
		{
			name:    "unchanged",
			change:  func() {},
			version: 2,
		},
		{
			name:    "reordered",
			change:  func() { slices.Reverse(p.Reporters) },
			version: 2,
		},
		{
			name:    "different fraction with same integer quorum",
			change:  func() { p.ReportersThreshold = sdkmath.LegacyMustNewDecFromStr("0.7") },
			version: 3,
		},
		{
			name:    "different membership",
			change:  func() { p.Reporters[0] = testOwner(8).String() },
			version: 4,
		},
		{
			name:    "unrelated parameter",
			change:  func() { p.RefundsEnabled = !p.RefundsEnabled },
			version: 4,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.change()
			require.NoError(t, p.ValidateBasic())
			require.NoError(t, k.SetParams(ctx, p))
			current, err := k.GetCurrentPolicy(ctx)
			require.NoError(t, err)
			require.Equal(t, tc.version, current.Version)
			require.True(t, p.ReportersThreshold.Equal(current.Threshold))
			require.ElementsMatch(t, p.Reporters, current.Members)
			sequence, err := k.policySequence.Peek(ctx)
			require.NoError(t, err)
			require.Equal(t, current.Version, sequence)
		})
	}

	for _, expected := range g.Policies {
		retained, err := k.policies.Get(ctx, expected.Version)
		require.NoError(t, err)
		require.Equal(t, expected, retained)
	}
}

func TestPolicyBootstrapAndOverflow(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := types.DefaultGenesis()
	importTestGenesis(t, k, ctx, bank, g)
	exported, err := k.ExportGenesis(ctx)
	require.NoError(t, err)
	expectedBytes, err := g.Marshal()
	require.NoError(t, err)
	actualBytes, err := exported.Marshal()
	require.NoError(t, err)
	require.Equal(t, expectedBytes, actualBytes)
	require.Empty(t, exported.Policies)
	require.Zero(t, exported.PolicySequence)

	p := g.Params
	p.Reporters = []string{testOwner(1).String()}
	require.NoError(t, p.ValidateBasic())
	require.NoError(t, k.SetParams(ctx, p))
	policy, err := k.GetCurrentPolicy(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(1), policy.Version)
	require.NoError(t, policy.Validate())

	policy.Version = math.MaxUint64
	require.NoError(t, k.policies.Set(ctx, policy.Version, policy))
	require.NoError(t, k.policySequence.Set(ctx, policy.Version))
	p.ReportersThreshold = sdkmath.LegacyOneDec()
	require.ErrorIs(t, k.SetParams(ctx, p), types.ErrOverflow)
	current, err := k.GetCurrentPolicy(ctx)
	require.NoError(t, err)
	require.Equal(t, policy, current)
	stored, err := k.GetParams(ctx)
	require.NoError(t, err)
	require.False(t, p.ReportersThreshold.Equal(stored.ReportersThreshold))
}

func TestSetParamsRejectsInvalidReporterSet(t *testing.T) {
	members := func(n int) []string {
		out := make([]string, 0, n)
		for i := 1; i <= n; i++ {
			out = append(out, testOwner(byte(i)).String())
		}

		return out
	}

	for _, tc := range []struct {
		name      string
		reporters []string
		threshold string
	}{
		{name: "three at default ratio", reporters: members(3), threshold: "0.66"},
		{name: "six at default ratio", reporters: members(6), threshold: "0.66"},
		{name: "nine at default ratio", reporters: members(9), threshold: "0.66"},
		{name: "exactly two thirds", reporters: members(3), threshold: "0.666666666666666666"},
		{name: "zero ratio", reporters: members(4), threshold: "0"},
		{name: "duplicate member", reporters: append(members(3), members(1)...), threshold: "0.75"},
		// Bech32 also decodes the upper-case spelling. Four strings at 0.66
		// need three votes, which two accounts would cast.
		{name: "second spelling of a member", reporters: append(members(3), strings.ToUpper(members(1)[0])), threshold: "0.66"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k, ctx, bank := testStore(t)
			g := populatedGenesis()
			importTestGenesis(t, k, ctx, bank, g)
			before, err := k.ExportGenesis(ctx)
			require.NoError(t, err)

			p := g.Params
			p.Reporters = tc.reporters
			p.ReportersThreshold = sdkmath.LegacyMustNewDecFromStr(tc.threshold)
			require.ErrorIs(t, k.SetParams(ctx, p), types.ErrInvalidReporters)

			// Nothing was stored, so policy history stays valid and exportable.
			after, err := k.ExportGenesis(ctx)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}

	for _, tc := range []struct {
		name      string
		reporters []string
		threshold string
	}{
		// A reporter key that was never funded has no account yet.
		{name: "member without an account", reporters: []string{members(3)[0], members(3)[1], testOwner(200).String(), members(4)[3]}, threshold: "0.75"},
		{name: "just above two thirds", reporters: members(3), threshold: "0.666666666666666667"},
		{name: "seven at default ratio", reporters: members(7), threshold: "0.66"},
		{name: "empty set", reporters: nil, threshold: "0.66"},
		{name: "upper-case member", reporters: append([]string{strings.ToUpper(members(1)[0])}, members(4)[1:]...), threshold: "0.75"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k, ctx, bank := testStore(t)
			g := populatedGenesis()
			importTestGenesis(t, k, ctx, bank, g)

			p := g.Params
			p.Reporters = tc.reporters
			p.ReportersThreshold = sdkmath.LegacyMustNewDecFromStr(tc.threshold)
			require.NoError(t, k.SetParams(ctx, p))

			current, err := k.GetCurrentPolicy(ctx)
			require.NoError(t, err)
			require.Equal(t, uint64(3), current.Version)
			require.ElementsMatch(t, tc.reporters, current.Members)
			_, err = k.ExportGenesis(ctx)
			require.NoError(t, err)
		})
	}
}

// A module account cannot run a reporter, so listing one would only shrink the
// set that can reach quorum. The check decodes the address, so the upper-case
// spelling is rejected as well.
func TestSetParamsRejectsModuleAccountReporter(t *testing.T) {
	for _, spelling := range []func(string) string{
		func(s string) string { return s },
		strings.ToUpper,
	} {
		k, ctx, bank := testStore(t)
		g := populatedGenesis()
		importTestGenesis(t, k, ctx, bank, g)
		module := k.accounts.GetModuleAccount(ctx, "distribution").GetAddress().String()
		before, err := k.ExportGenesis(ctx)
		require.NoError(t, err)

		p := g.Params
		p.Reporters = []string{testOwner(1).String(), testOwner(2).String(), testOwner(3).String(), spelling(module)}
		p.ReportersThreshold = sdkmath.LegacyMustNewDecFromStr("0.75")
		err = k.SetParams(ctx, p)
		require.ErrorIs(t, err, types.ErrInvalidReporters)
		require.ErrorContains(t, err, "module account")

		after, err := k.ExportGenesis(ctx)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
}
