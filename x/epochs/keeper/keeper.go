package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cosmos/cosmos-sdk/codec"

	types "pkg.akt.dev/go/node/epochs/v1beta1"
)

type Keeper interface {
	Schema() collections.Schema

	SetHooks(eh types.EpochHooks)

	BeginBlocker(ctx sdk.Context) error
	GetEpoch(ctx sdk.Context, identifier string) (types.EpochInfo, error)
	AddEpoch(ctx sdk.Context, epoch types.EpochInfo) error
	RemoveEpoch(ctx sdk.Context, identifier string) error
	IterateEpochs(ctx sdk.Context, fn func(string, types.EpochInfo) (bool, error)) error
	NumBlocksSinceEpochStart(ctx sdk.Context, identifier string) (int64, error)

	InitGenesis(ctx sdk.Context, genState types.GenesisState) error
	ExportGenesis(ctx sdk.Context) (*types.GenesisState, error)

	Hooks() types.EpochHooks
	AfterEpochEnd(ctx context.Context, identifier string, epochNumber int64) error
	BeforeEpochStart(ctx context.Context, identifier string, epochNumber int64) error
}

type keeper struct {
	storeService store.KVStoreService

	cdc   codec.BinaryCodec
	hooks types.EpochHooks

	schema    collections.Schema
	EpochInfo collections.Map[string, types.EpochInfo]
}

// NewKeeper returns a new keeper by codec and storeKey inputs.
func NewKeeper(storeService store.KVStoreService, cdc codec.BinaryCodec) Keeper {
	sb := collections.NewSchemaBuilder(storeService)
	k := &keeper{
		storeService: storeService,
		cdc:          cdc,
		EpochInfo:    collections.NewMap(sb, types.KeyPrefixEpoch, "epoch_info", collections.StringKey, codec.CollValue[types.EpochInfo](cdc)),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.schema = schema

	return k
}

func (k *keeper) Schema() collections.Schema {
	return k.schema
}

// SetHooks sets the hooks on the x/epochs keeper.
func (k *keeper) SetHooks(eh types.EpochHooks) {
	if k.hooks != nil {
		panic("cannot set epochs hooks twice")
	}

	k.hooks = eh
}
