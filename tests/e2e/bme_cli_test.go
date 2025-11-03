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

func (s *bmeIntegrationTestSuite) TestQueryBMEStatus() {
	result, err := clitestutil.ExecQueryBMEStatus(
		s.ContextForTest(),
		s.ClientContextForTest(),
		cli.TestFlags().
			WithOutputJSON()...,
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), result)

	var statusResp types.QueryStatusResponse
	err = s.ClientContextForTest().Codec.UnmarshalJSON(result.Bytes(), &statusResp)
	require.NoError(s.T(), err)

	// Status should be one of the valid MintStatus values
	require.True(s.T(),
		statusResp.Status == types.MintStatusHealthy ||
			statusResp.Status == types.MintStatusWarning ||
			statusResp.Status == types.MintStatusHaltCR ||
			statusResp.Status == types.MintStatusHaltOracle,
		"unexpected status: %v", statusResp.Status,
	)
}
