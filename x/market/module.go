package market

import (
	"context"
	"encoding/json"
	"fmt"

	"cosmossdk.io/core/appmodule"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/spf13/cobra"
	v1 "pkg.akt.dev/go/node/market/v1"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	types "pkg.akt.dev/go/node/market/v1beta5"

	akeeper "pkg.akt.dev/node/x/audit/keeper"
	ekeeper "pkg.akt.dev/node/x/escrow/keeper"
	"pkg.akt.dev/node/x/market/handler"
	"pkg.akt.dev/node/x/market/keeper"
	"pkg.akt.dev/node/x/market/simulation"
)

// type check to ensure the interface is properly implemented
var (
	_ module.AppModuleBasic   = AppModuleBasic{}
	_ module.HasGenesisBasics = AppModuleBasic{}

	_ appmodule.AppModule        = AppModule{}
	_ module.HasConsensusVersion = AppModule{}
	_ module.HasGenesis          = AppModule{}
	_ module.HasServices         = AppModule{}

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
	return v1.ModuleName
}

// RegisterLegacyAminoCodec registers the market module's types for the given codec.
func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	types.RegisterLegacyAminoCodec(cdc) // nolint staticcheck
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
		return fmt.Errorf("failed to unmarshal %s genesis state: %w", v1.ModuleName, err)
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
	authzkeeper authzkeeper.Keeper,
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
			Authz:      authzkeeper,
			Bank:       bkeeper,
		},
	}
}

// Name returns the market module name
func (AppModule) Name() string {
	return v1.ModuleName
}

// IsOnePerModuleType implements the depinject.OnePerModuleType interface.
func (am AppModule) IsOnePerModuleType() {}

// IsAppModule implements the appmodule.AppModule interface.
func (am AppModule) IsAppModule() {}

// RegisterServices registers the module's services
func (am AppModule) RegisterServices(cfg module.Configurator) {
	types.RegisterMsgServer(cfg.MsgServer(), handler.NewServer(am.keepers))
	querier := am.keepers.Market.NewQuerier()
	types.RegisterQueryServer(cfg.QueryServer(), querier)
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

// InitGenesis performs genesis initialization for the market module. It returns
// no validator updates.
func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data json.RawMessage) {
	var genesisState types.GenesisState
	cdc.MustUnmarshalJSON(data, &genesisState)
	InitGenesis(ctx, am.keepers.Market, &genesisState)
}

// ExportGenesis returns the exported genesis state as raw bytes for the market
// module.
func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) json.RawMessage {
	gs := ExportGenesis(ctx, am.keepers.Market)
	return cdc.MustMarshalJSON(gs)
}

// ConsensusVersion implements module.AppModule#ConsensusVersion
func (am AppModule) ConsensusVersion() uint64 {
	return 8
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
func (am AppModule) RegisterStoreDecoder(_ simtypes.StoreDecoderRegistry) {}

// WeightedOperations doesn't return any take module operation.
func (am AppModule) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
	return simulation.WeightedOperations(simState.AppParams, simState.Cdc, am.keepers)
}
