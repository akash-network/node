package deployment

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/spf13/cobra"
	"pkg.akt.dev/go/node/migrate"

	"cosmossdk.io/core/appmodule"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	v1 "pkg.akt.dev/go/node/deployment/v1"
	types "pkg.akt.dev/go/node/deployment/v1beta4"

	"pkg.akt.dev/node/v2/x/deployment/handler"
	"pkg.akt.dev/node/v2/x/deployment/keeper"
	"pkg.akt.dev/node/v2/x/deployment/simulation"
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

// AppModuleBasic defines the basic application module used by the deployment module.
type AppModuleBasic struct {
	cdc codec.Codec
}

// AppModule implements an application module for the deployment module.
type AppModule struct {
	AppModuleBasic
	keeper      keeper.IKeeper
	mkeeper     handler.MarketKeeper
	ekeeper     handler.EscrowKeeper
	coinKeeper  bankkeeper.Keeper
	authzKeeper handler.AuthzKeeper
	acckeeper   govtypes.AccountKeeper
}

// Name returns deployment module's name
func (AppModuleBasic) Name() string {
	return v1.ModuleName
}

// RegisterLegacyAminoCodec registers the deployment module's types for the given codec.
func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	types.RegisterLegacyAminoCodec(cdc) // nolint staticcheck
}

// RegisterInterfaces registers the module's interface types
func (b AppModuleBasic) RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	types.RegisterInterfaces(registry)

	migrate.RegisterDeploymentInterfaces(registry)
}

// DefaultGenesis returns default genesis state as raw bytes for the deployment
// module.
func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(DefaultGenesisState())
}

// ValidateGenesis does validation check of the Genesis and returns error in case of failure
func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage) error {
	var data types.GenesisState
	err := cdc.UnmarshalJSON(bz, &data)
	if err != nil {
		return fmt.Errorf("failed to unmarshal %s genesis state: %v", v1.ModuleName, err)
	}
	return ValidateGenesis(&data)
}

// RegisterGRPCGatewayRoutes registers the gRPC Gateway routes for the deployment module.
func (AppModuleBasic) RegisterGRPCGatewayRoutes(clientCtx client.Context, mux *runtime.ServeMux) {
	err := types.RegisterQueryHandlerClient(context.Background(), mux, types.NewQueryClient(clientCtx))
	if err != nil {
		panic(fmt.Sprintf("couldn't register deployment grpc routes: %s", err.Error()))
	}
}

// GetQueryCmd get the root query command of this module
func (AppModuleBasic) GetQueryCmd() *cobra.Command {
	panic("akash modules do not export cli commands via cosmos interface")
}

// GetTxCmd get the root tx command of this module
func (AppModuleBasic) GetTxCmd() *cobra.Command {
	panic("akash modules do not export cli commands via cosmos interface")
}

// NewAppModule creates a new AppModule Object
func NewAppModule(
	cdc codec.Codec,
	k keeper.IKeeper,
	mkeeper handler.MarketKeeper,
	ekeeper handler.EscrowKeeper,
	acckeeper govtypes.AccountKeeper,
	bankKeeper bankkeeper.Keeper,
	authzKeeper handler.AuthzKeeper,
) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{cdc: cdc},
		keeper:         k,
		mkeeper:        mkeeper,
		ekeeper:        ekeeper,
		acckeeper:      acckeeper,
		coinKeeper:     bankKeeper,
		authzKeeper:    authzKeeper,
	}
}

// Name returns the deployment module name
func (AppModule) Name() string {
	return v1.ModuleName
}

// IsOnePerModuleType implements the depinject.OnePerModuleType interface.
func (am AppModule) IsOnePerModuleType() {}

// IsAppModule implements the appmodule.AppModule interface.
func (am AppModule) IsAppModule() {}

// RegisterServices registers the module's services
func (am AppModule) RegisterServices(cfg module.Configurator) {
	types.RegisterMsgServer(cfg.MsgServer(), handler.NewServer(am.keeper, am.mkeeper, am.ekeeper))

	querier := am.keeper.NewQuerier()

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

// InitGenesis performs genesis initialization for the deployment module. It returns
// no validator updates.
func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data json.RawMessage) {
	var genesisState types.GenesisState
	cdc.MustUnmarshalJSON(data, &genesisState)

	InitGenesis(ctx, am.keeper, &genesisState)
}

// ExportGenesis returns the exported genesis state as raw bytes for the deployment
// module.
func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) json.RawMessage {
	gs := ExportGenesis(ctx, am.keeper)
	return cdc.MustMarshalJSON(gs)
}

// ConsensusVersion implements module.AppModule#ConsensusVersion
func (am AppModule) ConsensusVersion() uint64 {
	return 5
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
	return simulation.WeightedOperations(simState.AppParams, simState.Cdc, am.acckeeper, am.coinKeeper, am.keeper)
}
