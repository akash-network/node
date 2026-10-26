package app_test

// Regression tests for the findings of the 2026-10-22 x/sink audit.

import (
	"bytes"
	"context"
	"encoding/binary"
	"strconv"
	"testing"
	"time"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

// refundQueue returns the IDs in the derived refund queue, head first.
func (h *sinkHarness) refundQueue(t *testing.T) []uint64 {
	t.Helper()
	prefix := []byte("refundq/")
	it := storetypes.KVStorePrefixIterator(h.ctx().KVStore(h.a.GetKey(types.StoreKey)), prefix)
	defer func() { require.NoError(t, it.Close()) }()
	ids := []uint64{}
	for ; it.Valid(); it.Next() {
		ids = append(ids, binary.BigEndian.Uint64(it.Key()[len(prefix)+8:]))
	}
	return ids
}

func clearSinkStore(t *testing.T, h *sinkHarness, ctx sdk.Context) {
	t.Helper()
	s := ctx.KVStore(h.a.GetKey(types.StoreKey))
	it := storetypes.KVStorePrefixIterator(s, nil)
	var keys [][]byte
	for ; it.Valid(); it.Next() {
		keys = append(keys, bytes.Clone(it.Key()))
	}
	require.NoError(t, it.Close())
	for _, key := range keys {
		s.Delete(key)
	}
}

// M-02: refunds are paid oldest first, so a small refund paid at the start of
// a day cannot starve a cap-sized one.
func TestSinkRefundsPayOldestFirst(t *testing.T) {
	h := reportingHarness(t) // max_transfer and daily refund cap are both 100
	k := h.a.Keepers.Akash.Sink
	res := h.deliver(t, "", h.deposit(100))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 1, types.Outcome_OUTCOME_VOIDED)
	h.now = h.now.Add(time.Hour)
	res = h.deliver(t, "", h.deposit(1))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 2, types.Outcome_OUTCOME_VOIDED)
	require.Equal(t, []uint64{1, 2}, h.refundQueue(t))

	// Both have matured at the start of a later day.
	day := h.settlement(t, 2).ChallengeEnd/86400 + 1
	h.now = time.Unix(day*86400, 0)
	res = h.refund(t, 2)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "must wait for older refund 1")
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, []uint64{2}, h.refundQueue(t))
	res = h.refund(t, 2)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "daily refund cap") // the cap still bounds each day
	h.now = h.now.Add(24 * time.Hour)
	res = h.refund(t, 2)
	require.Zero(t, res.Code, res.Log)
	require.Empty(t, h.refundQueue(t))
	require.Equal(t, uint64(101), k.GetTotals(h.ctx()).RefundedAmount)
	h.checkSettlementExport(t)
}

// M-02: holds, governance confirmations and reporter rotations keep the queue
// in step with the settlements.
func TestSinkRefundQueueFollowsSettlement(t *testing.T) {
	h := reportingHarness(t)
	k := h.a.Keepers.Akash.Sink
	gov := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	for id := uint64(1); id <= 2; id++ {
		res := h.deliver(t, "", h.deposit(40))
		require.Zero(t, res.Code, res.Log)
		h.quorum(t, id, types.Outcome_OUTCOME_VOIDED)
		h.now = h.now.Add(time.Hour)
	}
	require.Equal(t, []uint64{1, 2}, h.refundQueue(t))

	// A veto holds 1 and takes it out of the queue, so 2 is next.
	member := sdk.AccAddress(h.reporters[3].PubKey().Address()).String()
	res := h.deliverBytes(t, h.txFrom(t, h.reporters[3], "", &types.MsgVetoVoid{Signer: member, Id: 1, Reason: "disputed"}))
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, []uint64{2}, h.refundQueue(t))
	h.now = time.Unix(h.settlement(t, 2).ChallengeEnd, 0)
	res = h.refund(t, 2)
	require.Zero(t, res.Code, res.Log)

	// Governance confirms 1 again, and it rejoins with a new window.
	require.NoError(t, k.RequestVoid(h.ctx(), gov, 1))
	require.NoError(t, h.resolve(t, 1, types.Decision_DECISION_CONFIRM_VOID))
	require.Equal(t, []uint64{1}, h.refundQueue(t))
	h.checkSettlementExport(t)

	// A rotation leaves 1 stale. It blocks the queue until a current reporter
	// reports on it, which returns it to pending and out of the queue.
	p := k.GetParams(h.ctx())
	p.Reporters[0] = sdk.AccAddress(h.reporters[4].PubKey().Address()).String()
	require.NoError(t, k.SetParams(h.ctx(), p))
	h.now = time.Unix(h.settlement(t, 1).ChallengeEnd, 0)
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "stale void authorization")
	res = h.report(t, 1, 1, types.Outcome_OUTCOME_VOIDED)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, types.Status_STATUS_PENDING, h.settlement(t, 1).Status)
	require.Empty(t, h.refundQueue(t))
	h.checkSettlementExport(t)
}

// M-04 and L-02: a mint quorum that votes before Solana resolves closes the
// transfer at once. Governance can now order a void and reopen it, so the
// sender is refunded.
func TestSinkGovernanceReopensContestedMint(t *testing.T) {
	h := reportingHarness(t)
	k := h.a.Keepers.Akash.Sink
	gov := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 1, types.Outcome_OUTCOME_MINTED)
	require.Equal(t, types.Status_STATUS_MINTED, h.settlement(t, 1).Status)
	require.Zero(t, k.GetTotals(h.ctx()).PendingCount)
	// Solana voided it; the honest reporter's vote can only hold it.
	res = h.report(t, 3, 1, types.Outcome_OUTCOME_VOIDED)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, types.Hold_HOLD_CONFLICT, h.settlement(t, 1).Hold)

	require.ErrorContains(t, h.resolve(t, 1, types.Decision_DECISION_CONFIRM_VOID), "void order")
	require.NoError(t, k.RequestVoid(h.ctx(), gov, 1))
	require.NoError(t, h.resolve(t, 1, types.Decision_DECISION_CONFIRM_VOID))
	s := h.settlement(t, 1)
	require.Equal(t, types.Status_STATUS_VOIDED, s.Status)
	require.Equal(t, types.Hold_HOLD_NONE, s.Hold)
	require.Equal(t, uint64(2), s.Round)
	totals := k.GetTotals(h.ctx())
	require.Zero(t, totals.MintedAmount)
	require.Equal(t, uint64(60), totals.VoidedAmount)
	require.Equal(t, uint64(1), totals.PendingCount)
	require.Equal(t, uint32(1), k.GetAccountUsage(h.ctx(), h.sender).PendingCount)
	require.Contains(t, h.checkOpenIndex(t, h.ctx()), uint64(1))
	require.Equal(t, []uint64{1}, h.refundQueue(t))
	h.checkSettlementExport(t)

	before := h.a.Keepers.Cosmos.Bank.GetBalance(h.ctx(), h.sender, "uakt")
	h.now = time.Unix(s.ChallengeEnd, 0)
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, before.Add(sdk.NewInt64Coin("uakt", 60)), h.a.Keepers.Cosmos.Bank.GetBalance(h.ctx(), h.sender, "uakt"))
	require.Equal(t, types.Status_STATUS_REFUNDED, h.settlement(t, 1).Status)
	h.checkSettlementExport(t)

	// An uncontested mint stays closed.
	res = h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 2, types.Outcome_OUTCOME_MINTED)
	require.ErrorContains(t, k.RequestVoid(h.ctx(), gov, 2), "already closed")
	// A reopened refund must fit the daily cap, or it would block the queue.
	res = h.report(t, 3, 2, types.Outcome_OUTCOME_VOIDED)
	require.Zero(t, res.Code, res.Log)
	require.NoError(t, k.RequestVoid(h.ctx(), gov, 2))
	p := k.GetParams(h.ctx())
	p.MaxTransfer, p.DailyRefundCap = 50, 50
	require.NoError(t, k.SetParams(h.ctx(), p)) // 2 is minted, not open
	require.ErrorContains(t, h.resolve(t, 2, types.Decision_DECISION_CONFIRM_VOID), "exceeds the daily refund cap")
}

// L-04 and L-03: a lowered refund cap must cover every open transfer, and the
// challenge floor applies to every parameter update.
func TestSinkParameterFloors(t *testing.T) {
	h := reportingHarness(t)
	k := h.a.Keepers.Akash.Sink
	res := h.deliver(t, "", h.deposit(100))
	require.Zero(t, res.Code, res.Log)
	p := k.GetParams(h.ctx())
	p.MaxTransfer, p.DailyRefundCap = 50, 50
	require.ErrorContains(t, k.SetParams(h.ctx(), p), "below open transfer amount 100")
	p.DailyRefundCap = 100
	require.NoError(t, k.SetParams(h.ctx(), p)) // max_transfer may drop; the cap may not
	h.quorum(t, 1, types.Outcome_OUTCOME_MINTED)
	p.DailyRefundCap = 50
	require.NoError(t, k.SetParams(h.ctx(), p)) // nothing is open any more
	p.ChallengePeriodSeconds = types.MinChallengePeriodSeconds - 1
	require.ErrorContains(t, k.SetParams(h.ctx(), p), "challenge period")
}

// I-09: a send restriction added after the sink's cannot redirect a refund.
func TestSinkRefundMustReachSender(t *testing.T) {
	h := reportingHarness(t)
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 1, types.Outcome_OUTCOME_VOIDED)
	h.now = time.Unix(h.settlement(t, 1).ChallengeEnd, 0)
	redirect := true
	h.a.Keepers.Cosmos.Bank.AppendSendRestriction(func(_ context.Context, from, to sdk.AccAddress, _ sdk.Coins) (sdk.AccAddress, error) {
		if redirect && from.Equals(sinkkeeper.Address()) {
			return h.bystander, nil
		}
		return to, nil
	})
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "did not reach the original sender")
	require.Equal(t, types.Status_STATUS_VOIDED, h.settlement(t, 1).Status)
	redirect = false
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	h.checkSettlementExport(t)
}

// I-08 and I-10: the deposit event names the recipient and height, and
// genesis rejects day counters from the future.
func TestSinkDepositEventAndGenesisDays(t *testing.T) {
	h := reportingHarness(t)
	k := h.a.Keepers.Akash.Sink
	res := h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	attrs := map[string]string{}
	for _, event := range res.Events {
		if event.Type == "sink_deposit" {
			for _, a := range event.Attributes {
				attrs[a.Key] = a.Value
			}
		}
	}
	entry, ok := k.GetTransfer(h.ctx(), 1)
	require.True(t, ok)
	transfer, err := types.DecodeTransfer(entry.Record)
	require.NoError(t, err)
	require.Equal(t, base58.Encode(h.recipient[:]), attrs["recipient"])
	require.Equal(t, strconv.FormatUint(transfer.Height, 10), attrs["height"])

	ctx := h.ctx()
	today := uint64(ctx.BlockTime().Unix() / 86400)
	for _, mutate := range []func(*types.GenesisState){
		func(g *types.GenesisState) { g.Totals.RefundDay = today + 1 },
		func(g *types.GenesisState) { g.Accounts[0].Day = today + 1 },
	} {
		g := k.ExportGenesis(ctx)
		mutate(&g)
		cache, _ := ctx.CacheContext()
		clearSinkStore(t, h, cache)
		require.ErrorContains(t, k.InitGenesis(cache, g), "in the future")
	}
}

// L-01: withdraw addresses set to the sink before it existed are cleared.
func TestSinkUpgradeClearsSinkWithdrawAddresses(t *testing.T) {
	h, ctx := preSinkLedger(t)
	distr := h.a.Keepers.Cosmos.Distr
	other := sdk.AccAddress(h.reporters[0].PubKey().Address())
	// The old binary does not block the sink address yet.
	require.NoError(t, distr.SetDelegatorWithdrawAddr(ctx, h.sender, sinkkeeper.Address()))
	require.NoError(t, distr.SetDelegatorWithdrawAddr(ctx, other, h.bystander))
	require.NoError(t, runSinkUpgrade(t, h, ctx))
	got, err := distr.GetDelegatorWithdrawAddr(ctx, h.sender)
	require.NoError(t, err)
	require.Equal(t, h.sender, got) // rewards go to the delegator again
	got, err = distr.GetDelegatorWithdrawAddr(ctx, other)
	require.NoError(t, err)
	require.Equal(t, h.bystander, got) // unrelated records stay
	var reset []string
	for _, event := range ctx.EventManager().Events() {
		if event.Type == "sink_withdraw_address_reset" {
			reset = append(reset, event.Attributes[0].Value)
		}
	}
	require.Equal(t, []string{h.sender.String()}, reset)
}
