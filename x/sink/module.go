package sink

import (
	"context"
	"encoding/json"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	gwruntime "github.com/grpc-ecosystem/grpc-gateway/runtime"

	types "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/x/sink/handler"
	"pkg.akt.dev/node/v2/x/sink/keeper"
	"pkg.akt.dev/node/v2/x/sink/simulation"
)

const ModuleName = types.ModuleName

var (
	_ module.AppModuleBasic      = AppModuleBasic{}
	_ module.AppModuleSimulation = AppModule{}
	_ module.HasGenesis          = AppModule{}
	_ module.HasProposalMsgs     = AppModule{}
	_ module.HasServices         = AppModule{}
)

type AppModuleBasic struct{}

func (AppModuleBasic) Name() string { return ModuleName }

func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	types.RegisterLegacyAminoCodec(cdc)
}

func (AppModuleBasic) RegisterInterfaces(reg codectypes.InterfaceRegistry) {
	types.RegisterInterfaces(reg)
}

func (AppModuleBasic) RegisterGRPCGatewayRoutes(cctx client.Context, mux *gwruntime.ServeMux) {
	if err := types.RegisterQueryHandlerClient(context.Background(), mux, types.NewQueryClient(cctx)); err != nil {
		panic(err)
	}
}

func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(types.DefaultGenesis())
}

func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage) error {
	var g types.GenesisState
	if err := cdc.UnmarshalJSON(bz, &g); err != nil {
		return err
	}

	return g.ValidateBasic()
}

type AppModule struct {
	AppModuleBasic
	keeper   keeper.Keeper
	accounts keeper.AccountKeeper
	bank     keeper.BankKeeper
}

func NewAppModule(k keeper.Keeper, ak keeper.AccountKeeper, bk keeper.BankKeeper) AppModule {
	return AppModule{
		keeper:   k,
		accounts: ak,
		bank:     bk,
	}
}

func (AppModule) IsAppModule() {}

func (AppModule) IsOnePerModuleType() {}

func (AppModule) ConsensusVersion() uint64 { return 3 }

func (am AppModule) RegisterServices(cfg module.Configurator) {
	types.RegisterMsgServer(cfg.MsgServer(), handler.NewServer(am.keeper))
	types.RegisterQueryServer(cfg.QueryServer(), am.keeper.NewQuerier())
}

func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, bz json.RawMessage) {
	var g types.GenesisState
	cdc.MustUnmarshalJSON(bz, &g)

	if err := am.keeper.InitGenesis(ctx, &g); err != nil {
		panic(err)
	}
}

func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) json.RawMessage {
	g, err := am.keeper.ExportGenesis(ctx)
	if err != nil {
		panic(err)
	}

	return cdc.MustMarshalJSON(g)
}

func (am AppModule) BeginBlock(ctx context.Context) error {
	return am.keeper.BeginBlocker(ctx)
}

// GenerateGenesisState creates randomized Sink parameters and reporter policy.
func (AppModule) GenerateGenesisState(simState *module.SimulationState) {
	simulation.RandomizedGenState(simState)
}

// ProposalMsgs returns Sink governance messages for simulations.
func (am AppModule) ProposalMsgs(_ module.SimulationState) []simtypes.WeightedProposalMsg {
	return simulation.ProposalMsgs(am.keeper)
}

// RegisterStoreDecoder registers the Sink collections schema for state comparisons.
func (am AppModule) RegisterStoreDecoder(sdr simtypes.StoreDecoderRegistry) {
	sdr[types.StoreKey] = simtypes.NewStoreDecoderFuncFromCollectionsSchema(am.keeper.Schema())
}

// WeightedOperations returns signed Sink deposit, report and refund operations.
func (am AppModule) WeightedOperations(simState module.SimulationState) []simtypes.WeightedOperation {
	return simulation.WeightedOperations(simState.AppParams, simState.TxConfig, am.accounts, am.bank, am.keeper)
}
