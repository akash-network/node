package keeper

import (
	"time"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "pkg.akt.dev/go/node/provider/v1beta4"
)

const (
	defaultMaintenanceMaxDuration  = 7 * 24 * time.Hour
	defaultMaintenanceMaxLookahead = 90 * 24 * time.Hour
)

func DefaultParams() types.ProviderMaintenanceParams {
	return types.ProviderMaintenanceParams{
		MaintenanceMaxDuration:  defaultMaintenanceMaxDuration,
		MaintenanceMaxLookahead: defaultMaintenanceMaxLookahead,
	}
}

func ValidateParams(params types.ProviderMaintenanceParams) error {
	if params.MaintenanceMaxDuration <= 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("maintenance max duration must be positive")
	}

	if params.MaintenanceMaxLookahead <= 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("maintenance max lookahead must be positive")
	}

	return nil
}
