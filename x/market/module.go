package market

import (
	"context"
	"encoding/json"
	"fmt"

	"cosmossdk.io/core/appmodule"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/spf13/cobra"

	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	types "pkg.akt.dev/go/node/market/v1beta5"

	utypes "pkg.akt.dev/akashd/upgrades/types"
	akeeper "pkg.akt.dev/akashd/x/audit/keeper"
	ekeeper "pkg.akt.dev/akashd/x/escrow/keeper"
	"pkg.akt.dev/akashd/x/market/handler"
	"pkg.akt.dev/akashd/x/market/keeper"
	"pkg.akt.dev/akashd/x/market/simulation"
)

var (
	_ module.AppModuleBasic = AppModuleBasic{}

	_ module.BeginBlockAppModule = AppModule{}
	_ appmodule.AppModule        = AppModule{}
	_ module.AppModuleSimulation = AppModule{}
)

// AppModuleBasic defines the basic application module used by the market module.
type AppModuleBasic struct {
	cdc codec.Codec
}

// AppModule implements an application module for the market module.
type AppModule struct {
	AppModuleBasic
	keepers handler.Keepers
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

// RegisterGRPCGatewayRoutes registers the gRPC Gateway routes for the market module.
func (AppModuleBasic) RegisterGRPCGatewayRoutes(clientCtx client.Context, mux *runtime.ServeMux) {
	err := types.RegisterQueryHandlerClient(context.Background(), mux, types.NewQueryClient(clientCtx))
	if err != nil {
		panic(fmt.Sprintf("couldn't register market grpc routes: %s", err.Error()))
	}
}

// GetQueryCmd returns the root query command of this module
func (AppModuleBasic) GetQueryCmd() *cobra.Command {
	panic("akash modules do not export cli commands via cosmos interface")
}

// GetTxCmd returns the root tx command of this module
func (AppModuleBasic) GetTxCmd() *cobra.Command {
	panic("akash modules do not export cli commands via cosmos interface")
}

// GetQueryClient returns a new query client for this module
func (AppModuleBasic) GetQueryClient(clientCtx client.Context) types.QueryClient {
	return types.NewQueryClient(clientCtx)
}

// NewAppModule creates a new AppModule object
func NewAppModule(
	cdc codec.Codec,
	keeper keeper.IKeeper,
	ekeeper ekeeper.Keeper,
	akeeper akeeper.Keeper,
	dkeeper handler.DeploymentKeeper,
	pkeeper handler.ProviderKeeper,
	acckeeper govtypes.AccountKeeper,
	bkeeper bankkeeper.Keeper,
) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{cdc: cdc},
		keepers: handler.Keepers{
			Account:    acckeeper,
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

// IsOnePerModuleType implements the depinject.OnePerModuleType interface.
func (am AppModule) IsOnePerModuleType() {}

// IsAppModule implements the appmodule.AppModule interface.
func (am AppModule) IsAppModule() {}

// RegisterInvariants registers module invariants
func (am AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}

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
	return 5
	// return utypes.ModuleVersion(ModuleName)
}

// AppModuleSimulation functions

// GenerateGenesisState creates a randomized GenState of the staking module.
func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
	simulation.RandomizedGenState(simState)
}

// ProposalMsgs returns msgs used for governance proposals for simulations.
func (AppModule) ProposalMsgs(_ module.SimulationState) []simtypes.WeightedProposalMsg {
	return simulation.ProposalMsgs()
}

// RegisterStoreDecoder registers a decoder for take module's types.
func (am AppModule) RegisterStoreDecoder(sdr sdk.StoreDecoderRegistry) {
	// sdr[types.StoreKey] = simulation.NewDecodeStore(am.cdc)
}

// WeightedOperations doesn't return any take module operation.
func (am AppModule) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
	return simulation.WeightedOperations(simState.AppParams, simState.Cdc, am.keepers)
}

// // AppModuleSimulation functions
//
// // GenerateGenesisState creates a randomized GenState of the staking module.
// func (AppModuleSimulation) GenerateGenesisState(simState *module.SimulationState) {
// 	simulation.RandomizedGenState(simState)
// }
//
// // ProposalContents doesn't return any content functions for governance proposals.
// func (AppModuleSimulation) ProposalContents(_ module.SimulationState) []simtypes.WeightedProposalContent {
// 	return nil
// }
//
// // RandomizedParams creates randomized staking param changes for the simulator.
// func (AppModuleSimulation) RandomizedParams(_ *rand.Rand) []simtypes.ParamChange {
// 	return nil
// }
//
// // RegisterStoreDecoder registers a decoder for staking module's types
// func (AppModuleSimulation) RegisterStoreDecoder(_ sdk.StoreDecoderRegistry) {
// 	// sdr[StoreKey] = simulation.DecodeStore
// }
//
// // WeightedOperations returns the all the staking module operations with their respective weights.
// func (am AppModuleSimulation) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
// 	return simulation.WeightedOperations(simState.AppParams, simState.Cdc,
// 		am.akeeper, am.keepers)
// }
