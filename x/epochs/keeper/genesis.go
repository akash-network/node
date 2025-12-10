package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	types "pkg.akt.dev/go/node/epochs/v1beta1"
)

// InitGenesis sets epoch info from genesis
func (k *keeper) InitGenesis(ctx sdk.Context, genState types.GenesisState) error {
	for _, epoch := range genState.Epochs {
		err := k.AddEpoch(ctx, epoch)
		if err != nil {
			return err
		}
	}
	return nil
}

// ExportGenesis returns the capability module's exported genesis.
func (k *keeper) ExportGenesis(ctx sdk.Context) (*types.GenesisState, error) {
	epochs := make([]types.EpochInfo, 0)
	err := k.IterateEpochs(ctx, func(_ string, info types.EpochInfo) (bool, error) {
		epochs = append(epochs, info)
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	genesis := &types.GenesisState{
		Epochs: epochs,
	}

	return genesis, nil
}
