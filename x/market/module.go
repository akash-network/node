package market

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"

	"github.com/gorilla/mux"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/spf13/cobra"

	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	v1beta1types "github.com/akash-network/akash-api/go/node/market/v1beta1"
	v1beta2types "github.com/akash-network/akash-api/go/node/market/v1beta2"
	v1beta3types "github.com/akash-network/akash-api/go/node/market/v1beta3"
	types "github.com/akash-network/akash-api/go/node/market/v1beta4"

	utypes "github.com/akash-network/node/upgrades/types"
	akeeper "github.com/akash-network/node/x/audit/keeper"
	ekeeper "github.com/akash-network/node/x/escrow/keeper"
	"github.com/akash-network/node/x/market/client/cli"
	"github.com/akash-network/node/x/market/client/rest"
	"github.com/akash-network/node/x/market/handler"
	"github.com/akash-network/node/x/market/keeper"
	"github.com/akash-network/node/x/market/simulation"
)

var (
	_ module.AppModule           = AppModule{}
	_ module.AppModuleBasic      = AppModuleBasic{}
	_ module.AppModuleSimulation = AppModuleSimulation{}
)

// AppModuleBasic defines the basic application module used by the market module.
type AppModuleBasic struct {
	cdc codec.Codec
}

// Name returns market module's name
func (AppModuleBasic) Name() string {
	return types.ModuleName
}

// RegisterLegacyAminoCodec registers the market module's types for the given codec.
func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	types.RegisterLegacyAminoCodec(cdc)
}

// RegisterInterfaces registers the module's interface types
func (b AppModuleBasic) RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	types.RegisterInterfaces(registry)
	v1beta3types.RegisterInterfaces(registry)
	v1beta2types.RegisterInterfaces(registry)
	v1beta1types.RegisterInterfaces(registry)
}

// DefaultGenesis returns default genesis state as raw bytes for the market
// module.
func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(DefaultGenesisState())
}

// ValidateGenesis validation check of the Genesis
func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage) error {
	var data types.GenesisState
	err := cdc.UnmarshalJSON(bz, &data)
	if err != nil {
		return fmt.Errorf("failed to unmarshal %s genesis state: %w", types.ModuleName, err)
	}
	return ValidateGenesis(&data)
}

// RegisterRESTRoutes registers rest routes for this module
func (AppModuleBasic) RegisterRESTRoutes(clientCtx client.Context, rtr *mux.Router) {
	rest.RegisterRoutes(clientCtx, rtr, StoreKey)
}

// RegisterGRPCGatewayRoutes registers the gRPC Gateway routes for the market module.
func (AppModuleBasic) RegisterGRPCGatewayRoutes(clientCtx client.Context, mux *runtime.ServeMux) {
	err := types.RegisterQueryHandlerClient(context.Background(), mux, types.NewQueryClient(clientCtx))
	if err != nil {
		panic(fmt.Sprintf("couldn't register market grpc routes: %s", err.Error()))
	}
}

// GetQueryCmd returns the root query command of this module
func (AppModuleBasic) GetQueryCmd() *cobra.Command {
	return cli.GetQueryCmd()
}

// GetTxCmd returns the root tx command of this module
func (AppModuleBasic) GetTxCmd() *cobra.Command {
	return cli.GetTxCmd(StoreKey)
}

// GetQueryClient returns a new query client for this module
func (AppModuleBasic) GetQueryClient(clientCtx client.Context) types.QueryClient {
	return types.NewQueryClient(clientCtx)
}

// AppModule implements an application module for the market module.
type AppModule struct {
	AppModuleBasic
	keepers handler.Keepers
}

// NewAppModule creates a new AppModule object
func NewAppModule(
	cdc codec.Codec,
	keeper keeper.IKeeper,
	ekeeper ekeeper.Keeper,
	akeeper akeeper.Keeper,
	dkeeper handler.DeploymentKeeper,
	pkeeper handler.ProviderKeeper,
	bkeeper bankkeeper.Keeper,
) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{cdc: cdc},
		keepers: handler.Keepers{
			Escrow:     ekeeper,
			Audit:      akeeper,
			Market:     keeper,
			Deployment: dkeeper,
			Provider:   pkeeper,
			Bank:       bkeeper,
		},
	}
}

// Name returns the market module name
func (AppModule) Name() string {
	return types.ModuleName
}

// RegisterInvariants registers module invariants
func (am AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}

// Route returns the message routing key for the market module.
func (am AppModule) Route() sdk.Route {
	return sdk.NewRoute(types.RouterKey, handler.NewHandler(am.keepers))
}

// QuerierRoute returns the market module's querier route name.
func (am AppModule) QuerierRoute() string {
	return ""
}

// LegacyQuerierHandler returns the sdk.Querier for market module
func (am AppModule) LegacyQuerierHandler(_ *codec.LegacyAmino) sdk.Querier {
	return nil
}

// RegisterServices registers the module's services
func (am AppModule) RegisterServices(cfg module.Configurator) {
	types.RegisterMsgServer(cfg.MsgServer(), handler.NewServer(am.keepers))
	querier := am.keepers.Market.NewQuerier()
	types.RegisterQueryServer(cfg.QueryServer(), querier)

	utypes.ModuleMigrations(ModuleName, am.keepers.Market, func(name string, forVersion uint64, handler module.MigrationHandler) {
		if err := cfg.RegisterMigration(name, forVersion, handler); err != nil {
			panic(err)
		}
	})
}

// BeginBlock performs no-op
func (am AppModule) BeginBlock(_ sdk.Context, _ abci.RequestBeginBlock) {}

// EndBlock returns the end blocker for the market module. It returns no validator
// updates.
func (am AppModule) EndBlock(_ sdk.Context, _ abci.RequestEndBlock) []abci.ValidatorUpdate {
	return []abci.ValidatorUpdate{}
}

// InitGenesis performs genesis initialization for the market module. It returns
// no validator updates.
func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data json.RawMessage) []abci.ValidatorUpdate {
	var genesisState types.GenesisState
	cdc.MustUnmarshalJSON(data, &genesisState)
	return InitGenesis(ctx, am.keepers.Market, &genesisState)
}

// ExportGenesis returns the exported genesis state as raw bytes for the market
// module.
func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) json.RawMessage {
	gs := ExportGenesis(ctx, am.keepers.Market)
	return cdc.MustMarshalJSON(gs)
}

// ConsensusVersion implements module.AppModule#ConsensusVersion
func (am AppModule) ConsensusVersion() uint64 {
	return utypes.ModuleVersion(ModuleName)
}

// AppModuleSimulation implements an application simulation module for the market module.
type AppModuleSimulation struct {
	keepers handler.Keepers
	akeeper govtypes.AccountKeeper
}

// NewAppModuleSimulation creates a new AppModuleSimulation instance
func NewAppModuleSimulation(
	keeper keeper.IKeeper,
	akeeper govtypes.AccountKeeper,
	dkeeper handler.DeploymentKeeper,
	pkeeper handler.ProviderKeeper,
	bkeeper bankkeeper.Keeper,
) AppModuleSimulation {
	return AppModuleSimulation{
		keepers: handler.Keepers{
			Market:     keeper,
			Deployment: dkeeper,
			Provider:   pkeeper,
			Bank:       bkeeper,
		},
		akeeper: akeeper,
	}
}

// AppModuleSimulation functions

// GenerateGenesisState creates a randomized GenState of the staking module.
func (AppModuleSimulation) GenerateGenesisState(simState *module.SimulationState) {
	simulation.RandomizedGenState(simState)
}

// ProposalContents doesn't return any content functions for governance proposals.
func (AppModuleSimulation) ProposalContents(_ module.SimulationState) []simtypes.WeightedProposalContent {
	return nil
}

// RandomizedParams creates randomized staking param changes for the simulator.
func (AppModuleSimulation) RandomizedParams(_ *rand.Rand) []simtypes.ParamChange {
	return nil
}

// RegisterStoreDecoder registers a decoder for staking module's types
func (AppModuleSimulation) RegisterStoreDecoder(_ sdk.StoreDecoderRegistry) {
	// sdr[StoreKey] = simulation.DecodeStore
}

// WeightedOperations returns the all the staking module operations with their respective weights.
func (am AppModuleSimulation) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
	return simulation.WeightedOperations(simState.AppParams, simState.Cdc,
		am.akeeper, am.keepers)
}
