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

func TestSinkDailyAmountPerAccount(t *testing.T) {
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
	dayStart := h.now.UTC().Truncate(24 * time.Hour)
	h.now = dayStart.Add(24*time.Hour - 2*time.Second)
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
	require.Error(t, h.update(p))
	p.MaxEpochAmountPerAccount = coin(110)
	require.NoError(t, h.update(p))
	res = h.deliver(t, "", h.deposit(11))
	require.NotZero(t, res.Code)
	res = h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, coin(110), h.usage(h.sender).EpochSent)
	ctx := h.ctx().WithBlockTime(dayStart)
	_, err := h.a.Keepers.Akash.Sink.NewQuerier().Account(ctx, &types.QueryAccountRequest{Owner: h.sender.String()})
	require.Error(t, err)
	future := h.ctx().WithBlockTime(h.now.Add(24 * time.Hour))
	q := h.a.Keepers.Akash.Sink.NewQuerier()
	a, err := q.Account(future, &types.QueryAccountRequest{Owner: h.sender.String()})
	require.NoError(t, err)
	require.True(t, a.Usage.EpochSent.IsZero())
	require.Equal(t, coin(110), h.usage(h.sender).EpochSent)
	h.checkSettlementExport(t)
}
