package provider

//import (
//	"context"
//	"encoding/json"
//	"fmt"
//
//	"cosmossdk.io/core/appmodule"
//	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
//	"github.com/grpc-ecosystem/grpc-gateway/runtime"
//	"github.com/spf13/cobra"
//
//	abci "github.com/cometbft/cometbft/abci/types"
//
//	"github.com/cosmos/cosmos-sdk/client"
//	"github.com/cosmos/cosmos-sdk/codec"
//	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
//	sdk "github.com/cosmos/cosmos-sdk/types"
//	"github.com/cosmos/cosmos-sdk/types/module"
//
//	types "pkg.akt.dev/go/node/staking/v1beta3"
//
//<<<<<<< HEAD
//	"github.com/akash-network/node/x/staking/keeper"
//	"github.com/akash-network/node/x/staking/simulation"
//||||||| parent of c489be40 (feat: sdk-50)
//	utypes "github.com/akash-network/node/upgrades/types"
//	"github.com/akash-network/node/x/staking/keeper"
//	"github.com/akash-network/node/x/staking/simulation"
//=======
//	utypes "pkg.akt.dev/node/upgrades/types"
//	"pkg.akt.dev/node/x/staking/handler"
//	"pkg.akt.dev/node/x/staking/keeper"
//	"pkg.akt.dev/node/x/staking/simulation"
//>>>>>>> c489be40 (feat: sdk-50)
//)
//
//var (
//	_ module.AppModuleBasic = AppModuleBasic{}
//
//	_ module.BeginBlockAppModule = AppModule{}
//	_ appmodule.AppModule        = AppModule{}
//	_ module.AppModuleSimulation = AppModule{}
//)
//
//// AppModuleBasic defines the basic application module used by the provider module.
//type AppModuleBasic struct {
//	cdc codec.Codec
//}
//
//// AppModule implements an application module for the provider module.
//type AppModule struct {
//	AppModuleBasic
//	keeper keeper.IKeeper
//}
//
//// Name returns provider module's name
//func (AppModuleBasic) Name() string {
//	return types.ModuleName
//}
//
//// RegisterLegacyAminoCodec registers the provider module's types for the given codec.
////
//// Deprecated: RegisterLegacyAminoCodec is deprecated
//func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
//	types.RegisterLegacyAminoCodec(cdc)
//}
//
//// RegisterInterfaces registers the module's interface types
//func (b AppModuleBasic) RegisterInterfaces(r cdctypes.InterfaceRegistry) {
//	types.RegisterInterfaces(r)
//}
//
//// DefaultGenesis returns default genesis state as raw bytes for the provider
//// module.
//func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
//	return cdc.MustMarshalJSON(DefaultGenesisState())
//}
//
//// ValidateGenesis validation check of the Genesis
//func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage) error {
//	var data types.GenesisState
//	err := cdc.UnmarshalJSON(bz, &data)
//	if err != nil {
//		return fmt.Errorf("failed to unmarshal %s genesis state: %v", types.ModuleName, err)
//	}
//	return ValidateGenesis(&data)
//}
//
//// RegisterGRPCGatewayRoutes registers the gRPC Gateway routes for the provider module.
//func (AppModuleBasic) RegisterGRPCGatewayRoutes(cctx client.Context, mux *runtime.ServeMux) {
//	if err := types.RegisterQueryHandlerClient(context.Background(), mux, types.NewQueryClient(cctx)); err != nil {
//		panic(err)
//	}
//}
//
//// GetQueryCmd returns the root query command of this module
//func (AppModuleBasic) GetQueryCmd() *cobra.Command {
//	return nil
//}
//
//// GetTxCmd returns the transaction commands for this module
//func (AppModuleBasic) GetTxCmd() *cobra.Command {
//	return nil
//}
//
//<<<<<<< HEAD
//// AppModule implements an application module for the provider module.
//type AppModule struct {
//	AppModuleBasic
//	keeper keeper.IKeeper
//}
//
//||||||| parent of c489be40 (feat: sdk-50)
//// GetQueryClient returns a new query client for this module
//// func (AppModuleBasic) GetQueryClient(clientCtx client.Context) types.QueryClient {
//// 	return types.NewQueryClient(clientCtx)
//// }
//
//// AppModule implements an application module for the provider module.
//type AppModule struct {
//	AppModuleBasic
//	keeper keeper.IKeeper
//}
//
//=======
//>>>>>>> c489be40 (feat: sdk-50)
//// NewAppModule creates a new AppModule object
//func NewAppModule(cdc codec.Codec, k keeper.IKeeper) AppModule {
//	return AppModule{
//		AppModuleBasic: AppModuleBasic{cdc: cdc},
//		keeper:         k,
//	}
//}
//
//// Name returns the provider module name
//func (AppModule) Name() string {
//	return types.ModuleName
//}
//
//// IsOnePerModuleType implements the depinject.OnePerModuleType interface.
//func (am AppModule) IsOnePerModuleType() {}
//
//// IsAppModule implements the appmodule.AppModule interface.
//func (am AppModule) IsAppModule() {}
//
//// RegisterInvariants registers module invariants
//func (am AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}
//
//// RegisterServices registers the module's services
//<<<<<<< HEAD
//func (am AppModule) RegisterServices(_ module.Configurator) {
//||||||| parent of c489be40 (feat: sdk-50)
//func (am AppModule) RegisterServices(cfg module.Configurator) {
//	utypes.ModuleMigrations(ModuleName, am.keeper, func(name string, forVersion uint64, handler module.MigrationHandler) {
//		if err := cfg.RegisterMigration(name, forVersion, handler); err != nil {
//			panic(err)
//		}
//	})
//=======
//func (am AppModule) RegisterServices(cfg module.Configurator) {
//	types.RegisterMsgServer(cfg.MsgServer(), handler.NewMsgServerImpl(am.keeper))
//
//	querier := am.keeper.NewQuerier()
//
//	types.RegisterQueryServer(cfg.QueryServer(), querier)
//
//	utypes.ModuleMigrations(ModuleName, am.keeper, func(name string, forVersion uint64, handler module.MigrationHandler) {
//		if err := cfg.RegisterMigration(name, forVersion, handler); err != nil {
//			panic(err)
//		}
//	})
//>>>>>>> c489be40 (feat: sdk-50)
//}
//
//// BeginBlock performs no-op
//func (am AppModule) BeginBlock(_ sdk.Context, _ abci.RequestBeginBlock) {}
//
//// EndBlock returns the end blocker for the provider module. It returns no validator
//// updates.
//func (am AppModule) EndBlock(_ sdk.Context, _ abci.RequestEndBlock) []abci.ValidatorUpdate {
//	return []abci.ValidatorUpdate{}
//}
//
//// InitGenesis performs genesis initialization for the provider module. It returns
//// no validator updates.
//func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data json.RawMessage) []abci.ValidatorUpdate {
//	var genesisState types.GenesisState
//	cdc.MustUnmarshalJSON(data, &genesisState)
//	return InitGenesis(ctx, am.keeper, &genesisState)
//}
//
//// ExportGenesis returns the exported genesis state as raw bytes for the provider
//// module.
//func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) json.RawMessage {
//	gs := ExportGenesis(ctx, am.keeper)
//	return cdc.MustMarshalJSON(gs)
//}
//
//// ConsensusVersion implements module.AppModule#ConsensusVersion
//func (am AppModule) ConsensusVersion() uint64 {
//<<<<<<< HEAD
//	return 1
//}
//
//// ____________________________________________________________________________
//
//// AppModuleSimulation implements an application simulation module for the provider module.
//type AppModuleSimulation struct {
//	keeper keeper.IKeeper
//}
//
//// NewAppModuleSimulation creates a new AppModuleSimulation instance
//func NewAppModuleSimulation(k keeper.IKeeper) AppModuleSimulation {
//	return AppModuleSimulation{
//		keeper: k,
//	}
//||||||| parent of c489be40 (feat: sdk-50)
//	return utypes.ModuleVersion(ModuleName)
//}
//
//// ____________________________________________________________________________
//
//// AppModuleSimulation implements an application simulation module for the provider module.
//type AppModuleSimulation struct {
//	keeper keeper.IKeeper
//}
//
//// NewAppModuleSimulation creates a new AppModuleSimulation instance
//func NewAppModuleSimulation(k keeper.IKeeper) AppModuleSimulation {
//	return AppModuleSimulation{
//		keeper: k,
//	}
//=======
//	return 1
//	// return utypes.ModuleVersion(ModuleName)
//>>>>>>> c489be40 (feat: sdk-50)
//}
//
//// AppModuleSimulation functions
//
//// GenerateGenesisState creates a randomized GenState of the staking module.
//func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
//	simulation.RandomizedGenState(simState)
//}
//
//// ProposalMsgs returns msgs used for governance proposals for simulations.
//func (AppModule) ProposalMsgs(_ module.SimulationState) []simtypes.WeightedProposalMsg {
//	return simulation.ProposalMsgs()
//}
//
//// RegisterStoreDecoder registers a decoder for staking module's types.
//func (am AppModule) RegisterStoreDecoder(_ sdk.StoreDecoderRegistry) {
//	// sdr[types.StoreKey] = simulation.NewDecodeStore(am.cdc)
//}
//
//// WeightedOperations doesn't return any staking module operation.
//func (am AppModule) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
//	return simulation.WeightedOperations(simState.AppParams, simState.Cdc, am.keeper)
//}
//import (
//	"context"
//	"encoding/json"
//	"fmt"
//
//	"cosmossdk.io/core/appmodule"
//	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
//	"github.com/grpc-ecosystem/grpc-gateway/runtime"
//	"github.com/spf13/cobra"
//
//	abci "github.com/cometbft/cometbft/abci/types"
//
//	"github.com/cosmos/cosmos-sdk/client"
//	"github.com/cosmos/cosmos-sdk/codec"
//	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
//	sdk "github.com/cosmos/cosmos-sdk/types"
//	"github.com/cosmos/cosmos-sdk/types/module"
//
//	types "pkg.akt.dev/go/node/staking/v1beta3"
//
//<<<<<<< HEAD
//	"github.com/akash-network/node/x/staking/keeper"
//	"github.com/akash-network/node/x/staking/simulation"
//||||||| parent of c489be40 (feat: sdk-50)
//	utypes "github.com/akash-network/node/upgrades/types"
//	"github.com/akash-network/node/x/staking/keeper"
//	"github.com/akash-network/node/x/staking/simulation"
//=======
//	utypes "pkg.akt.dev/node/upgrades/types"
//	"pkg.akt.dev/node/x/staking/handler"
//	"pkg.akt.dev/node/x/staking/keeper"
//	"pkg.akt.dev/node/x/staking/simulation"
//>>>>>>> c489be40 (feat: sdk-50)
//)
//
//var (
//	_ module.AppModuleBasic = AppModuleBasic{}
//
//	_ module.BeginBlockAppModule = AppModule{}
//	_ appmodule.AppModule        = AppModule{}
//	_ module.AppModuleSimulation = AppModule{}
//)
//
//// AppModuleBasic defines the basic application module used by the provider module.
//type AppModuleBasic struct {
//	cdc codec.Codec
//}
//
//// AppModule implements an application module for the provider module.
//type AppModule struct {
//	AppModuleBasic
//	keeper keeper.IKeeper
//}
//
//// Name returns provider module's name
//func (AppModuleBasic) Name() string {
//	return types.ModuleName
//}
//
//// RegisterLegacyAminoCodec registers the provider module's types for the given codec.
////
//// Deprecated: RegisterLegacyAminoCodec is deprecated
//func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
//	types.RegisterLegacyAminoCodec(cdc)
//}
//
//// RegisterInterfaces registers the module's interface types
//func (b AppModuleBasic) RegisterInterfaces(r cdctypes.InterfaceRegistry) {
//	types.RegisterInterfaces(r)
//}
//
//// DefaultGenesis returns default genesis state as raw bytes for the provider
//// module.
//func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
//	return cdc.MustMarshalJSON(DefaultGenesisState())
//}
//
//// ValidateGenesis validation check of the Genesis
//func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage) error {
//	var data types.GenesisState
//	err := cdc.UnmarshalJSON(bz, &data)
//	if err != nil {
//		return fmt.Errorf("failed to unmarshal %s genesis state: %v", types.ModuleName, err)
//	}
//	return ValidateGenesis(&data)
//}
//
//// RegisterGRPCGatewayRoutes registers the gRPC Gateway routes for the provider module.
//func (AppModuleBasic) RegisterGRPCGatewayRoutes(cctx client.Context, mux *runtime.ServeMux) {
//	if err := types.RegisterQueryHandlerClient(context.Background(), mux, types.NewQueryClient(cctx)); err != nil {
//		panic(err)
//	}
//}
//
//// GetQueryCmd returns the root query command of this module
//func (AppModuleBasic) GetQueryCmd() *cobra.Command {
//	return nil
//}
//
//// GetTxCmd returns the transaction commands for this module
//func (AppModuleBasic) GetTxCmd() *cobra.Command {
//	return nil
//}
//
//<<<<<<< HEAD
//// AppModule implements an application module for the provider module.
//type AppModule struct {
//	AppModuleBasic
//	keeper keeper.IKeeper
//}
//
//||||||| parent of c489be40 (feat: sdk-50)
//// GetQueryClient returns a new query client for this module
//// func (AppModuleBasic) GetQueryClient(clientCtx client.Context) types.QueryClient {
//// 	return types.NewQueryClient(clientCtx)
//// }
//
//// AppModule implements an application module for the provider module.
//type AppModule struct {
//	AppModuleBasic
//	keeper keeper.IKeeper
//}
//
//=======
//>>>>>>> c489be40 (feat: sdk-50)
//// NewAppModule creates a new AppModule object
//func NewAppModule(cdc codec.Codec, k keeper.IKeeper) AppModule {
//	return AppModule{
//		AppModuleBasic: AppModuleBasic{cdc: cdc},
//		keeper:         k,
//	}
//}
//
//// Name returns the provider module name
//func (AppModule) Name() string {
//	return types.ModuleName
//}
//
//// IsOnePerModuleType implements the depinject.OnePerModuleType interface.
//func (am AppModule) IsOnePerModuleType() {}
//
//// IsAppModule implements the appmodule.AppModule interface.
//func (am AppModule) IsAppModule() {}
//
//// RegisterInvariants registers module invariants
//func (am AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}
//
//// RegisterServices registers the module's services
//<<<<<<< HEAD
//func (am AppModule) RegisterServices(_ module.Configurator) {
//||||||| parent of c489be40 (feat: sdk-50)
//func (am AppModule) RegisterServices(cfg module.Configurator) {
//	utypes.ModuleMigrations(ModuleName, am.keeper, func(name string, forVersion uint64, handler module.MigrationHandler) {
//		if err := cfg.RegisterMigration(name, forVersion, handler); err != nil {
//			panic(err)
//		}
//	})
//=======
//func (am AppModule) RegisterServices(cfg module.Configurator) {
//	types.RegisterMsgServer(cfg.MsgServer(), handler.NewMsgServerImpl(am.keeper))
//
//	querier := am.keeper.NewQuerier()
//
//	types.RegisterQueryServer(cfg.QueryServer(), querier)
//
//	utypes.ModuleMigrations(ModuleName, am.keeper, func(name string, forVersion uint64, handler module.MigrationHandler) {
//		if err := cfg.RegisterMigration(name, forVersion, handler); err != nil {
//			panic(err)
//		}
//	})
//>>>>>>> c489be40 (feat: sdk-50)
//}
//
//// BeginBlock performs no-op
//func (am AppModule) BeginBlock(_ sdk.Context, _ abci.RequestBeginBlock) {}
//
//// EndBlock returns the end blocker for the provider module. It returns no validator
//// updates.
//func (am AppModule) EndBlock(_ sdk.Context, _ abci.RequestEndBlock) []abci.ValidatorUpdate {
//	return []abci.ValidatorUpdate{}
//}
//
//// InitGenesis performs genesis initialization for the provider module. It returns
//// no validator updates.
//func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data json.RawMessage) []abci.ValidatorUpdate {
//	var genesisState types.GenesisState
//	cdc.MustUnmarshalJSON(data, &genesisState)
//	return InitGenesis(ctx, am.keeper, &genesisState)
//}
//
//// ExportGenesis returns the exported genesis state as raw bytes for the provider
//// module.
//func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) json.RawMessage {
//	gs := ExportGenesis(ctx, am.keeper)
//	return cdc.MustMarshalJSON(gs)
//}
//
//// ConsensusVersion implements module.AppModule#ConsensusVersion
//func (am AppModule) ConsensusVersion() uint64 {
//<<<<<<< HEAD
//	return 1
//}
//
//// ____________________________________________________________________________
//
//// AppModuleSimulation implements an application simulation module for the provider module.
//type AppModuleSimulation struct {
//	keeper keeper.IKeeper
//}
//
//// NewAppModuleSimulation creates a new AppModuleSimulation instance
//func NewAppModuleSimulation(k keeper.IKeeper) AppModuleSimulation {
//	return AppModuleSimulation{
//		keeper: k,
//	}
//||||||| parent of c489be40 (feat: sdk-50)
//	return utypes.ModuleVersion(ModuleName)
//}
//
//// ____________________________________________________________________________
//
//// AppModuleSimulation implements an application simulation module for the provider module.
//type AppModuleSimulation struct {
//	keeper keeper.IKeeper
//}
//
//// NewAppModuleSimulation creates a new AppModuleSimulation instance
//func NewAppModuleSimulation(k keeper.IKeeper) AppModuleSimulation {
//	return AppModuleSimulation{
//		keeper: k,
//	}
//=======
//	return 1
//	// return utypes.ModuleVersion(ModuleName)
//>>>>>>> c489be40 (feat: sdk-50)
//}
//
//// AppModuleSimulation functions
//
//// GenerateGenesisState creates a randomized GenState of the staking module.
//func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
//	simulation.RandomizedGenState(simState)
//}
//
//// ProposalMsgs returns msgs used for governance proposals for simulations.
//func (AppModule) ProposalMsgs(_ module.SimulationState) []simtypes.WeightedProposalMsg {
//	return simulation.ProposalMsgs()
//}
//
//// RegisterStoreDecoder registers a decoder for staking module's types.
//func (am AppModule) RegisterStoreDecoder(_ sdk.StoreDecoderRegistry) {
//	// sdr[types.StoreKey] = simulation.NewDecodeStore(am.cdc)
//}
//
//// WeightedOperations doesn't return any staking module operation.
//func (am AppModule) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
//	return simulation.WeightedOperations(simState.AppParams, simState.Cdc, am.keeper)
//}
