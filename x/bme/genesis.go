package bme

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/bme/v1"

	"pkg.akt.dev/node/v2/x/bme/keeper"
)

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, k keeper.Keeper, data *types.GenesisState) {
	if err := data.Validate(); err != nil {
		panic(err)
	}
	if err := k.SetParams(ctx, data.Params); err != nil {
		panic(err)
	}
	//err := k.SetVaultState(ctx, data.VaultState)
	//if err != nil {
	//	panic(err)
	//}
}

// ExportGenesis returns genesis state for the deployment module
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	params, err := k.GetParams(ctx)
	if err != nil {
		panic(err)
	}

	//vaultState, err := k.GetVaultState(ctx)
	//if err != nil {
	//	panic(err)
	//}

	return &types.GenesisState{
		Params: params,
		//VaultState:       vaultState,
		//NetBurnSnapshots: []types.NetBurnSnapshot{},
	}
}
