package deployment

import (
	"encoding/json"
	"math/rand"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/gogo/protobuf/grpc"
	"github.com/gorilla/mux"
	"github.com/ovrclk/akash/x/deployment/client/cli"
	"github.com/ovrclk/akash/x/deployment/client/rest"
	"github.com/ovrclk/akash/x/deployment/handler"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/query"
	"github.com/ovrclk/akash/x/deployment/simulation"
	"github.com/ovrclk/akash/x/deployment/types"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sim "github.com/cosmos/cosmos-sdk/types/simulation"
	abci "github.com/tendermint/tendermint/abci/types"
)

// type check to ensure the interface is properly implemented
var (
	_ module.AppModule           = AppModule{}
	_ module.AppModuleBasic      = AppModuleBasic{}
	_ module.AppModuleSimulation = AppModuleSimulation{}
)

// AppModuleBasic defines the basic application module used by the deployment module.
type AppModuleBasic struct {
	cdc codec.Marshaler
}

// Name returns deployment module's name
func (AppModuleBasic) Name() string {
	return types.ModuleName
}

// RegisterCodec registers the deployment module's types for the given codec.
func (AppModuleBasic) RegisterCodec(cdc *codec.Codec) {
	types.RegisterCodec(cdc)
}

// RegisterInterfaces registers the module's interface types
func (b AppModuleBasic) RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	types.RegisterInterfaces(registry)
}

// DefaultGenesis returns default genesis state as raw bytes for the deployment
// module.
func (AppModuleBasic) DefaultGenesis(cdc codec.JSONMarshaler) json.RawMessage {
	return cdc.MustMarshalJSON(DefaultGenesisState())
}

// ValidateGenesis does validation check of the Genesis and returns error incase of failure
func (AppModuleBasic) ValidateGenesis(cdc codec.JSONMarshaler, config client.TxEncodingConfig, bz json.RawMessage) error {
	var data GenesisState
	err := cdc.UnmarshalJSON(bz, &data)
	if err != nil {
		return errors.Errorf("failed to unmarshal %s genesis state: %w", types.ModuleName, err)
	}
	return ValidateGenesis(data)
}

// RegisterRESTRoutes registers rest routes for this module
func (AppModuleBasic) RegisterRESTRoutes(clientCtx client.Context, rtr *mux.Router) {
	rest.RegisterRoutes(clientCtx, rtr, StoreKey)
}

// GetQueryCmd get the root query command of this module
func (AppModuleBasic) GetQueryCmd() *cobra.Command {
	// TODO: Change parameters when working on queries
	return cli.GetQueryCmd(StoreKey, nil)
}

// GetTxCmd get the root tx command of this module
func (AppModuleBasic) GetTxCmd() *cobra.Command {
	// TODO: Change parameters when working on txs
	return cli.GetTxCmd(StoreKey, nil)
}

// // GetQueryClient returns a new query client for this module
// func (AppModuleBasic) GetQueryClient(ctx context.CLIContext) query.Client {
// 	return query.NewClient(ctx, StoreKey)
// }

// AppModule implements an application module for the deployment module.
type AppModule struct {
	AppModuleBasic
	keeper     keeper.Keeper
	mkeeper    handler.MarketKeeper
	coinKeeper bankkeeper.Keeper
}

// NewAppModule creates a new AppModule Object
func NewAppModule(cdc codec.Marshaler, k keeper.Keeper, mkeeper handler.MarketKeeper, bankKeeper bankkeeper.Keeper) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{cdc: cdc},
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
func (am AppModule) Route() sdk.Route {
	return sdk.NewRoute(types.RouterKey, handler.NewHandler(am.keeper, am.mkeeper))
}

// // NewHandler returns an sdk.Handler for the deployment module.
// func (am AppModule) NewHandler() sdk.Handler {
// 	return handler.NewHandler(am.keeper, am.mkeeper)
// }

// QuerierRoute returns the deployment module's querier route name.
func (am AppModule) QuerierRoute() string {
	return types.ModuleName
}

// LegacyQuerierHandler returns the sdk.Querier for market module
func (am AppModule) LegacyQuerierHandler(legacyQuerierCdc codec.JSONMarshaler) sdk.Querier {
	return query.NewQuerier(am.keeper, legacyQuerierCdc)
}

// RegisterQueryService registers a GRPC query service to respond to the
// module-specific GRPC queries.
func (am AppModule) RegisterQueryService(_ grpc.Server) {
	// TODO: Add grpc querier
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
func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONMarshaler, data json.RawMessage) []abci.ValidatorUpdate {
	var genesisState GenesisState
	cdc.MustUnmarshalJSON(data, &genesisState)
	return InitGenesis(ctx, am.keeper, genesisState)
}

// ExportGenesis returns the exported genesis state as raw bytes for the deployment
// module.
func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONMarshaler) json.RawMessage {
	gs := ExportGenesis(ctx, am.keeper)
	return cdc.MustMarshalJSON(gs)
}

//____________________________________________________________________________

// AppModuleSimulation implements an application simulation module for the deployment module.
type AppModuleSimulation struct {
	keeper  keeper.Keeper
	akeeper govtypes.AccountKeeper
	bkeeper bankkeeper.Keeper
}

// NewAppModuleSimulation creates a new AppModuleSimulation instance
func NewAppModuleSimulation(k keeper.Keeper, akeeper govtypes.AccountKeeper, bankKeeper bankkeeper.Keeper) AppModuleSimulation {
	return AppModuleSimulation{
		keeper:  k,
		akeeper: akeeper,
		bkeeper: bankKeeper,
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
		am.akeeper, am.bkeeper, am.keeper)
}
