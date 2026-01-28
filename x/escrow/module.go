package escrow

import (
	"context"
	"encoding/json"
	"fmt"

	"cosmossdk.io/core/appmodule"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/gogoproto/grpc"
	"github.com/gorilla/mux"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/spf13/cobra"
	emodule "pkg.akt.dev/go/node/escrow/module"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	v1 "pkg.akt.dev/go/node/escrow/v1"

	"pkg.akt.dev/node/x/escrow/client/rest"
	"pkg.akt.dev/node/x/escrow/handler"
	"pkg.akt.dev/node/x/escrow/keeper"
)

var (
	_ module.AppModuleBasic   = AppModuleBasic{}
	_ module.HasGenesisBasics = AppModuleBasic{}

	_ appmodule.AppModule        = AppModule{}
	_ module.HasConsensusVersion = AppModule{}
	_ module.HasGenesis          = AppModule{}
	_ module.HasServices         = AppModule{}

	_ module.AppModuleSimulation = AppModule{}
)

// AppModuleBasic defines the basic application module used by the provider module.
type AppModuleBasic struct {
	cdc codec.Codec
}

// Name returns provider module's name
func (AppModuleBasic) Name() string {
	return emodule.ModuleName
}

// RegisterLegacyAminoCodec registers the provider module's types for the given codec.
func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	v1.RegisterLegacyAminoCodec(cdc)
}

// RegisterInterfaces registers the module's interface types
func (b AppModuleBasic) RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	v1.RegisterInterfaces(registry)
}

// DefaultGenesis returns default genesis state as raw bytes for the provider
// module.
func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(DefaultGenesisState())
}

// ValidateGenesis validation check of the Genesis
func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage) error {
	if bz == nil {
		return nil
	}

	var data v1.GenesisState

	err := cdc.UnmarshalJSON(bz, &data)
	if err != nil {
		return fmt.Errorf("failed to unmarshal %s genesis state: %w", emodule.ModuleName, err)
	}

	return ValidateGenesis(&data)
}

// RegisterRESTRoutes registers rest routes for this module
func (AppModuleBasic) RegisterRESTRoutes(clientCtx client.Context, rtr *mux.Router) {
	rest.RegisterRoutes(clientCtx, rtr, emodule.StoreKey)
}

// RegisterGRPCGatewayRoutes registers the gRPC Gateway routes for the provider module.
func (AppModuleBasic) RegisterGRPCGatewayRoutes(clientCtx client.Context, mux *runtime.ServeMux) {
	err := v1.RegisterQueryHandlerClient(context.Background(), mux, v1.NewQueryClient(clientCtx))
	if err != nil {
		panic(fmt.Sprintf("couldn't register provider grpc routes: %s", err.Error()))
	}
}

// GetQueryCmd returns the root query command of this module
func (AppModuleBasic) GetQueryCmd() *cobra.Command {
	panic("akash modules do not export cli commands via cosmos interface")
}

// GetTxCmd returns the transaction commands for this module
func (AppModuleBasic) GetTxCmd() *cobra.Command {
	panic("akash modules do not export cli commands via cosmos interface")
}

// GetQueryClient returns a new query client for this module
func (AppModuleBasic) GetQueryClient(clientCtx client.Context) v1.QueryClient {
	return v1.NewQueryClient(clientCtx)
}

// AppModule implements an application module for the audit module.
type AppModule struct {
	AppModuleBasic
	keeper      keeper.Keeper
	authzKeeper keeper.AuthzKeeper
	bankKeeper  keeper.BankKeeper
}

// NewAppModule creates a new AppModule object
func NewAppModule(cdc codec.Codec, k keeper.Keeper, authzKeeper keeper.AuthzKeeper, bankKeeper keeper.BankKeeper) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{cdc: cdc},
		keeper:         k,
		authzKeeper:    authzKeeper,
		bankKeeper:     bankKeeper,
	}
}

// Name returns the provider module name
func (AppModule) Name() string {
	return emodule.ModuleName
}

// IsOnePerModuleType implements the depinject.OnePerModuleType interface.
func (am AppModule) IsOnePerModuleType() {}

// IsAppModule implements the appmodule.AppModule interface.
func (am AppModule) IsAppModule() {}

// QuerierRoute returns the audit module's querier route name.
func (am AppModule) QuerierRoute() string {
	return emodule.ModuleName
}

// RegisterServices registers the module's services
func (am AppModule) RegisterServices(cfg module.Configurator) {
	v1.RegisterMsgServer(cfg.MsgServer(), handler.NewServer(am.keeper, am.authzKeeper, am.bankKeeper))

	querier := am.keeper.NewQuerier()

	v1.RegisterQueryServer(cfg.QueryServer(), querier)
}

// RegisterQueryService registers a GRPC query service to respond to the
// module-specific GRPC queries.
func (am AppModule) RegisterQueryService(server grpc.Server) {
	querier := keeper.NewQuerier(am.keeper)
	v1.RegisterQueryServer(server, querier)
}

// BeginBlock performs no-op
func (am AppModule) BeginBlock(_ context.Context) error {
	return nil
}

// EndBlock returns the end blocker for the deployment module. It returns no validator
// updates.
func (am AppModule) EndBlock(_ context.Context) error {
	return nil
}

// InitGenesis performs genesis initialization for the escrow module. It returns
// no validator updates.
func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data json.RawMessage) {
	var genesisState v1.GenesisState
	cdc.MustUnmarshalJSON(data, &genesisState)
	InitGenesis(ctx, am.keeper, &genesisState)
}

// ExportGenesis returns the exported genesis state as raw bytes for the audit
// module.
func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) json.RawMessage {
	gs := ExportGenesis(ctx, am.keeper)
	return cdc.MustMarshalJSON(gs)
}

// ConsensusVersion implements module.AppModule#ConsensusVersion
func (am AppModule) ConsensusVersion() uint64 {
	return 3
}

// ____________________________________________________________________________

// RegisterStoreDecoder registers a decoder for take module's types.
func (am AppModule) RegisterStoreDecoder(_ simtypes.StoreDecoderRegistry) {}

// WeightedOperations doesn't return any take module operation.
func (am AppModule) WeightedOperations(_ module.SimulationState) []simtypes.WeightedOperation {
	return []simtypes.WeightedOperation{}
}

// GenerateGenesisState creates a randomized GenState of the staking module.
func (AppModule) GenerateGenesisState(_ *module.SimulationState) {
}

// AppModuleSimulation implements an application simulation module for the audit module.
type AppModuleSimulation struct {
	keeper keeper.Keeper
}

// NewAppModuleSimulation creates a new AppModuleSimulation instance
func NewAppModuleSimulation(k keeper.Keeper) AppModuleSimulation {
	return AppModuleSimulation{
		keeper: k,
	}
}

// // AppModuleSimulation functions
// // GenerateGenesisState creates a randomized GenState of the staking module.
// func (AppModuleSimulation) GenerateGenesisState(simState *module.SimulationState) {
// 	// simulation.RandomizedGenState(simState)
// }
//
// // ProposalContents doesn't return any content functions for governance proposals.
// func (AppModuleSimulation) ProposalContents(_ module.SimulationState) []sim.WeightedProposalContent {
// 	return nil
// }
//
// // RandomizedParams creates randomized staking param changes for the simulator.
// func (AppModuleSimulation) RandomizedParams(r *rand.Rand) []sim.ParamChange {
// 	return nil
// }
//
// // RegisterStoreDecoder registers a decoder for staking module's types
// func (AppModuleSimulation) RegisterStoreDecoder(sdr sdk.StoreDecoderRegistry) {
//
// }
//
// // WeightedOperations returns the all the staking module operations with their respective weights.
// func (am AppModuleSimulation) WeightedOperations(simState module.SimulationState) []sim.WeightedOperation {
// 	return nil
// }
