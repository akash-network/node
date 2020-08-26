package deployment

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

// // GenesisDeployment defines the basic genesis state used by deployment module
// type GenesisDeployment struct {
// 	types.Deployment
// 	Groups []types.Group
// }

// // GenesisState stores slice of genesis deployment instance
// type GenesisState struct {
// 	Deployments []GenesisDeployment `json:"deployments"`
// }

// func NewGenesisState(deployments []Deployment) GenesisState {
// 	return GenesisState{
// 		Deployments: deployments,
// 	}
// }

// ValidateGenesis does validation check of the Genesis and return error incase of failure
func ValidateGenesis(data *types.GenesisState) error {
	for _, record := range data.Deployments {
		if err := record.Deployment.ID().Validate(); err != nil {
			return errors.Wrap(err, types.ErrInvalidDeployment.Error())
		}
	}
	return nil
}

// DefaultGenesisState returns default genesis state as raw bytes for the deployment
// module.
func DefaultGenesisState() *types.GenesisState {
	return &types.GenesisState{}
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.Keeper, data *types.GenesisState) []abci.ValidatorUpdate {
	for _, record := range data.Deployments {
		if err := keeper.Create(ctx, record.Deployment, record.Groups); err != nil {
			return nil
		}
	}
	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state for the deployment module
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	var records []types.GenesisDeployment
	k.WithDeployments(ctx, func(deployment types.Deployment) bool {
		groups := k.GetGroups(ctx, deployment.ID())
		records = append(records, types.GenesisDeployment{
			Deployment: deployment,
			Groups:     groups,
		})
		return false
	})
	return &types.GenesisState{Deployments: records}
}
