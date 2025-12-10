//go:build e2e.integration

package e2e

import (
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/go/cli"
	clitestutil "pkg.akt.dev/go/cli/testutil"
	types "pkg.akt.dev/go/node/bme/v1"

	"pkg.akt.dev/node/v2/testutil"
)

type bmeIntegrationTestSuite struct {
	*testutil.NetworkTestSuite
}

func (s *bmeIntegrationTestSuite) TestQueryBMEParams() {
	result, err := clitestutil.ExecQueryBMEParams(
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

func (s *bmeIntegrationTestSuite) TestQueryBMEVaultState() {
	result, err := clitestutil.ExecQueryBMEVaultState(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithOutputJSON()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	var vaultResp types.QueryVaultStateResponse
	err = s.ClientContextForTest().Codec.UnmarshalJSON(result.Bytes(), &vaultResp)
	require.NoError(s.T(), err)
	// VaultState should be valid even if empty
	require.NotNil(s.T(), vaultResp.VaultState)
}

func (s *bmeIntegrationTestSuite) TestQueryBMECollateralRatio() {
	result, err := clitestutil.ExecQueryBMECollateralRatio(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithOutputJSON()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	var crResp types.QueryCollateralRatioResponse
	err = s.ClientContextForTest().Codec.UnmarshalJSON(result.Bytes(), &crResp)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), crResp.CollateralRatio)
}

func (s *bmeIntegrationTestSuite) TestQueryBMECircuitBreakerStatus() {
	result, err := clitestutil.ExecQueryBMECircuitBreakerStatus(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithOutputJSON()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	var cbResp types.QueryCircuitBreakerStatusResponse
	err = s.ClientContextForTest().Codec.UnmarshalJSON(result.Bytes(), &cbResp)
	require.NoError(s.T(), err)

	// Circuit breaker status should be valid
	require.True(s.T(), cbResp.Status == types.CircuitBreakerStatusHealthy ||
		cbResp.Status == types.CircuitBreakerStatusWarning ||
		cbResp.Status == types.CircuitBreakerStatusHalt)

	// In default test setup, settlements and refunds should always be allowed
	require.True(s.T(), cbResp.SettlementsAllowed)
	require.True(s.T(), cbResp.RefundsAllowed)
}
