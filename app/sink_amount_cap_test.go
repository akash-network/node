package app_test

import (
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

func (h *sinkHarness) depositFrom(t *testing.T, key *secp256k1.PrivKey, amount int64) *abci.ExecTxResult {
	return h.deliverBytes(t, h.txFrom(t, key, "", &types.MsgSink{DestType: types.DestTypeSolana, Dest: base58.Encode(h.recipient[:]), Amount: coin(amount), Sender: sdk.AccAddress(key.PubKey().Address()).String()}))
}

func TestSinkUnlimitedAccountRetainsUsage(t *testing.T) {
	h := newSinkHarness(t, func(g *types.GenesisState) {
		g.Params.MaxAmountPerTransfer = coin(10)
		g.Params.MaxEpochAmountPerAccount = coin(0)
		g.Params.MaxTransferAmountPerEpoch = coin(25)
	})

	for range 2 {
		res := h.deliver(t, "", h.deposit(10))
		require.Zero(t, res.Code, res.Log)
	}

	require.Equal(t, coin(20), h.usage(h.sender).EpochSent)
	p := h.params()
	p.MaxEpochAmountPerAccount = coin(20)
	require.NoError(t, h.update(p))
	res := h.deliver(t, "", h.deposit(1))
	require.NotZero(t, res.Code)
	require.Equal(t, coin(20), h.usage(h.sender).EpochSent)

	p.MaxEpochAmountPerAccount = coin(0)
	require.NoError(t, h.update(p))
	res = h.deliver(t, "", h.deposit(5))
	require.Zero(t, res.Code, res.Log)
	res = h.depositFrom(t, h.reporters[4], 1)
	require.NotZero(t, res.Code)
	require.Equal(t, coin(25), h.state().SendUsage.Amount)
	require.Equal(t, coin(25), h.usage(h.sender).EpochSent)
}

func TestSinkEpochAmountPerAccount(t *testing.T) {
	h := newSinkHarness(t, func(g *types.GenesisState) {
		g.Params.MinAmountPerTransfer = coin(10)
		g.Params.MaxAmountPerTransfer = coin(100)
		g.Params.MaxEpochAmountPerAccount = coin(100)
		g.Params.MaxTransferAmountPerEpoch = coin(200)
	})

	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	res = h.deliver(t, "", h.deposit(41))
	require.NotZero(t, res.Code)
	require.Equal(t, coin(60), h.usage(h.sender).EpochSent)
	res = h.deliver(t, "", h.deposit(40))
	require.Zero(t, res.Code, res.Log)
	res = h.depositFrom(t, h.reporters[4], 100)
	require.Zero(t, res.Code, res.Log)
	res = h.depositFrom(t, h.reporters[3], 10)
	require.NotZero(t, res.Code)
	require.Equal(t, coin(200), h.state().SendUsage.Amount)
	epoch := h.quotaEpoch(t)
	end := epoch.CurrentEpochStartTime.Add(epoch.Duration)
	h.now = end.Add(-time.Second)
	res = h.deliver(t, "", h.deposit(10))
	require.NotZero(t, res.Code)
	res = h.deliver(t, "", h.deposit(100))
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, coin(200), h.usage(h.sender).TotalSent)
	require.Equal(t, coin(100), h.usage(h.sender).EpochSent)
	p := h.params()
	p.MaxAmountPerTransfer = coin(50)
	p.MaxEpochAmountPerAccount = coin(50)
	require.NoError(t, h.update(p))
	res = h.deliver(t, "", h.deposit(10))
	require.NotZero(t, res.Code)
	p.MaxEpochAmountPerAccount = coin(0)
	require.NoError(t, h.update(p))
	p.MaxEpochAmountPerAccount = coin(110)
	require.NoError(t, h.update(p))
	res = h.deliver(t, "", h.deposit(11))
	require.NotZero(t, res.Code)
	res = h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, coin(110), h.usage(h.sender).EpochSent)
	ctx := h.ctx().WithBlockTime(epoch.StartTime)
	_, err := h.a.Keepers.Akash.Sink.NewQuerier().Account(ctx, &types.QueryAccountRequest{Owner: h.sender.String()})
	require.NoError(t, err)
	future := h.ctx().WithBlockTime(h.now.Add(24 * time.Hour))
	q := h.a.Keepers.Akash.Sink.NewQuerier()
	a, err := q.Account(future, &types.QueryAccountRequest{Owner: h.sender.String()})
	require.NoError(t, err)
	require.Equal(t, coin(110), a.Usage.EpochSent)
	require.Equal(t, coin(110), h.usage(h.sender).EpochSent)
	h.checkSettlementExport(t)
}
