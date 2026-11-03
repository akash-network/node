package app_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	commitment "github.com/cosmos/ibc-go/v10/modules/core/23-commitment/types"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/app"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

func TestSinkFullAppGenesisBankRestoration(t *testing.T) {
	h := reportingHarness(t)
	native := h.quotaEpoch(t)
	native.Duration = 4 * h.params().ReporterChallengePeriod
	native.StartTime = h.now.Add(-56 * native.Duration)
	native.CurrentEpoch = 57
	native.CurrentEpochStartTime = h.now
	native.CurrentEpochStartHeight = 1
	native.EpochCountingStarted = true
	h.setQuotaEpoch(t, native)

	for i := 0; i < 3; i++ {
		res := h.deliver(t, "", h.deposit(10))
		require.Zero(t, res.Code, res.Log)
	}

	h.quorum(t, 1, types.OutcomeMinted)
	h.quorum(t, 2, types.OutcomeVoided)
	h.now = time.Unix(h.transfer(t, 2).Settlement.ChallengeEnd, 0).UTC()
	res := h.refund(t, 2)
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 3, types.OutcomeVoided)
	require.NoError(t, h.a.Keepers.Akash.Sink.VetoVoid(h.ctx(), &types.MsgVetoVoid{
		Signer: h.reporterAddress(0),
		ID:     h.id(3),
		Reason: "recovery hold",
	}))

	res = h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	res = h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	h.quorum(t, 5, types.OutcomeVoided)
	h.commitBlock(t, time.Unix(h.transfer(t, 5).Settlement.ChallengeEnd, 0).UTC())
	p := h.params()
	p.MaxAmountPerTransfer = coin(10)
	p.MaxEpochAmountPerAccount = coin(50)
	p.MaxTransferAmountPerEpoch = coin(50)
	p.MaxEpochRefund = coin(10)
	require.NoError(t, h.update(p))
	h.commitBlock(t, h.now.Add(time.Second))
	require.Equal(t, uint64(57), h.state().Epoch)
	res = h.deliver(t, "", h.deposit(1))
	require.Equal(t, types.ErrAccountDailyAmountExceeded.ABCICode(), res.Code)
	res = h.refund(t, 5)
	require.Equal(t, types.ErrRefundCapReached.ABCICode(), res.Code)
	before, err := h.a.Keepers.Akash.Sink.ExportGenesis(h.ctx())
	require.NoError(t, err)
	escrow := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), sinkkeeper.Address())
	epochsBefore, err := h.a.Keepers.Akash.Epochs.ExportGenesis(h.ctx())
	require.NoError(t, err)
	stateBefore := h.state()
	key, err := sinkkeeper.TransferKey(h.id(1))
	require.NoError(t, err)
	record := bytes.Clone(h.ctx().KVStore(h.a.GetKey(types.StoreKey)).Get(key))
	proofReply, err := h.a.Query(context.Background(), &abci.RequestQuery{
		Path:   "/store/sink/key",
		Data:   key,
		Height: h.height,
		Prove:  true,
	})
	require.NoError(t, err)
	require.Zero(t, proofReply.Code, proofReply.Log)
	proof, err := commitment.ConvertProofs(proofReply.ProofOps)
	require.NoError(t, err)
	require.NoError(t, proof.VerifyMembership(commitment.GetSDKSpecs(), commitment.NewMerkleRoot(h.a.CommitMultiStore().LastCommitID().Hash), commitment.NewMerklePath([]byte("sink"), key), record))
	exported, err := h.a.ExportAppStateAndValidators(false, nil, nil)
	require.NoError(t, err)
	require.Greater(t, exported.Height, h.height)
	root := filepath.Join("..", ".cache", "sink-review-corrections", "app-homes")
	require.NoError(t, os.MkdirAll(root, 0o700))
	home, err := os.MkdirTemp(root, "sink-genesis-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(home)) })

	restored := app.Setup(app.WithHome(home), app.WithCheckTx(true))
	t.Cleanup(func() { require.NoError(t, restored.Close()) })
	_, err = restored.InitChain(&abci.RequestInitChain{
		ChainId:       "akash-1",
		Time:          h.now,
		InitialHeight: exported.Height,
		AppStateBytes: exported.AppState,
	})
	require.NoError(t, err)
	ctx := restored.NewContext(false)
	after, err := restored.Keepers.Akash.Sink.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	epochsAfter, err := restored.Keepers.Akash.Epochs.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Equal(t, epochsBefore, epochsAfter)
	stateAfter, err := restored.Keepers.Akash.Sink.NewQuerier().State(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, stateBefore, stateAfter)
	require.Equal(t, record, ctx.KVStore(restored.GetKey(types.StoreKey)).Get(key))
	require.Equal(t, escrow, restored.Keepers.Cosmos.Bank.GetAllBalances(ctx, sinkkeeper.Address()))
	clone := *h
	clone.a = restored
	clone.height = 0
	clone.ids = append([]types.ID(nil), h.ids...)
	bz := clone.tx(t, "", clone.deposit(1))
	clone.height = exported.Height - 1
	res = clone.deliverBytes(t, bz)
	require.Equal(t, types.ErrAccountDailyAmountExceeded.ABCICode(), res.Code)
	res = clone.refund(t, 5)
	require.Equal(t, types.ErrRefundCapReached.ABCICode(), res.Code)
	clone.commitBlock(t, native.CurrentEpochStartTime.Add(native.Duration))
	require.Equal(t, uint64(57), clone.state().Epoch)
	clone.commitBlock(t, native.CurrentEpochStartTime.Add(native.Duration+time.Nanosecond))
	require.Equal(t, uint64(58), clone.state().Epoch)
	res = clone.deliver(t, "", clone.deposit(10))
	require.Zero(t, res.Code, res.Log)
	id := clone.ids[len(clone.ids)-1]
	require.GreaterOrEqual(t, id.Height, exported.Height)
	require.Equal(t, uint64(1), id.Sequence)
	res = clone.refund(t, 5)
	require.Zero(t, res.Code, res.Log)
	require.NoError(t, restored.Keepers.Akash.Sink.CheckBalance(clone.ctx()))
	proofReply, err = restored.Query(context.Background(), &abci.RequestQuery{
		Path:   "/store/sink/key",
		Data:   key,
		Height: clone.height,
		Prove:  true,
	})
	require.NoError(t, err)
	require.Zero(t, proofReply.Code, proofReply.Log)
	require.Equal(t, record, proofReply.Value)
	proof, err = commitment.ConvertProofs(proofReply.ProofOps)
	require.NoError(t, err)
	require.NoError(t, proof.VerifyMembership(commitment.GetSDKSpecs(), commitment.NewMerkleRoot(restored.CommitMultiStore().LastCommitID().Hash), commitment.NewMerklePath([]byte("sink"), key), record))
}
