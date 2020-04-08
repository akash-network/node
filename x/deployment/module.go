package deployment

import (
	"encoding/json"
	"math/rand"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/bank"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/gorilla/mux"
	"github.com/ovrclk/akash/x/deployment/client/cli"
	"github.com/ovrclk/akash/x/deployment/client/rest"
	"github.com/ovrclk/akash/x/deployment/handler"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/query"
	"github.com/ovrclk/akash/x/deployment/simulation"
	"github.com/ovrclk/akash/x/deployment/types"

	"github.com/spf13/cobra"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sim "github.com/cosmos/cosmos-sdk/x/simulation"
	abci "github.com/tendermint/tendermint/abci/types"
)

// type check to ensure the interface is properly implemented
var (
	_ module.AppModule           = AppModule{}
	_ module.AppModuleBasic      = AppModuleBasic{}
	_ module.AppModuleSimulation = AppModuleSimulation{}
)

// AppModuleBasic defines the basic application module used by the deployment module.
type AppModuleBasic struct{}

// Name returns deployment module's name
func (AppModuleBasic) Name() string {
	return types.ModuleName
}

// RegisterCodec registers the deployment module's types for the given codec.
func (AppModuleBasic) RegisterCodec(cdc *codec.Codec) {
	types.RegisterCodec(cdc)
}

// DefaultGenesis returns default genesis state as raw bytes for the deployment
// module.
func (AppModuleBasic) DefaultGenesis() json.RawMessage {
	return types.MustMarshalJSON(DefaultGenesisState())
}

// ValidateGenesis does validation check of the Genesis and returns error incase of failure
func (AppModuleBasic) ValidateGenesis(bz json.RawMessage) error {
	var data GenesisState
	err := types.UnmarshalJSON(bz, &data)
	if err != nil {
		return err
	}
	return ValidateGenesis(data)
}

// RegisterRESTRoutes registers rest routes for this module
func (AppModuleBasic) RegisterRESTRoutes(ctx context.CLIContext, rtr *mux.Router) {
	rest.RegisterRoutes(ctx, rtr, StoreKey)
}

// GetQueryCmd get the root query command of this module
func (AppModuleBasic) GetQueryCmd(cdc *codec.Codec) *cobra.Command {
	return cli.GetQueryCmd(StoreKey, cdc)
}

// GetTxCmd get the root tx command of this module
func (AppModuleBasic) GetTxCmd(cdc *codec.Codec) *cobra.Command {
	return cli.GetTxCmd(StoreKey, cdc)
}

// GetQueryClient returns a new query client for this module
func (AppModuleBasic) GetQueryClient(ctx context.CLIContext) query.Client {
	return query.NewClient(ctx, StoreKey)
}

// AppModule implements an application module for the deployment module.
type AppModule struct {
	AppModuleBasic
	keeper     keeper.Keeper
	mkeeper    handler.MarketKeeper
	coinKeeper bank.Keeper
}

// NewAppModule creates a new AppModule Object
func NewAppModule(k keeper.Keeper, mkeeper handler.MarketKeeper, bankKeeper bank.Keeper) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{},
		keeper:         k,
		mkeeper:        mkeeper,
		coinKeeper:     bankKeeper,
	}
}

// Name returns the deployment module name
func (AppModule) Name() string {
	return types.ModuleName
}

// RegisterInvariants registers module invariants
func (am AppModule) RegisterInvariants(ir sdk.InvariantRegistry) {}

// Route returns the message routing key for the deployment module
func (am AppModule) Route() string {
	return types.RouterKey
}

// NewHandler returns an sdk.Handler for the deployment module.
func (am AppModule) NewHandler() sdk.Handler {
	return handler.NewHandler(am.keeper, am.mkeeper)
}

// QuerierRoute returns the deployment module's querier route name.
func (am AppModule) QuerierRoute() string {
	return types.ModuleName
}

// NewQuerierHandler returns the sdk.Querier for deployment module
func (am AppModule) NewQuerierHandler() sdk.Querier {
	return query.NewQuerier(am.keeper)
}

// BeginBlock performs no-op
func (am AppModule) BeginBlock(_ sdk.Context, _ abci.RequestBeginBlock) {}

// EndBlock returns the end blocker for the deployment module. It returns no validator
// updates.
func (am AppModule) EndBlock(ctx sdk.Context, _ abci.RequestEndBlock) []abci.ValidatorUpdate {

	handler.OnEndBlock(ctx, am.keeper, am.mkeeper)

	return []abci.ValidatorUpdate{}
}

// InitGenesis performs genesis initialization for the deployment module. It returns
// no validator updates.
func (am AppModule) InitGenesis(ctx sdk.Context, data json.RawMessage) []abci.ValidatorUpdate {
	var genesisState GenesisState
	types.MustUnmarshalJSON(data, &genesisState)
	return InitGenesis(ctx, am.keeper, genesisState)
}

// ExportGenesis returns the exported genesis state as raw bytes for the deployment
// module.
func (am AppModule) ExportGenesis(ctx sdk.Context) json.RawMessage {
	gs := ExportGenesis(ctx, am.keeper)
	return types.MustMarshalJSON(gs)
}

//____________________________________________________________________________

// AppModuleSimulation implements an application simulation module for the deployment module.
type AppModuleSimulation struct {
	keeper  keeper.Keeper
	akeeper govtypes.AccountKeeper
}

// NewAppModule creates a new AppModuleSimulation instance
func NewAppModuleSimulation(k keeper.Keeper, akeeper govtypes.AccountKeeper) AppModuleSimulation {
	return AppModuleSimulation{
		keeper:  k,
		akeeper: akeeper,
	}
}

// AppModuleSimulation functions

// GenerateGenesisState creates a randomized GenState of the staking module.
func (AppModuleSimulation) GenerateGenesisState(simState *module.SimulationState) {
	simulation.RandomizedGenState(simState)
}

// ProposalContents doesn't return any content functions for governance proposals.
func (AppModuleSimulation) ProposalContents(_ module.SimulationState) []sim.WeightedProposalContent {
	return nil
}

// RandomizedParams creates randomized staking param changes for the simulator.
func (AppModuleSimulation) RandomizedParams(r *rand.Rand) []sim.ParamChange {
	return nil
}

// RegisterStoreDecoder registers a decoder for staking module's types
func (AppModuleSimulation) RegisterStoreDecoder(sdr sdk.StoreDecoderRegistry) {
	// sdr[StoreKey] = simulation.DecodeStore
}

// WeightedOperations returns the all the staking module operations with their respective weights.
func (am AppModuleSimulation) WeightedOperations(simState module.SimulationState) []sim.WeightedOperation {
	return simulation.WeightedOperations(simState.AppParams, simState.Cdc,
		am.akeeper, am.keeper)
}
