package supply

import (
	"encoding/json"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"

	"github.com/ovrclk/akash/x/supply/client/rest"
	"github.com/ovrclk/akash/x/supply/query"
	"github.com/ovrclk/akash/x/supply/types"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	abci "github.com/tendermint/tendermint/abci/types"
)

var (
	_ module.AppModule      = AppModule{}
	_ module.AppModuleBasic = AppModuleBasic{}
)

// AppModuleBasic defines the basic application module used by the supply module.
type AppModuleBasic struct{}

// Name returns supply module's name
func (AppModuleBasic) Name() string {
	return types.ModuleName
}

// RegisterCodec registers the suupply module's types for the given codec.
func (AppModuleBasic) RegisterCodec(_ *codec.Codec) {}

// DefaultGenesis returns default genesis state as raw bytes for the supply module.
func (AppModuleBasic) DefaultGenesis() json.RawMessage {
	return nil
}

// ValidateGenesis validation check of the Genesis
func (AppModuleBasic) ValidateGenesis(_ json.RawMessage) error {
	return nil
}

// RegisterRESTRoutes registers rest routes for this module
func (AppModuleBasic) RegisterRESTRoutes(ctx context.CLIContext, rtr *mux.Router) {
	rest.RegisterRoutes(ctx, rtr)
}

// GetQueryCmd returns the root query command of this module
// This module has a query which is directly added to SDK supply queries
func (AppModuleBasic) GetQueryCmd(cdc *codec.Codec) *cobra.Command {
	return nil
}

// GetTxCmd returns the root tx command of this module
func (AppModuleBasic) GetTxCmd(_ *codec.Codec) *cobra.Command {
	return nil
}

// GetQueryClient returns a new query client for this module
func (AppModuleBasic) GetQueryClient(ctx context.CLIContext) query.Client {
	return query.NewClient(ctx, types.ModuleName)
}

type AppModule struct {
	AppModuleBasic

	cdc           *codec.Codec
	AccountKeeper types.AccountKeeper
	SupplyKeeper  types.SupplyKeeper
}

// NewAppModule creates a new AppModule object
func NewAppModule(cdc *codec.Codec, accKeeper types.AccountKeeper, supKeeper types.SupplyKeeper) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{},
		cdc:            cdc,
		AccountKeeper:  accKeeper,
		SupplyKeeper:   supKeeper,
	}
}

// Name returns the supply module name
func (AppModule) Name() string {
	return types.ModuleName
}

// RegisterInvariants registers module invariant
func (am AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}

// Route returns the message routing key for the supply module.
func (am AppModule) Route() string {
	return ""
}

// NewHandler returns an sdk.Handler for the supply module.
func (am AppModule) NewHandler() sdk.Handler {
	return nil
}

// QuerierRoute returns the supply module's querier route name.
func (am AppModule) QuerierRoute() string {
	return types.ModuleName
}

// NewQuerierHandler returns the sdk.Querier for supply module
func (am AppModule) NewQuerierHandler() sdk.Querier {
	return query.NewQuerier(am.cdc, am.AccountKeeper, am.SupplyKeeper)
}

// BeginBlock performs no-op
func (am AppModule) BeginBlock(ctx sdk.Context, beginBlock abci.RequestBeginBlock) {}

// EndBlock performs no-op
func (am AppModule) EndBlock(ctx sdk.Context, endBlock abci.RequestEndBlock) []abci.ValidatorUpdate {
	return nil
}

// InitGenesis performs genesis initialization for the supply module. It returns
// no validator updates.
func (am AppModule) InitGenesis(_ sdk.Context, _ json.RawMessage) []abci.ValidatorUpdate {
	return nil
}

// ExportGenesis returns the exported genesis state as raw bytes for the supply module.
func (am AppModule) ExportGenesis(_ sdk.Context) json.RawMessage {
	return nil
}
