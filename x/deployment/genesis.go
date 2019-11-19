package deployment

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

type GenesisDeployment struct {
	types.Deployment
	Groups []types.Group
}

type GenesisState struct {
	Deployments []GenesisDeployment `json:"deployments"`
}

// func NewGenesisState(deployments []Deployment) GenesisState {
// 	return GenesisState{
// 		Deployments: deployments,
// 	}
// }

func ValidateGenesis(data GenesisState) error {
	for _, record := range data.Deployments {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("invalid deployment: missing ID")
		}
	}
	return nil
}

func DefaultGenesisState() GenesisState {
	return GenesisState{}
}

func InitGenesis(ctx sdk.Context, keeper keeper.Keeper, data GenesisState) []abci.ValidatorUpdate {
	for _, record := range data.Deployments {
		keeper.Create(ctx, record.Deployment, record.Groups)
	}
	return []abci.ValidatorUpdate{}
}

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
