package app_test

import (
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/node/v2/app"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

func TestSinkFullAppGenesisBankRestoration(t *testing.T) {
	h := reportingHarness(t)
	for i := 0; i < 3; i++ {
		res := h.deliver(t, "", h.deposit(10))
		require.Zero(t, res.Code, res.Log)
	}

	h.quorum(t, 1, types.OutcomeMinted)
	h.quorum(t, 2, types.OutcomeVoided)
	h.now = *h.settlement(t, 2).ChallengeEnd
	res := h.refund(t, 2)
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 3, types.OutcomeVoided)
	require.NoError(t, h.a.Keepers.Akash.Sink.VetoVoid(h.ctx(), &types.MsgVetoVoid{Signer: h.reporterAddress(0), ID: h.id(3), Reason: "recovery hold"}))
	res = h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	before, err := h.a.Keepers.Akash.Sink.ExportGenesis(h.ctx())
	require.NoError(t, err)
	escrow := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), sinkkeeper.Address())
	exported, err := h.a.ExportAppStateAndValidators(false, nil, nil)
	require.NoError(t, err)
	require.Greater(t, exported.Height, h.height)
	restored := app.Setup(app.WithHome(t.TempDir()), app.WithCheckTx(true))
	t.Cleanup(func() { require.NoError(t, restored.Close()) })
	_, err = restored.InitChain(&abci.RequestInitChain{ChainId: "akash-1", Time: h.now, InitialHeight: exported.Height, AppStateBytes: exported.AppState})
	require.NoError(t, err)
	ctx := restored.NewContext(false)
	after, err := restored.Keepers.Akash.Sink.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, escrow, restored.Keepers.Cosmos.Bank.GetAllBalances(ctx, sinkkeeper.Address()))
	clone := *h
	clone.a = restored
	clone.height = 0
	clone.ids = append([]types.ID(nil), h.ids...)
	bz := clone.tx(t, "", clone.deposit(10))
	clone.height = exported.Height - 1
	res = clone.deliverBytes(t, bz)
	require.Zero(t, res.Code, res.Log)
	id := clone.ids[len(clone.ids)-1]
	require.Equal(t, exported.Height, id.Height)
	require.Equal(t, uint64(1), id.Sequence)
	require.NoError(t, restored.Keepers.Akash.Sink.CheckBalance(clone.ctx()))
}
