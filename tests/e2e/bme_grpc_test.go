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

	// Test via REST - note the endpoint is "/vault", not "/vault-state"
	testCases := []struct {
		name   string
		url    string
		expErr bool
	}{
		{
			"query vault state via REST",
			fmt.Sprintf("%s/akash/bme/v1/vault", val.APIAddress),
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

func (s *bmeGRPCRestTestSuite) TestQueryStatus() {
	val := s.Network().Validators[0]
	ctx := context.Background()

	// Test via CLI - ExecQueryBMEStatus returns status with collateral ratio and mint/refund flags
	resp, err := clitestutil.ExecQueryBMEStatus(
		ctx,
		s.cctx.WithOutputFormat("json"),
		cli.TestFlags().WithOutputJSON()...,
	)
	s.Require().NoError(err)

	var statusResp types.QueryStatusResponse
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &statusResp)
	s.Require().NoError(err)

	// Status should be one of the valid MintStatus values
	s.Require().True(
		statusResp.Status == types.MintStatusHealthy ||
			statusResp.Status == types.MintStatusWarning ||
			statusResp.Status == types.MintStatusHaltCR ||
			statusResp.Status == types.MintStatusHaltOracle,
		"unexpected status: %v", statusResp.Status,
	)

	// Test via REST
	testCases := []struct {
		name   string
		url    string
		expErr bool
	}{
		{
			"query status via REST",
			fmt.Sprintf("%s/akash/bme/v1/status", val.APIAddress),
			false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var status types.QueryStatusResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &status)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				// Verify status is valid
				s.Require().True(
					status.Status == types.MintStatusHealthy ||
						status.Status == types.MintStatusWarning ||
						status.Status == types.MintStatusHaltCR ||
						status.Status == types.MintStatusHaltOracle,
				)
			}
		})
	}
}
