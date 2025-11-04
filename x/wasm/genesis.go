package wasm

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	types "pkg.akt.dev/go/node/wasm/v1"

	"pkg.akt.dev/node/v2/x/wasm/keeper"
)

// ValidateGenesis does validation check of the Genesis and return error incase of failure
func ValidateGenesis(data *types.GenesisState) error {
	return data.Params.Validate()
}

// DefaultGenesisState returns default genesis state as raw bytes for the deployment
// module.
func DefaultGenesisState() *types.GenesisState {
	return &types.GenesisState{
		Params: types.Params{
			BlockedAddresses: []string{
				authtypes.NewModuleAddress(govtypes.ModuleName).String(),
				authtypes.NewModuleAddress(distrtypes.ModuleName).String(),
				authtypes.NewModuleAddress(stakingtypes.BondedPoolName).String(),
				authtypes.NewModuleAddress(stakingtypes.NotBondedPoolName).String(),
			},
		},
	}
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.Keeper, data *types.GenesisState) {
	err := keeper.SetParams(ctx, data.Params)
	if err != nil {
		panic(err.Error())
	}
}

// ExportGenesis returns genesis state for the deployment module
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	params := k.GetParams(ctx)
	return &types.GenesisState{
		Params: params,
	}
}
