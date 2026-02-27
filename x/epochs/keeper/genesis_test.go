package keeper_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/epochs/v1beta1"
)

func TestEpochsExportGenesis(t *testing.T) {
	ctx, epochsKeeper := Setup(t)

	chainStartTime := ctx.BlockTime()
	chainStartHeight := ctx.BlockHeight()

	genesis, err := epochsKeeper.ExportGenesis(ctx)
	require.NoError(t, err)
	require.Len(t, genesis.Epochs, 4)

	expectedEpochs := types.DefaultGenesis().Epochs
	for i := range expectedEpochs {
		expectedEpochs[i].CurrentEpochStartHeight = chainStartHeight
		expectedEpochs[i].StartTime = chainStartTime
	}
	require.Equal(t, expectedEpochs, genesis.Epochs)
}

func TestEpochsInitGenesis(t *testing.T) {
	ctx, epochsKeeper := Setup(t)

	// On init genesis, default epochs information is set
	// To check init genesis again, should make it fresh status

	allEpochs := make([]types.EpochInfo, 0)
	err := epochsKeeper.IterateEpochs(ctx, func(_ string, info types.EpochInfo) (bool, error) {
		allEpochs = append(allEpochs, info)
		return false, nil
	})

	require.NoError(t, err)
	for _, epochInfo := range allEpochs {
		err := epochsKeeper.RemoveEpoch(ctx, epochInfo.ID)
		require.NoError(t, err)
	}

	// now := time.Now()
	ctx = ctx.WithBlockHeight(1).WithBlockTime(time.Now().UTC())

	// test genesisState validation
	genesisState := types.GenesisState{
		Epochs: []types.EpochInfo{
			{
				ID:                      "monthly",
				StartTime:               time.Time{},
				Duration:                time.Hour * 24,
				CurrentEpoch:            0,
				CurrentEpochStartHeight: ctx.BlockHeight(),
				CurrentEpochStartTime:   time.Time{},
				EpochCountingStarted:    true,
			},
			{
				ID:                      "monthly",
				StartTime:               time.Time{},
				Duration:                time.Hour * 24,
				CurrentEpoch:            0,
				CurrentEpochStartHeight: ctx.BlockHeight(),
				CurrentEpochStartTime:   time.Time{},
				EpochCountingStarted:    true,
			},
		},
	}
	require.EqualError(t, genesisState.Validate(), "epoch identifier should be unique")

	genesisState = types.GenesisState{
		Epochs: []types.EpochInfo{
			{
				ID:                      "monthly",
				StartTime:               time.Time{},
				Duration:                time.Hour * 24,
				CurrentEpoch:            0,
				CurrentEpochStartHeight: ctx.BlockHeight(),
				CurrentEpochStartTime:   time.Time{},
				EpochCountingStarted:    true,
			},
		},
	}

	err = epochsKeeper.InitGenesis(ctx, genesisState)
	require.NoError(t, err)
	epochInfo, err := epochsKeeper.GetEpoch(ctx, "monthly")
	require.NoError(t, err)
	require.Equal(t, epochInfo.ID, "monthly")
	require.Equal(t, epochInfo.StartTime.UTC().String(), ctx.BlockTime().UTC().String())
	require.Equal(t, epochInfo.Duration, time.Hour*24)
	require.Equal(t, epochInfo.CurrentEpoch, int64(0))
	require.Equal(t, epochInfo.CurrentEpochStartHeight, ctx.BlockHeight())
	require.Equal(t, epochInfo.CurrentEpochStartTime.UTC().String(), time.Time{}.String())
	require.Equal(t, epochInfo.EpochCountingStarted, true)
}
