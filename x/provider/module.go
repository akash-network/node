package provider

import (
	"encoding/json"

	sdk "github.com/cosmos/cosmos-sdk/types"
	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/gorilla/mux"
	"github.com/ovrclk/akash/x/provider/client/cli"
	"github.com/ovrclk/akash/x/provider/handler"
	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/query"
	"github.com/ovrclk/akash/x/provider/types"
	"github.com/spf13/cobra"
)

var (
	_ module.AppModule      = AppModule{}
	_ module.AppModuleBasic = AppModuleBasic{}
)

// AppModuleBasic defines the basic application module used by the provider module.
type AppModuleBasic struct{}

// Name returns provider module's name
func (AppModuleBasic) Name() string {
	return types.ModuleName
}

// RegisterCodec registers the provider module's types for the given codec.
func (AppModuleBasic) RegisterCodec(cdc *codec.Codec) {
	types.RegisterCodec(cdc)
}

// DefaultGenesis returns default genesis state as raw bytes for the provider
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

// GetTxCmd returns the transaction commands for this module
func (AppModuleBasic) GetTxCmd(cdc *codec.Codec) *cobra.Command {
	return cli.GetTxCmd(StoreKey, cdc)
}

// GetQueryClient returns a new query client for this module
func (AppModuleBasic) GetQueryClient(ctx context.CLIContext) query.Client {
	return query.NewClient(ctx, StoreKey)
}

// AppModule implements an application module for the provider module.
type AppModule struct {
	AppModuleBasic
	keeper  keeper.Keeper
	bkeeper bank.Keeper
}

// NewAppModule creates a new AppModule object
func NewAppModule(k keeper.Keeper, bkeeper bank.Keeper) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{},
		keeper:         k,
		bkeeper:        bkeeper,
	}
}

// Name returns the market module name
func (AppModule) Name() string {
	return types.ModuleName
}

// RegisterInvariants registers module invariants
func (am AppModule) RegisterInvariants(ir sdk.InvariantRegistry) {}

// Route returns the message routing key for the provider module.
func (am AppModule) Route() string {
	return types.RouterKey
}

// NewHandler returns an sdk.Handler for the provider module.
func (am AppModule) NewHandler() sdk.Handler {
	return handler.NewHandler(am.keeper)
}

// QuerierRoute returns the provider module's querier route name.
func (am AppModule) QuerierRoute() string {
	return types.ModuleName
}

// NewQuerierHandler returns the sdk.Querier for provider module
func (am AppModule) NewQuerierHandler() sdk.Querier {
	return query.NewQuerier(am.keeper)
}

// BeginBlock performs no-op
func (am AppModule) BeginBlock(_ sdk.Context, _ abci.RequestBeginBlock) {}

// EndBlock returns the end blocker for the provider module. It returns no validator
// updates.
func (am AppModule) EndBlock(ctx sdk.Context, _ abci.RequestEndBlock) []abci.ValidatorUpdate {
	return []abci.ValidatorUpdate{}
}

// InitGenesis performs genesis initialization for the provider module. It returns
// no validator updates.
func (am AppModule) InitGenesis(ctx sdk.Context, data json.RawMessage) []abci.ValidatorUpdate {
	var genesisState GenesisState
	types.MustUnmarshalJSON(data, &genesisState)
	return InitGenesis(ctx, am.keeper, genesisState)
}

// ExportGenesis returns the exported genesis state as raw bytes for the provider
// module.
func (am AppModule) ExportGenesis(ctx sdk.Context) json.RawMessage {
	gs := ExportGenesis(ctx, am.keeper)
	return types.MustMarshalJSON(gs)
}
