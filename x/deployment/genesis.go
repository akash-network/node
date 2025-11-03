package deployment

import (
	"encoding/json"
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"

	"pkg.akt.dev/node/v2/x/deployment/keeper"
	"pkg.akt.dev/node/v2/x/deployment/keeper/keys"
)

// ValidateGenesis does validation check of the Genesis and return error in case of failure
func ValidateGenesis(data *dvbeta.GenesisState) error {
	for _, record := range data.Deployments {
		if err := record.Deployment.ID.Validate(); err != nil {
			return fmt.Errorf("%w: %s", err, v1.ErrInvalidDeployment.Error())
		}
	}
	return data.Params.Validate()
}

// DefaultGenesisState returns default genesis state as raw bytes for the deployment
// module.
func DefaultGenesisState() *dvbeta.GenesisState {
	return &dvbeta.GenesisState{
		Params: dvbeta.DefaultParams(),
	}
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, kpr keeper.IKeeper, data *dvbeta.GenesisState) {
	k := kpr.(*keeper.Keeper)

	for _, record := range data.Deployments {
		pk := keys.DeploymentIDToKey(record.Deployment.ID)
		has, err := k.Deployments().Has(ctx, pk)
		if err != nil {
			panic(fmt.Errorf("deployment genesis init. deployment id %s: %w", record.Deployment.ID, err))
		}
		if has {
			panic(fmt.Errorf("deployment genesis init. deployment id %s: %w", record.Deployment.ID, v1.ErrDeploymentExists))
		}
		if err := k.Deployments().Set(ctx, pk, record.Deployment); err != nil {
			panic(fmt.Errorf("deployment genesis init. deployment id %s: %w", record.Deployment.ID, err))
		}

		for idx := range record.Groups {
			group := record.Groups[idx]

			if !group.ID.DeploymentID().Equals(record.Deployment.ID) {
				panic(v1.ErrInvalidGroupID)
			}

			gpk := keys.GroupIDToKey(group.ID)
			if err := k.Groups().Set(ctx, gpk, group); err != nil {
				panic(fmt.Errorf("deployment genesis groups init. group id %s: %w", group.ID, err))
			}
		}
	}

	err := kpr.SetParams(ctx, data.Params)
	if err != nil {
		panic(err.Error())
	}
}

// ExportGenesis returns genesis state for the deployment module
func ExportGenesis(ctx sdk.Context, k keeper.IKeeper) (*dvbeta.GenesisState, error) {
	var records []dvbeta.GenesisDeployment
	err := k.WithDeployments(ctx, func(deployment v1.Deployment) bool {
		records = append(records, dvbeta.GenesisDeployment{
			Deployment: deployment,
		})
		return false
	})
	if err != nil {
		return nil, err
	}

	for i := range records {
		var groups dvbeta.Groups
		groups, err = k.GetGroups(ctx, records[i].Deployment.ID)
		if err != nil {
			return nil, err
		}

		records[i].Groups = groups
	}

	params := k.GetParams(ctx)
	return &dvbeta.GenesisState{
		Deployments: records,
		Params:      params,
	}, nil
}

// GetGenesisStateFromAppState returns x/deployment GenesisState given raw application
// genesis state.
func GetGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *dvbeta.GenesisState {
	var genesisState dvbeta.GenesisState

	if appState[ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[ModuleName], &genesisState)
	}

	return &genesisState
}
