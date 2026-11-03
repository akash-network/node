package keeper

import (
	"bytes"
	"testing"
	"time"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

func historyMembers() []string {
	members := make([]string, 0, types.MaxReporters)
	for i := 0; i < types.MaxReporters; i++ {
		members = append(members, sdk.AccAddress(bytes.Repeat([]byte{byte(100 + i)}, 20)).String())
	}

	return members
}

func settlementIn(status types.Status) func(types.ReporterPolicy) types.SettlementData {
	return func(p types.ReporterPolicy) types.SettlementData {
		s := types.SettlementData{Status: status, Round: 1, PolicyVersion: p.Version}
		switch status {
		case types.StatusMinted:
			s.Votes = quorum(p, types.OutcomeMinted)
		case types.StatusRefundPending, types.StatusRefunded:
			s.Votes = quorum(p, types.OutcomeVoided)
		}

		if status == types.StatusRefundPending {
			deadline := time.Unix(86400*21, 0).UTC()
			s.ChallengeEnd = &deadline
		}

		return s
	}
}

// historyGenesis retains policies identical 32-member versions and minted
// terminal transfers behind one transfer in first's state.
func historyGenesis(policies int, first func(types.ReporterPolicy) types.SettlementData, amount int64, minted int) *types.GenesisState {
	g := types.DefaultGenesis()
	g.Params.Reporters = historyMembers()
	g.Params.MaxEpochRefund = testCoin(2_000_000_000_000)
	g.Policies = nil
	for v := 1; v <= policies; v++ {
		g.Policies = append(g.Policies, g.Params.Policy(uint64(v)))
	}

	current := g.Policies[len(g.Policies)-1]
	owner := testOwner(6).String()
	total := types.ZeroAmount()
	add := func(sequence uint64, amount int64, s types.SettlementData) {
		g.Transfers = append(g.Transfers, types.TransferState{
			ID:         types.ID{Owner: owner, Height: 7, Sequence: sequence},
			Data:       types.RecordData{Dest: types.SolanaAddress{1}.String(), Amount: testCoin(amount)},
			Settlement: s,
		})

		if err := g.Totals.Add(s.Status, testCoin(amount)); err != nil {
			panic(err)
		}

		total = total.Add(testCoin(amount))
	}

	add(1, amount, first(current))

	for i := 0; i < minted; i++ {
		add(uint64(i+2), 10, settlementIn(types.StatusMinted)(current))
	}

	g.Accounts = []types.GenesisAccount{{Owner: owner, Usage: types.AccountUsage{TotalSent: total, EpochSent: total, LastEpoch: 19}}}
	g.SendUsage = types.EpochUsage{Epoch: 19, Amount: total}
	return g
}

func TestSettlementWritesRejectMissingPolicies(t *testing.T) {
	k, ctx, bank := testStore(t)
	g := historyGenesis(1, settlementIn(types.StatusTransferPending), 10, 0)
	importTestGenesis(t, k, ctx, bank, g)
	transfer, err := k.GetTransfer(ctx, g.Transfers[0].ID)
	require.NoError(t, err)
	missing := transfer.Settlement
	missing.PolicyVersion = 2
	err = k.put(ctx, transfer.ID, transfer.Data.Amount, &transfer.Settlement, missing)
	require.ErrorIs(t, err, types.ErrInvalidSettlement)
	require.ErrorContains(t, err, "missing policy version 2")
	outsider := transfer.Settlement
	outsider.Votes = []types.Vote{{Reporter: g.Policies[0].Members[0], PolicyVersion: 2, Observation: types.Observation{Outcome: types.OutcomeMinted, Evidence: testEvidence()}}}
	err = k.put(ctx, transfer.ID, transfer.Data.Amount, &transfer.Settlement, outsider)
	require.ErrorIs(t, err, types.ErrInvalidSettlement)
	require.ErrorContains(t, err, "missing policy version 2")
	unchanged, err := k.GetTransfer(ctx, transfer.ID)
	require.NoError(t, err)
	require.Equal(t, transfer, unchanged)
}

func TestSettlementWriteGasIgnoresRetainedPolicies(t *testing.T) {
	report := func(policies int) uint64 {
		k, ctx, bank := testStore(t)
		g := historyGenesis(policies, settlementIn(types.StatusTransferPending), 10, 0)
		importTestGenesis(t, k, ctx, bank, g)
		current := g.Policies[len(g.Policies)-1]
		ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
		require.NoError(t, k.ReportOutcome(ctx, &types.MsgReportOutcome{
			Reporter:      current.Members[0],
			ID:            g.Transfers[0].ID,
			PolicyVersion: current.Version,
			Round:         1,
			Observation:   types.Observation{Outcome: types.OutcomeMinted, Evidence: testEvidence()},
		}))

		return ctx.GasMeter().GasConsumed()
	}

	require.Equal(t, report(1), report(50), "a report must not pay for unreferenced policy versions")
}

func TestRefundCapReductionChecksOnlyUnpaidTransfers(t *testing.T) {
	for _, tc := range []struct {
		status   types.Status
		rejected bool
	}{
		{types.StatusTransferPending, true},
		{types.StatusRefundPending, true},
		{types.StatusMinted, false},
		{types.StatusRefunded, false},
	} {
		t.Run(tc.status.String(), func(t *testing.T) {
			k, ctx, bank := testStore(t)
			g := historyGenesis(1, settlementIn(tc.status), 1_000_000_000_000, 0)
			importTestGenesis(t, k, ctx, bank, g)
			p := g.Params
			p.MaxAmountPerTransfer = testCoin(999_999_999_999)
			p.MaxEpochRefund = testCoin(999_999_999_999)
			err := k.UpdateParams(ctx, &types.MsgUpdateParams{Authority: authtypes.NewModuleAddress("gov").String(), Params: p})
			if tc.rejected {
				require.ErrorContains(t, err, "refund cap below unpaid transfer amount")
				return
			}

			require.NoError(t, err)
		})
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
		require.NoError(t, k.UpdateParams(ctx, &types.MsgUpdateParams{Authority: authtypes.NewModuleAddress("gov").String(), Params: p}))

		return ctx.GasMeter().GasConsumed()
	}

	require.Equal(t, lower(0), lower(2000), "lowering the refund cap must not scan terminal transfers")
}
