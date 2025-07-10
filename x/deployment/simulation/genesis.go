package simulation

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"pkg.akt.dev/go/node/deployment/v1"
	types "pkg.akt.dev/go/node/deployment/v1beta4"
)

var (
	minDeposit, _ = types.DefaultParams().MinDepositFor("uakt")
)

// RandomizedGenState generates a random GenesisState for supply
func RandomizedGenState(simState *module.SimulationState) {
	// numDeployments := simulation.RandIntBetween(simState.Rand, 0, len(simState.Accounts))

	deploymentGenesis := &types.GenesisState{
		Params: types.Params{
			MinDeposits: sdk.Coins{
				minDeposit,
			},
		},
		// Deployments: make([]types.GenesisDeployment, 0, numDeployments),
	}

	// for range cap(deploymentGenesis.Deployments) {
	// 	acc, _ := simtypes.RandomAcc(simState.Rand, simState.Accounts)
	//
	// 	t := &testing.T{}
	//
	// 	depl := testutil.Deployment(t)
	// 	depl.ID.Owner = acc.Address.String()
	//
	// 	dgroups := testutil.DeploymentGroups(t, depl.ID, 1)
	//
	// 	deploymentGenesis.Deployments = append(deploymentGenesis.Deployments, types.GenesisDeployment{
	// 		Deployment: depl,
	// 		Groups:     dgroups,
	// 	})
	// }

	simState.GenState[v1.ModuleName] = simState.Cdc.MustMarshalJSON(deploymentGenesis)
}
