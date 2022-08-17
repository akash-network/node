package app

import (
	"encoding/json"
	"fmt"
	"math/rand"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/types/simulation"
	ica "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts"
	icatypes "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/types"
)

type ICAHostSimModule struct {
	ica.AppModule
	ica.AppModuleBasic
	cdc codec.Codec
}

var (
	_ module.AppModule           = ICAHostSimModule{}
	_ module.AppModuleBasic      = ICAHostSimModule{}
	_ module.AppModuleSimulation = ICAHostSimModule{}
)

// NewICAHostSimModule Simulation functions for ICAHost module
// This thing does the bare minimum to avoid simulation panic-ing for missing state data.
func NewICAHostSimModule(baseModule ica.AppModule, cdc codec.Codec) ICAHostSimModule {
	return ICAHostSimModule{
		cdc:            cdc,
		AppModule:      baseModule,
		AppModuleBasic: baseModule.AppModuleBasic,
	}
}

// GenerateGenesisState implements module.AppModuleSimulation
func (ICAHostSimModule) GenerateGenesisState(simState *module.SimulationState) {
	genesis := icatypes.DefaultGenesis()

	bz, err := json.MarshalIndent(&genesis, "", " ")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Selected randomly generated %s parameters:\n%s\n", icatypes.ModuleName, bz)
	simState.GenState[icatypes.ModuleName] = simState.Cdc.MustMarshalJSON(genesis)
}

// ProposalContents implements module.AppModuleSimulation
func (ICAHostSimModule) ProposalContents(simState module.SimulationState) []simulation.WeightedProposalContent {
	return nil
}

// RandomizedParams implements module.AppModuleSimulation
func (ICAHostSimModule) RandomizedParams(r *rand.Rand) []simulation.ParamChange {
	return nil
}

// RegisterStoreDecoder implements module.AppModuleSimulation
func (ICAHostSimModule) RegisterStoreDecoder(sdk.StoreDecoderRegistry) {
}

// WeightedOperations implements module.AppModuleSimulation
func (ICAHostSimModule) WeightedOperations(simState module.SimulationState) []simulation.WeightedOperation {
	return nil
}
