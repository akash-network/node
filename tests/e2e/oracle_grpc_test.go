//go:build e2e.integration

package e2e

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	sdktestutil "github.com/cosmos/cosmos-sdk/testutil"

	"pkg.akt.dev/go/cli"
	clitestutil "pkg.akt.dev/go/cli/testutil"
	types "pkg.akt.dev/go/node/oracle/v1"

	"pkg.akt.dev/node/v2/testutil"
)

type oracleGRPCRestTestSuite struct {
	*testutil.NetworkTestSuite

	cctx client.Context
}

func (s *oracleGRPCRestTestSuite) SetupSuite() {
	s.NetworkTestSuite.SetupSuite()

	val := s.Network().Validators[0]
	s.cctx = val.ClientCtx
}

func (s *oracleGRPCRestTestSuite) TestQueryParams() {
	val := s.Network().Validators[0]
	ctx := context.Background()

	// Test via CLI
	resp, err := clitestutil.ExecQueryOracleParams(
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
			fmt.Sprintf("%s/akash/oracle/v1/params", val.APIAddress),
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

func (s *oracleGRPCRestTestSuite) TestQueryPrices() {
	val := s.Network().Validators[0]
	ctx := context.Background()

	// Query prices via CLI - should return empty since no prices are fed yet
	resp, err := clitestutil.ExecQueryOraclePrices(
		ctx,
		s.cctx.WithOutputFormat("json"),
		cli.TestFlags().WithOutputJSON()...,
	)
	s.Require().NoError(err)

	var pricesResp types.QueryPricesResponse
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &pricesResp)
	s.Require().NoError(err)
	// Prices may be empty if no price data has been fed
	s.Require().NotNil(pricesResp.Prices)

	// Test via REST
	testCases := []struct {
		name   string
		url    string
		expErr bool
	}{
		{
			"query prices without filters",
			fmt.Sprintf("%s/akash/oracle/v1/prices", val.APIAddress),
			false,
		},
		{
			"query prices with asset filter",
			fmt.Sprintf("%s/akash/oracle/v1/prices?filters.asset_denom=uakt", val.APIAddress),
			false,
		},
		{
			"query prices with base filter",
			fmt.Sprintf("%s/akash/oracle/v1/prices?filters.base_denom=uusd", val.APIAddress),
			false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var prices types.QueryPricesResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &prices)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				// Prices list should not be nil even if empty
				s.Require().NotNil(prices.Prices)
			}
		})
	}
}

func (s *oracleGRPCRestTestSuite) TestQueryPriceFeedConfig() {
	val := s.Network().Validators[0]
	ctx := context.Background()

	// Query price feed config via CLI - requires denom argument
	resp, err := clitestutil.ExecQueryOraclePriceFeedConfig(
		ctx,
		s.cctx.WithOutputFormat("json"),
		cli.TestFlags().
			With("uakt").
			WithOutputJSON()...,
	)
	s.Require().NoError(err)

	var configResp types.QueryPriceFeedConfigResponse
	err = s.cctx.Codec.UnmarshalJSON(resp.Bytes(), &configResp)
	s.Require().NoError(err)
	// Config may not be enabled by default
	s.Require().False(configResp.Enabled)

	// Test via REST - note the endpoint path uses underscore, not hyphen
	testCases := []struct {
		name   string
		url    string
		expErr bool
	}{
		{
			"query price feed config",
			fmt.Sprintf("%s/akash/oracle/v1/price_feed_config/uakt", val.APIAddress),
			false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			resp, err := sdktestutil.GetRequest(tc.url)
			s.Require().NoError(err)

			var config types.QueryPriceFeedConfigResponse
			err = val.ClientCtx.Codec.UnmarshalJSON(resp, &config)

			if tc.expErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				// Config may not be enabled by default
			}
		})
	}
}
