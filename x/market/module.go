package market

import (
	"encoding/json"
	"math/rand"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/gorilla/mux"
	"github.com/ovrclk/akash/x/market/client/cli"
	"github.com/ovrclk/akash/x/market/handler"
	"github.com/ovrclk/akash/x/market/keeper"
	"github.com/ovrclk/akash/x/market/query"
	"github.com/ovrclk/akash/x/market/simulation"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/spf13/cobra"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sim "github.com/cosmos/cosmos-sdk/x/simulation"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

var (
	_ module.AppModule      = AppModule{}
	_ module.AppModuleBasic = AppModuleBasic{}
)

// AppModuleBasic defines the basic application module used by the market module.
type AppModuleBasic struct{}

// Name returns market module's name
func (AppModuleBasic) Name() string {
	return types.ModuleName
}

// RegisterCodec registers the market module's types for the given codec.
func (AppModuleBasic) RegisterCodec(cdc *codec.Codec) {
	types.RegisterCodec(cdc)
}

// DefaultGenesis returns default genesis state as raw bytes for the market
// module.
func (AppModuleBasic) DefaultGenesis() json.RawMessage {
	return types.MustMarshalJSON(DefaultGenesisState())
}

// ValidateGenesis validation check of the Genesis
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
	// rest.RegisterRoutes(ctx, rtr, StoreKey)
}

// GetQueryCmd returns the root query command of this module
func (AppModuleBasic) GetQueryCmd(cdc *codec.Codec) *cobra.Command {
	return cli.GetQueryCmd(StoreKey, cdc)
}

// GetTxCmd returns the root tx command of this module
func (AppModuleBasic) GetTxCmd(cdc *codec.Codec) *cobra.Command {
	return cli.GetTxCmd(StoreKey, cdc)
}

// GetQueryClient returns a new query client for this module
func (AppModuleBasic) GetQueryClient(ctx context.CLIContext) query.Client {
	return query.NewClient(ctx, StoreKey)
}

// AppModule implements an application module for the market module.
type AppModule struct {
	AppModuleBasic
	keepers handler.Keepers
	akeeper stakingtypes.AccountKeeper
}

// NewAppModule creates a new AppModule object
func NewAppModule(
	keeper keeper.Keeper,
	akeeper stakingtypes.AccountKeeper,
	dkeeper handler.DeploymentKeeper,
	pkeeper handler.ProviderKeeper,
	bkeeper bank.Keeper,
) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{},
		keepers: handler.Keepers{
			Market:     keeper,
			Deployment: dkeeper,
			Provider:   pkeeper,
			Bank:       bkeeper,
		},
		akeeper: akeeper,
	}
}

// Name returns the market module name
func (AppModule) Name() string {
	return types.ModuleName
}

// RegisterInvariants registers module invariants
func (am AppModule) RegisterInvariants(ir sdk.InvariantRegistry) {}

// Route returns the message routing key for the market module.
func (am AppModule) Route() string {
	return types.RouterKey
}

// NewHandler returns an sdk.Handler for the market module.
func (am AppModule) NewHandler() sdk.Handler {
	return handler.NewHandler(am.keepers)
}

// QuerierRoute returns the market module's querier route name.
func (am AppModule) QuerierRoute() string {
	return types.ModuleName
}

// NewQuerierHandler returns the sdk.Querier for market module
func (am AppModule) NewQuerierHandler() sdk.Querier {
	return query.NewQuerier(am.keepers.Market)
}

// BeginBlock performs no-op
func (am AppModule) BeginBlock(_ sdk.Context, _ abci.RequestBeginBlock) {}

// EndBlock returns the end blocker for the market module. It returns no validator
// updates.
func (am AppModule) EndBlock(ctx sdk.Context, _ abci.RequestEndBlock) []abci.ValidatorUpdate {
	handler.OnEndBlock(ctx, am.keepers)
	return []abci.ValidatorUpdate{}
}

// InitGenesis performs genesis initialization for the market module. It returns
// no validator updates.
func (am AppModule) InitGenesis(ctx sdk.Context, data json.RawMessage) []abci.ValidatorUpdate {
	var genesisState GenesisState
	types.MustUnmarshalJSON(data, &genesisState)
	return InitGenesis(ctx, am.keepers.Market, genesisState)
}

// ExportGenesis returns the exported genesis state as raw bytes for the market
// module.
func (am AppModule) ExportGenesis(ctx sdk.Context) json.RawMessage {
	gs := ExportGenesis(ctx, am.keepers.Market)
	return types.MustMarshalJSON(gs)
}

//____________________________________________________________________________

// AppModuleSimulation functions

// GenerateGenesisState creates a randomized GenState of the staking module.
func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
	simulation.RandomizedGenState(simState)
}

// ProposalContents doesn't return any content functions for governance proposals.
func (AppModule) ProposalContents(_ module.SimulationState) []sim.WeightedProposalContent {
	return nil
}

// RandomizedParams creates randomized staking param changes for the simulator.
func (AppModule) RandomizedParams(r *rand.Rand) []sim.ParamChange {
	return nil
}

// RegisterStoreDecoder registers a decoder for staking module's types
func (AppModule) RegisterStoreDecoder(sdr sdk.StoreDecoderRegistry) {
	// sdr[StoreKey] = simulation.DecodeStore
}

// WeightedOperations returns the all the staking module operations with their respective weights.
func (am AppModule) WeightedOperations(simState module.SimulationState) []sim.WeightedOperation {
	return simulation.WeightedOperations(simState.AppParams, simState.Cdc,
		am.akeeper, am.keepers)
}
