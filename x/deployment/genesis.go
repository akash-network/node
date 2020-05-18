package deployment

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

// GenesisDeployment defines the basic genesis state used by deployment module
type GenesisDeployment struct {
	types.Deployment
	Groups []types.Group
}

// GenesisState stores slice of genesis deployment instance
type GenesisState struct {
	Deployments []GenesisDeployment `json:"deployments"`
}

// func NewGenesisState(deployments []Deployment) GenesisState {
// 	return GenesisState{
// 		Deployments: deployments,
// 	}
// }

// ValidateGenesis does validation check of the Genesis and return error incase of failure
func ValidateGenesis(data GenesisState) error {
	for _, record := range data.Deployments {
		if err := record.Validate(); err != nil {
			return errors.Wrap(err, types.ErrInvalidDeployment.Error())
		}
	}
	return nil
}

// DefaultGenesisState returns default genesis state as raw bytes for the deployment
// module.
func DefaultGenesisState() GenesisState {
	return GenesisState{}
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.Keeper, data GenesisState) []abci.ValidatorUpdate {
	for _, record := range data.Deployments {
		keeper.Create(ctx, record.Deployment, record.Groups)
	}
	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state for the deployment module
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) GenesisState {
	var records []GenesisDeployment
	k.WithDeployments(ctx, func(deployment types.Deployment) bool {
		groups := k.GetGroups(ctx, deployment.ID())
		records = append(records, GenesisDeployment{
			Deployment: deployment,
			Groups:     groups,
		})
		return false
	})
	return GenesisState{Deployments: records}
}
