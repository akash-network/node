//go:build e2e.integration

package e2e

import (
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/go/cli"
	clitestutil "pkg.akt.dev/go/cli/testutil"
	types "pkg.akt.dev/go/node/oracle/v1"

	"pkg.akt.dev/node/v2/testutil"
)

type oracleIntegrationTestSuite struct {
	*testutil.NetworkTestSuite
}

func (s *oracleIntegrationTestSuite) TestQueryOracleParams() {
	result, err := clitestutil.ExecQueryOracleParams(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithOutputJSON()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	var paramsResp types.QueryParamsResponse
	err = s.ClientContextForTest().Codec.UnmarshalJSON(result.Bytes(), &paramsResp)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), paramsResp.Params)
}

func (s *oracleIntegrationTestSuite) TestQueryOraclePrices() {
	result, err := clitestutil.ExecQueryOraclePrices(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithOutputJSON()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	var pricesResp types.QueryPricesResponse
	err = s.ClientContextForTest().Codec.UnmarshalJSON(result.Bytes(), &pricesResp)
	require.NoError(s.T(), err)
	// Prices may be empty if no price data has been fed yet
	require.NotNil(s.T(), pricesResp.Prices)
}

func (s *oracleIntegrationTestSuite) TestQueryOraclePriceFeedConfig() {
	// Query price feed config for uakt denom
	result, err := clitestutil.ExecQueryOraclePriceFeedConfig(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			With("uakt").
			WithOutputJSON()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	var configResp types.QueryPriceFeedConfigResponse
	err = s.ClientContextForTest().Codec.UnmarshalJSON(result.Bytes(), &configResp)
	require.NoError(s.T(), err)
	// Config may not be enabled by default
	require.False(s.T(), configResp.Enabled)
}
