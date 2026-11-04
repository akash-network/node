package sink_test

import (
	"context"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/stretchr/testify/require"

	epochstypes "pkg.akt.dev/go/node/epochs/v1beta1"
	types "pkg.akt.dev/go/node/sink/v1"
)

func (h *testSuite) quotaEpoch(t *testing.T) epochstypes.EpochInfo {
	t.Helper()
	epoch, err := h.a.Keepers.Akash.Epochs.GetEpoch(h.ctx(), "day")
	require.NoError(t, err)

	return epoch
}

func (h *testSuite) setQuotaEpoch(t *testing.T, epoch epochstypes.EpochInfo) {
	t.Helper()
	require.NoError(t, h.a.Keepers.Akash.Epochs.RemoveEpoch(h.ctx(), "day"))
	require.NoError(t, h.a.Keepers.Akash.Epochs.AddEpoch(h.ctx(), epoch))
}

func (h *testSuite) catchUpQuotaEpoch(t *testing.T) {
	t.Helper()

	for range 1000 {
		epoch := h.quotaEpoch(t)
		if !h.now.After(epoch.CurrentEpochStartTime.Add(epoch.Duration)) {
			return
		}

		h.commitBlock(t, h.now.Add(time.Nanosecond))
	}

	t.Fatal("native epoch did not catch up within 1000 blocks")
}

func TestSinkNativeEpochBoundaries(t *testing.T) {
	for _, timestamp := range []string{"2026-11-02T10:03:17Z", "2026-11-02T23:57:17Z"} {
		t.Run(timestamp, func(t *testing.T) {
			h := setupTestSuite(t, func(p *types.Params) {
				p.MaxAmountPerTransfer = coin(10)
				p.MaxEpochAmountPerAccount = coin(10)
				p.MaxTransferAmountPerEpoch = coin(10)
			})

			start, err := time.Parse(time.RFC3339, timestamp)
			require.NoError(t, err)
			epoch := epochstypes.NewGenesisEpochInfo("day", 10*time.Minute)
			epoch.StartTime = start
			h.setQuotaEpoch(t, epoch)
			require.Zero(t, h.state().Epoch)
			res := h.commitBlock(t, start.Add(-time.Nanosecond), h.tx(t, "", h.deposit(1)))[0]
			require.Equal(t, types.ErrInvalidClock.ABCICode(), res.Code)
			require.Empty(t, h.ids)
			require.Zero(t, h.state().Epoch)
			res = h.commitBlock(t, start, h.tx(t, "", h.deposit(10)))[0]
			require.Zero(t, res.Code, res.Log)
			require.Equal(t, int64(1), h.quotaEpoch(t).CurrentEpoch)
			require.Equal(t, uint64(1), h.state().Epoch)
			require.Equal(t, uint64(1), h.id(1).Sequence)
			exhaustedHeight := h.height

			if start.Hour() == 23 {
				midnight := start.Truncate(24 * time.Hour).Add(24 * time.Hour)
				res = h.commitBlock(t, midnight, h.tx(t, "", h.deposit(1)))[0]
				require.NotZero(t, res.Code)
				require.Equal(t, uint64(1), h.state().Epoch)
			}

			end := start.Add(epoch.Duration)
			res = h.commitBlock(t, end, h.tx(t, "", h.deposit(1)))[0]
			require.NotZero(t, res.Code)
			require.Equal(t, uint64(1), h.state().Epoch)
			other := h.deposit(1)
			other.Sender = h.reporterAddress(4)
			res = h.commitBlock(t, end, h.txFrom(t, h.reporters[4], "", other))[0]
			require.NotZero(t, res.Code)
			require.Equal(t, types.ErrSendCapReached.ABCICode(), res.Code)
			h.commitBlock(t, end.Add(time.Nanosecond))
			require.Equal(t, int64(2), h.quotaEpoch(t).CurrentEpoch)
			before, err := h.a.Keepers.Akash.Sink.ExportGenesis(h.ctx())
			require.NoError(t, err)
			require.Equal(t, types.EpochUsage{
				Epoch:  1,
				Amount: coin(10),
			}, before.SendUsage)
			require.Equal(t, types.EpochUsage{
				Epoch:  2,
				Amount: coin(0),
			}, h.state().SendUsage)
			account, err := h.a.Keepers.Akash.Sink.NewQuerier().Account(h.ctx(), &types.QueryAccountRequest{Owner: h.sender.String()})
			require.NoError(t, err)
			require.Equal(t, uint64(2), account.Epoch)
			require.True(t, account.Usage.EpochSent.IsZero())
			after, err := h.a.Keepers.Akash.Sink.ExportGenesis(h.ctx())
			require.NoError(t, err)
			require.Equal(t, before, after)

			stateReply, err := h.a.Query(context.Background(), &abci.RequestQuery{
				Path:   "/akash.sink.v1.Query/State",
				Height: exhaustedHeight,
			})
			require.NoError(t, err)
			require.Zero(t, stateReply.Code, stateReply.Log)
			var state types.QueryStateResponse
			require.NoError(t, state.Unmarshal(stateReply.Value))
			require.Equal(t, types.EpochUsage{
				Epoch:  1,
				Amount: coin(10),
			}, state.SendUsage)
			request, err := (&epochstypes.QueryCurrentEpochRequest{Identifier: "day"}).Marshal()
			require.NoError(t, err)
			nativeReply, err := h.a.Query(context.Background(), &abci.RequestQuery{
				Path:   "/akash.epochs.v1beta1.Query/CurrentEpoch",
				Data:   request,
				Height: exhaustedHeight,
			})
			require.NoError(t, err)
			require.Zero(t, nativeReply.Code, nativeReply.Log)
			var native epochstypes.QueryCurrentEpochResponse
			require.NoError(t, native.Unmarshal(nativeReply.Value))
			require.EqualValues(t, native.CurrentEpoch, state.Epoch)
			res = h.deliver(t, "", h.deposit(10))
			require.Zero(t, res.Code, res.Log)
			require.Equal(t, uint64(1), h.id(2).Sequence)
			require.Greater(t, h.id(2).Height, h.id(1).Height)
		})
	}
}

func TestSinkNativeEpochDowntime(t *testing.T) {
	h := setupTestSuite(t, func(p *types.Params) {
		p.MaxAmountPerTransfer = coin(10)
		p.MaxEpochAmountPerAccount = coin(10)
		p.MaxTransferAmountPerEpoch = coin(10)
	})

	start := time.Date(2026, 11, 2, 10, 3, 17, 0, time.UTC)
	epoch := epochstypes.EpochInfo{
		ID:                      "day",
		StartTime:               start.Add(-36 * 10 * time.Minute),
		Duration:                10 * time.Minute,
		CurrentEpoch:            37,
		CurrentEpochStartTime:   start,
		CurrentEpochStartHeight: 1,
		EpochCountingStarted:    true,
	}

	h.setQuotaEpoch(t, epoch)
	res := h.commitBlock(t, start, h.tx(t, "", h.deposit(10)))[0]
	require.Zero(t, res.Code, res.Log)

	for i := int64(1); i <= 3; i++ {
		res = h.commitBlock(t, start.Add(37*time.Minute+time.Duration(i)*time.Nanosecond), h.tx(t, "", h.deposit(10)))[0]
		require.Zero(t, res.Code, res.Log)
		require.Equal(t, int64(37)+i, h.quotaEpoch(t).CurrentEpoch)
		require.Equal(t, uint64(37+i), h.state().Epoch)
		require.Equal(t, start.Add(time.Duration(i)*10*time.Minute), h.quotaEpoch(t).CurrentEpochStartTime)
		require.Equal(t, coin(10), h.state().SendUsage.Amount)
		require.Equal(t, uint64(1), h.ids[len(h.ids)-1].Sequence)
	}

	res = h.deliver(t, "", h.deposit(1))
	require.NotZero(t, res.Code)
	require.Equal(t, uint64(40), h.state().Epoch)
	require.Equal(t, coin(40), h.usage(h.sender).TotalSent)
}

func TestSinkChallengeIntervalCrossesNativeEpoch(t *testing.T) {
	h := setupReportingTestSuite(t)
	start := h.now
	epoch := epochstypes.NewGenesisEpochInfo("day", 10*time.Minute)
	epoch.StartTime = start
	h.setQuotaEpoch(t, epoch)
	res := h.deliver(t, "", h.deposit(10))
	require.Zero(t, res.Code, res.Log)
	h.now = start.Add(10*time.Minute - 4*time.Second)
	h.quorum(t, 1, types.OutcomeVoided)
	authorizedAt := h.now
	deadline := time.Unix(h.transfer(t, 1).Settlement.ChallengeEnd, 0).UTC()
	require.Equal(t, authorizedAt.Add(h.params().ReporterChallengePeriod), deadline)
	h.now = start.Add(10 * time.Minute)
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	require.Equal(t, uint64(2), h.state().Epoch)
	p := h.params()
	p.ReporterChallengePeriod += time.Hour
	require.NoError(t, h.update(p))
	require.Equal(t, deadline, time.Unix(h.transfer(t, 1).Settlement.ChallengeEnd, 0).UTC())
	h.now = deadline.Add(-time.Second - time.Nanosecond)
	res = h.refund(t, 1)
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "challenge")
	res = h.commitBlock(t, deadline, h.txFrom(t, h.reporters[4], "", &types.MsgFinalizeRefund{
		Executor: h.reporterAddress(4),
		ID:       h.id(1),
	}))[0]
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, types.StatusRefunded, h.transfer(t, 1).Status)
	require.Equal(t, coin(10), h.state().RefundUsage.Amount)
}
