package keeper

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/bme/v1"
)

type Querier struct {
	*keeper
}

var _ types.QueryServer = &Querier{}

func (qs Querier) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	sctx := sdk.UnwrapSDKContext(ctx)

	params, err := qs.GetParams(sctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: params}, nil
}

func (qs Querier) VaultState(ctx context.Context, _ *types.QueryVaultStateRequest) (*types.QueryVaultStateResponse, error) {
	sctx := sdk.UnwrapSDKContext(ctx)

	state, err := qs.GetVaultState(sctx)
	if err != nil {
		return nil, err
	}

	return &types.QueryVaultStateResponse{VaultState: state}, nil
}

func (qs Querier) CollateralRatio(ctx context.Context, req *types.QueryCollateralRatioRequest) (*types.QueryCollateralRatioResponse, error) {
	sctx := sdk.UnwrapSDKContext(ctx)

	cr, err := qs.GetCollateralRatio(sctx)
	if err != nil {
		return nil, err
	}

	return &types.QueryCollateralRatioResponse{
		CollateralRatio: types.CollateralRatio{
			Ratio: cr,
		},
	}, nil
}

func (qs Querier) CircuitBreakerStatus(ctx context.Context, req *types.QueryCircuitBreakerStatusRequest) (*types.QueryCircuitBreakerStatusResponse, error) {
	sctx := sdk.UnwrapSDKContext(ctx)

	params, err := qs.GetParams(sctx)
	if err != nil {
		return nil, err
	}

	status, err := qs.GetCircuitBreakerStatus(sctx)
	if err != nil {
		return nil, err
	}

	cr, _ := qs.GetCollateralRatio(sctx)

	warnThreshold := math.LegacyNewDec(int64(params.CircuitBreakerWarnThreshold)).Quo(math.LegacyNewDec(10000))
	haltThreshold := math.LegacyNewDec(int64(params.CircuitBreakerHaltThreshold)).Quo(math.LegacyNewDec(10000))

	return &types.QueryCircuitBreakerStatusResponse{
		Status:             status,
		CollateralRatio:    cr,
		WarnThreshold:      warnThreshold,
		HaltThreshold:      haltThreshold,
		MintsAllowed:       status != types.CircuitBreakerStatusHalt,
		SettlementsAllowed: true,
		RefundsAllowed:     true,
	}, nil
}
