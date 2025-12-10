//go:build e2e.integration

package e2e

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"

	"pkg.akt.dev/go/cli"
	clitestutil "pkg.akt.dev/go/cli/testutil"
	types "pkg.akt.dev/go/node/bme/v1"

	"pkg.akt.dev/node/v2/testutil"
)

type bmeGRPCRestTestSuite struct {
	*testutil.NetworkTestSuite

	cctx client.Context
}

func (s *bmeGRPCRestTestSuite) SetupSuite() {
	s.NetworkTestSuite.SetupSuite()

	val := s.Network().Validators[0]
	s.cctx = val.ClientCtx
}

func (s *bmeGRPCRestTestSuite) TestQueryParams() {
	val := s.Network().Validators[0]
	ctx := context.Background()

	// Test via CLI
	resp, err := clitestutil.ExecQueryBMEParams(
		ctx,
		s.cctx.WithOutputFormat("json"),
		cli.TestFlags().WithOutputJSON()...,
	)
	s.Require().NoError(err)

	var paramsResp types.QueryParamsResponse
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &paramsResp)
	s.Require().NoError(err)
	s.Require().NotNil(paramsResp.Params)

	// Test via REST
	testCases := []struct {
		name   string
		url    string
		expErr bool
	}{
		{
			"query params via REST",
			fmt.Sprintf("%s/akash/bme/v1/params", val.APIAddress),
			false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var params types.QueryParamsResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &params)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().NotNil(params.Params)
			}
		})
	}
}

func (s *bmeGRPCRestTestSuite) TestQueryVaultState() {
	val := s.Network().Validators[0]
	ctx := context.Background()

	// Test via CLI
	resp, err := clitestutil.ExecQueryBMEVaultState(
		ctx,
		s.cctx.WithOutputFormat("json"),
		cli.TestFlags().WithOutputJSON()...,
	)
	s.Require().NoError(err)

	var vaultResp types.QueryVaultStateResponse
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &vaultResp)
	s.Require().NoError(err)
	// VaultState should be valid even if empty
	s.Require().NotNil(vaultResp.VaultState)

	// Test via REST
	testCases := []struct {
		name   string
		url    string
		expErr bool
	}{
		{
			"query vault state via REST",
			fmt.Sprintf("%s/akash/bme/v1/vault-state", val.APIAddress),
			false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var vaultState types.QueryVaultStateResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &vaultState)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().NotNil(vaultState.VaultState)
			}
		})
	}
}

func (s *bmeGRPCRestTestSuite) TestQueryCollateralRatio() {
	val := s.Network().Validators[0]
	ctx := context.Background()

	// Test via CLI
	resp, err := clitestutil.ExecQueryBMECollateralRatio(
		ctx,
		s.cctx.WithOutputFormat("json"),
		cli.TestFlags().WithOutputJSON()...,
	)
	s.Require().NoError(err)

	var crResp types.QueryCollateralRatioResponse
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &crResp)
	s.Require().NoError(err)
	// Collateral ratio should be returned
	s.Require().NotNil(crResp.CollateralRatio)

	// Test via REST
	testCases := []struct {
		name   string
		url    string
		expErr bool
	}{
		{
			"query collateral ratio via REST",
			fmt.Sprintf("%s/akash/bme/v1/collateral-ratio", val.APIAddress),
			false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var cr types.QueryCollateralRatioResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &cr)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().NotNil(cr.CollateralRatio)
			}
		})
	}
}

func (s *bmeGRPCRestTestSuite) TestQueryCircuitBreakerStatus() {
	val := s.Network().Validators[0]
	ctx := context.Background()

	// Test via CLI
	resp, err := clitestutil.ExecQueryBMECircuitBreakerStatus(
		ctx,
		s.cctx.WithOutputFormat("json"),
		cli.TestFlags().WithOutputJSON()...,
	)
	s.Require().NoError(err)

	var cbResp types.QueryCircuitBreakerStatusResponse
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &cbResp)
	s.Require().NoError(err)
	// Circuit breaker status should be valid
	s.Require().True(cbResp.Status == types.CircuitBreakerStatusHealthy ||
		cbResp.Status == types.CircuitBreakerStatusWarning ||
		cbResp.Status == types.CircuitBreakerStatusHalt)
	// In default test setup, settlements and refunds should be allowed
	s.Require().True(cbResp.SettlementsAllowed)
	s.Require().True(cbResp.RefundsAllowed)

	// Test via REST
	testCases := []struct {
		name   string
		url    string
		expErr bool
	}{
		{
			"query circuit breaker status via REST",
			fmt.Sprintf("%s/akash/bme/v1/circuit-breaker-status", val.APIAddress),
			false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var cbStatus types.QueryCircuitBreakerStatusResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &cbStatus)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				// Verify status is one of the valid values
				s.Require().True(cbStatus.Status == types.CircuitBreakerStatusHealthy ||
					cbStatus.Status == types.CircuitBreakerStatusWarning ||
					cbStatus.Status == types.CircuitBreakerStatusHalt)
			}
		})
	}
}
