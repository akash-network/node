package app_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

func clearSinkStore(t *testing.T, h *sinkHarness, ctx sdk.Context) {
	t.Helper()
	s := ctx.KVStore(h.a.GetKey(types.StoreKey))
	it := s.Iterator(nil, nil)
	var keys [][]byte
	for ; it.Valid(); it.Next() {
		keys = append(keys, bytes.Clone(it.Key()))
	}

	require.NoError(t, it.Close())

	for _, key := range keys {
		s.Delete(key)
	}
}

func storeEntries(t *testing.T, s storetypes.KVStore, prefix []byte) map[string]string {
	t.Helper()
	it := storetypes.KVStorePrefixIterator(s, prefix)
	defer func() { require.NoError(t, it.Close()) }()
	result := map[string]string{}
	for ; it.Valid(); it.Next() {
		result[string(it.Key())] = string(it.Value())
	}

	return result
}

func TestSinkRefundsPayOldestFirst(t *testing.T) {
	h := reportingHarness(t)
	for _, amount := range []int64{100, 10} {
		res := h.deliver(t, "", h.deposit(amount))
		require.Zero(t, res.Code, res.Log)
		h.quorum(t, len(h.ids), types.OutcomeVoided)
	}

	h.now = time.Unix(h.transfer(t, 2).Settlement.ChallengeEnd, 0).UTC()
	h.catchUpQuotaEpoch(t)
	res := h.refund(t, 2)
	require.NotZero(t, res.Code)
	require.Equal(t, h.id(1), *h.state().RefundHead)
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	res = h.refund(t, 2)
	require.NotZero(t, res.Code)
	h.now = h.now.Add(24 * time.Hour)
	res = h.refund(t, 2)
	require.Zero(t, res.Code, res.Log)
	require.Nil(t, h.state().RefundHead)
}

func TestSinkRefundQueueFollowsSettlement(t *testing.T) {
	h := reportingHarness(t)
	for i := 0; i < 2; i++ {
		res := h.deliver(t, "", h.deposit(10))
		require.Zero(t, res.Code, res.Log)
		h.quorum(t, i+1, types.OutcomeVoided)
	}

	require.Equal(t, h.id(1), *h.state().RefundHead)
	require.NoError(t, h.a.Keepers.Akash.Sink.VetoVoid(h.ctx(), &types.MsgVetoVoid{Signer: h.reporterAddress(0), ID: h.id(1), Reason: "review"}))
	require.Equal(t, h.id(2), *h.state().RefundHead)
	rotate(h)
	require.Nil(t, h.state().RefundHead)
	require.NoError(t, h.order(h.id(1)))
	require.NoError(t, h.resolve(t, h.id(1), types.DecisionConfirmVoid))
	require.Equal(t, h.id(1), *h.state().RefundHead)
	h.now = time.Unix(h.transfer(t, 1).Settlement.ChallengeEnd, 0).UTC()
	res := h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	require.Nil(t, h.state().RefundHead)
	h.checkSettlementExport(t)
}

func TestSinkGovernanceReopensContestedMint(t *testing.T) {
	h := reportingHarness(t)
	res := h.deliver(t, "", h.deposit(100))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 1, types.OutcomeMinted)
	require.Error(t, h.order(h.id(1)))
	require.Error(t, h.resolve(t, h.id(1), types.DecisionConfirmVoid))
	res = h.report(t, 3, 1, types.OutcomeVoided)
	require.Zero(t, res.Code, res.Log)
	p := h.params()
	p.MaxAmountPerTransfer = coin(50)
	p.MaxEpochRefund = coin(50)
	require.NoError(t, h.update(p))
	require.NoError(t, h.order(h.id(1)))
	require.Error(t, h.resolve(t, h.id(1), types.DecisionConfirmVoid))
	p.MaxEpochRefund = coin(100)
	require.NoError(t, h.update(p))
	require.NoError(t, h.resolve(t, h.id(1), types.DecisionConfirmVoid))
	require.True(t, h.totals().MintedAmount.IsZero())
	require.Zero(t, h.totals().MintedCount)
	require.Equal(t, coin(100), h.totals().RefundPendingAmount)
	h.now = time.Unix(h.transfer(t, 1).Settlement.ChallengeEnd, 0).UTC()
	res = h.refund(t, 1)
	require.Zero(t, res.Code, res.Log)
	h.checkSettlementExport(t)
}

func TestSinkParameterFloors(t *testing.T) {
	h := reportingHarness(t)
	res := h.deliver(t, "", h.deposit(100))
	require.Zero(t, res.Code, res.Log)
	p := h.params()
	p.MaxAmountPerTransfer = coin(50)
	p.MaxEpochRefund = coin(50)
	require.Error(t, h.update(p))
	p = h.params()
	p.ReporterChallengePeriod = time.Duration(types.MinChallengePeriodSeconds)*time.Second - time.Nanosecond
	require.Error(t, h.update(p))
	h.quorum(t, 1, types.OutcomeVoided)
	deadline := h.transfer(t, 1).Settlement.ChallengeEnd
	p = h.params()
	p.ReporterChallengePeriod += time.Hour
	require.NoError(t, h.update(p))
	require.Equal(t, deadline, h.transfer(t, 1).Settlement.ChallengeEnd)
}

func TestSinkRefundMustReachSender(t *testing.T) {
	h := reportingHarness(t)
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 1, types.OutcomeVoided)
	h.now = time.Unix(h.transfer(t, 1).Settlement.ChallengeEnd, 0).UTC()
	h.a.Keepers.Cosmos.Bank.AppendSendRestriction(func(_ context.Context, from, to sdk.AccAddress, _ sdk.Coins) (sdk.AccAddress, error) {
		if from.Equals(sinkkeeper.Address()) {
			return h.bystander, nil
		}

		return to, nil
	})

	before := captureSinkBankState(t, h, h.ctx())
	ctx := h.ctx().WithBlockTime(h.now)
	events := ctx.EventManager().Events()
	err := h.a.Keepers.Akash.Sink.FinalizeRefund(ctx, &types.MsgFinalizeRefund{Executor: h.sender.String(), ID: h.id(1)})
	require.ErrorContains(t, err, "original sender")
	require.Equal(t, before, captureSinkBankState(t, h, h.ctx()))
	require.Equal(t, events, ctx.EventManager().Events())
	h.checkSettlementExport(t)
}

func TestSinkDepositEventAndGenesisDays(t *testing.T) {
	h := newSinkHarness(t, nil)
	res := h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	require.Contains(t, sinkEvents(t, res.Events), proto.Message(&types.EventTransferCreated{ID: h.id(1), Sender: h.sender.String(), Recipient: h.deposit(10).Dest, Amount: coin(10), Height: 1}))
	k := h.a.Keepers.Akash.Sink
	g, err := k.ExportGenesis(h.ctx())
	require.NoError(t, err)
	cache, _ := h.ctx().CacheContext()
	clearSinkStore(t, h, cache)
	require.Error(t, k.InitGenesis(cache, g))
	cache = cache.WithBlockHeight(h.height + 1)
	g.SendUsage.Epoch += 100
	require.Error(t, k.InitGenesis(cache, g))
}

func TestSinkUpgradeClearsSinkWithdrawAddresses(t *testing.T) {
	h, ctx := preSinkLedger(t)
	d := h.a.Keepers.Cosmos.Distr
	require.NoError(t, d.SetDelegatorWithdrawAddr(ctx, h.sender, sinkkeeper.Address()))
	require.NoError(t, runSinkUpgrade(t, h, ctx))
	withdraw, err := d.GetDelegatorWithdrawAddr(ctx, h.sender)
	require.NoError(t, err)
	require.Equal(t, h.sender, withdraw)
	require.Contains(t, ctx.EventManager().Events(), sdk.NewEvent("sink_withdraw_address_reset", sdk.NewAttribute("delegator", h.sender.String())))
	require.Equal(t, types.DefaultParams(), sinkParams(t, h.a.Keepers.Akash.Sink, ctx))
	require.Equal(t, authtypes.NewModuleAddress(types.ModuleName), sinkkeeper.Address())
}
