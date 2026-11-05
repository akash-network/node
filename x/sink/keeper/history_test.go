package keeper

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

func historyMembers() []string {
	members := make([]string, 0, 32)
	for i := 0; i < 32; i++ {
		members = append(members, sdk.AccAddress(bytes.Repeat([]byte{byte(100 + i)}, 20)).String())
	}

	return members
}

func settlementIn(status types.Status) func(types.ReporterPolicy) types.GenesisRecord {
	return func(p types.ReporterPolicy) types.GenesisRecord {
		s := types.SettlementData{
			Round:  1,
			Policy: p,
		}
		switch status {
		case types.StatusMinted:
			s.Votes = quorum(p, types.OutcomeMinted)
		case types.StatusRefundPending, types.StatusRefunded:
			s.Votes = quorum(p, types.OutcomeVoided)
		}

		if status == types.StatusRefundPending {
			deadline := time.Unix(86400*21, 0).UTC()
			s.ChallengeEnd = deadline.Unix()
		}

		if status == types.StatusVoided {
			s.VoidAuthorization = &types.VoidAuthorization{
				RequestedHeight: 7,
				RequestedRound:  1,
			}
		}

		return types.GenesisRecord{
			Status:     status,
			Settlement: s,
		}
	}
}

// historyGenesis retains policies identical 32-member versions and minted
// terminal transfers behind one transfer in first's state.
func historyGenesis(policies uint64, first func(types.ReporterPolicy) types.GenesisRecord, amount int64, minted int) *types.GenesisState {
	g := types.DefaultGenesis()
	g.Params.Reporters = historyMembers()
	g.Params.MaxEpochRefund = testCoin(2_000_000_000_000)
	g.Policies = nil

	for v := uint64(1); v <= policies; v++ {
		g.Policies = append(g.Policies, types.ReporterPolicy{
			Version:   v,
			Members:   g.Params.Reporters,
			Threshold: g.Params.ReportersThreshold,
		})
	}

	current := g.Policies[len(g.Policies)-1]
	g.PolicySequence = current.Version
	owner := testOwner(6).String()
	total := types.ZeroAmount()
	add := func(sequence uint64, amount int64, record types.GenesisRecord) {
		g.Records = append(g.Records, types.GenesisRecord{
			ID: types.ID{
				Owner:    owner,
				Height:   7,
				Sequence: sequence,
			},
			Data: types.RecordData{
				Dest:   types.SolanaAddress{1}.String(),
				Amount: testCoin(amount),
			},
			Settlement: record.Settlement,
			Status:     record.Status,
		})

		if err := g.Totals.Add(record.Status, testCoin(amount)); err != nil {
			panic(err)
		}

		total = total.Add(testCoin(amount))
	}

	add(1, amount, first(current))

	for i := 0; i < minted; i++ {
		add(uint64(i+2), 10, settlementIn(types.StatusMinted)(current))
	}

	g.Accounts = []types.GenesisAccount{{
		Owner: owner,
		Usage: types.AccountUsage{
			TotalSent: total,
			EpochSent: total,
			LastEpoch: 19,
		},
	}}
	g.SendUsage = types.EpochUsage{
		Epoch:  19,
		Amount: total,
	}
	return g
}

func TestSettlementWritesRejectMissingPolicies(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := historyGenesis(1, settlementIn(types.StatusTransferPending), 10, 0)
	importTestGenesis(t, k, ctx, bank, g)
	transfer, err := k.GetTransfer(ctx, g.Records[0].ID)
	require.NoError(t, err)
	missing := transfer
	missing.Settlement.Policy.Version = 2
	err = k.put(ctx, &transfer, missing)
	require.ErrorIs(t, err, types.ErrInvalidSettlement)
	require.ErrorContains(t, err, "missing policy version 2")
	outsider := transfer
	outsider.Settlement.Votes = []types.Vote{{
		Reporter:      g.Policies[0].Members[0],
		PolicyVersion: 2,
		Observation: types.Observation{
			Outcome:              types.OutcomeMinted,
			Slot:                 testEvidence().Slot,
			TransactionSignature: testEvidence().TransactionSignature,
		},
	}}
	err = k.put(ctx, &transfer, outsider)
	require.ErrorIs(t, err, types.ErrInvalidSettlement)
	require.ErrorContains(t, err, "missing policy version 2")
	unchanged, err := k.GetTransfer(ctx, transfer.ID)
	require.NoError(t, err)
	require.Equal(t, transfer, unchanged)
}

func TestSettlementWriteGasIgnoresRetainedPolicies(t *testing.T) {
	report := func(policies uint64) uint64 {
		k, ctx, bank := testStore(t)
		g := historyGenesis(policies, settlementIn(types.StatusTransferPending), 10, 0)
		importTestGenesis(t, k, ctx, bank, g)
		current := g.Policies[len(g.Policies)-1]
		ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
		require.NoError(t, k.ReportOutcome(ctx, &types.MsgReportOutcome{
			Reporter:      current.Members[0],
			ID:            g.Records[0].ID,
			PolicyVersion: current.Version,
			Round:         1,
			Observation: types.Observation{
				Outcome:              types.OutcomeMinted,
				Slot:                 testEvidence().Slot,
				TransactionSignature: testEvidence().TransactionSignature,
			},
		}))

		return ctx.GasMeter().GasConsumed()
	}

	one, fifty := report(1), report(50)
	t.Logf("report gas with 1 and 50 retained policies: %d / %d", one, fifty)
	require.Equal(t, one, fifty, "a report must not pay for unreferenced policy versions")
}

func TestRefundCapReductionChecksOnlyUnpaidTransfers(t *testing.T) {
	for _, tc := range []struct {
		status   types.Status
		rejected bool
	}{
		{types.StatusTransferPending, true},
		{types.StatusVoided, true},
		{types.StatusRefundPending, true},
		{types.StatusMinted, false},
		{types.StatusRefunded, false},
	} {
		for _, owner := range []sdk.AccAddress{testOwner(6), make(sdk.AccAddress, 20), sdk.AccAddress(bytes.Repeat([]byte{6}, 32)), sdk.AccAddress(bytes.Repeat([]byte{6}, 255))} {
			t.Run(fmt.Sprintf("%s/%d/%d", tc.status, len(owner), owner[0]), func(t *testing.T) {
				k, ctx, bank := testStore(t)
				g := historyGenesis(1, settlementIn(tc.status), 1_000_000_000_000, 0)
				g.Records[0].ID.Owner = owner.String()
				g.Accounts[0].Owner = owner.String()
				importTestGenesis(t, k, ctx, bank, g)
				p := g.Params
				p.MaxAmountPerTransfer = testCoin(999_999_999_999)
				p.MaxEpochRefund = testCoin(999_999_999_999)
				err := k.SetParams(ctx, p)
				if tc.rejected {
					require.ErrorContains(t, err, "refund cap below unpaid transfer amount")
					return
				}

				require.NoError(t, err)
			})
		}
	}
}

func TestRefundCapReductionGasIgnoresTerminalHistory(t *testing.T) {
	lower := func(minted int) uint64 {
		k, ctx, bank := testStore(t)
		g := historyGenesis(1, settlementIn(types.StatusTransferPending), 10, minted)
		importTestGenesis(t, k, ctx, bank, g)
		p := g.Params
		p.MaxEpochRefund = testCoin(1_000_000_000_000)
		ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())

		err := k.SetParams(ctx, p)
		require.NoError(t, err)

		return ctx.GasMeter().GasConsumed()
	}

	empty, populated := lower(0), lower(2000)
	t.Logf("refund cap reduction gas with 0 and 2000 terminal records: %d / %d", empty, populated)
	require.Equal(t, empty, populated, "lowering the refund cap must not scan terminal transfers")
}
