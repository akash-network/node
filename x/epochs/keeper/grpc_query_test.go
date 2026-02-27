package keeper_test

import (
	types "pkg.akt.dev/go/node/epochs/v1beta1"
)

func (s *KeeperTestSuite) TestQueryEpochInfos() {
	s.SetupTest()
	queryClient := s.queryClient

	// Check that querying epoch infos on default genesis returns the default genesis epoch infos
	epochInfosResponse, err := queryClient.EpochInfos(s.Ctx, &types.QueryEpochInfosRequest{})
	s.Require().NoError(err)
	s.Require().Len(epochInfosResponse.Epochs, 4)
	expectedEpochs := types.DefaultGenesis().Epochs
	for id := range expectedEpochs {
		expectedEpochs[id].StartTime = s.Ctx.BlockTime()
		expectedEpochs[id].CurrentEpochStartHeight = s.Ctx.BlockHeight()
	}

	s.Require().Equal(expectedEpochs, epochInfosResponse.Epochs)
}
