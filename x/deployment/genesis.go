package deployment

import (
	"encoding/json"
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/node/deployment/v1beta4"

	"pkg.akt.dev/node/x/deployment/keeper"
)

// ValidateGenesis does validation check of the Genesis and return error in case of failure
func ValidateGenesis(data *v1beta4.GenesisState) error {
	for _, record := range data.Deployments {
		if err := record.Deployment.ID.Validate(); err != nil {
			return fmt.Errorf("%w: %s", err, v1.ErrInvalidDeployment.Error())
		}
	}
	return data.Params.Validate()
}

// DefaultGenesisState returns default genesis state as raw bytes for the deployment
// module.
func DefaultGenesisState() *v1beta4.GenesisState {
	return &v1beta4.GenesisState{
		Params: v1beta4.DefaultParams(),
	}
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, kpr keeper.IKeeper, data *v1beta4.GenesisState) {
	cdc := kpr.Codec()
	store := ctx.KVStore(kpr.StoreKey())

	for _, record := range data.Deployments {
		key := keeper.MustDeploymentKey(keeper.DeploymentStateToPrefix(record.Deployment.State), record.Deployment.ID)

		store.Set(key, cdc.MustMarshal(&record.Deployment))

		for idx := range record.Groups {
			group := record.Groups[idx]

			if !group.ID.DeploymentID().Equals(record.Deployment.ID) {
				panic(v1.ErrInvalidGroupID)
			}

			gkey := keeper.MustGroupKey(keeper.GroupStateToPrefix(group.State), group.ID)
			store.Set(gkey, cdc.MustMarshal(&group))
		}
	}

	err := kpr.SetParams(ctx, data.Params)
	if err != nil {
		panic(err.Error())
	}
}

// ExportGenesis returns genesis state for the deployment module
func ExportGenesis(ctx sdk.Context, k keeper.IKeeper) *v1beta4.GenesisState {
	var records []v1beta4.GenesisDeployment
	k.WithDeployments(ctx, func(deployment v1.Deployment) bool {
		groups := k.GetGroups(ctx, deployment.ID)

		records = append(records, v1beta4.GenesisDeployment{
			Deployment: deployment,
			Groups:     groups,
		})
		return false
	})

	params := k.GetParams(ctx)
	return &v1beta4.GenesisState{
		Deployments: records,
		Params:      params,
	}
}

// GetGenesisStateFromAppState returns x/deployment GenesisState given raw application
// genesis state.
func GetGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *v1beta4.GenesisState {
	var genesisState v1beta4.GenesisState

	if appState[ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[ModuleName], &genesisState)
	}

	return &genesisState
}
