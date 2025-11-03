package keeper

import (
	"context"
	"math/big"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"pkg.akt.dev/go/sdkutil"

	types "pkg.akt.dev/go/node/oracle/v1"
)

type SetParamsHook func(sdk.Context, types.Params)

type Keeper interface {
	StoreKey() storetypes.StoreKey
	Codec() codec.BinaryCodec
	GetAuthority() string
	NewQuerier() Querier
	BeginBlocker(ctx context.Context) error
	GetParams(sdk.Context) (types.Params, error)
	SetParams(sdk.Context, types.Params) error

	SetPriceEntry(sdk.Context, sdk.Address, types.PriceEntry) error
	GetTWAP(ctx sdk.Context, denom string, window int64) (sdkmath.LegacyDec, error)
	WithPriceEntries(sdk.Context, func(types.PriceEntry) bool)
	WithLatestHeights(sdk.Context, func(height types.PriceEntryID) bool)
}

// Keeper of the deployment store
type keeper struct {
	cdc  codec.BinaryCodec
	skey *storetypes.KVStoreKey
	ssvc store.KVStoreService
	// The address capable of executing an MsgUpdateParams message.
	// This should be the x/gov module account.
	authority             string
	priceWriteAuthorities []string

	Schema collections.Schema
	Params collections.Item[types.Params]

	hooks struct {
		onSetParams []SetParamsHook
	}
}

// NewKeeper creates and returns an instance of take keeper
func NewKeeper(cdc codec.BinaryCodec, skey *storetypes.KVStoreKey, authority string) Keeper {
	ssvc := runtime.NewKVStoreService(skey)
	sb := collections.NewSchemaBuilder(ssvc)

	k := &keeper{
		cdc:       cdc,
		skey:      skey,
		ssvc:      runtime.NewKVStoreService(skey),
		authority: authority,
		Params:    collections.NewItem(sb, ParamsKey, "params", codec.CollValue[types.Params](cdc)),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema

	return k
}

// Codec returns keeper codec
func (k *keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

func (k *keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

func (k *keeper) Logger(sctx sdk.Context) log.Logger {
	return sctx.Logger().With("module", "x/"+types.ModuleName)
}

func (k *keeper) NewQuerier() Querier {
	return Querier{k}
}

// GetAuthority returns the x/mint module's authority.
func (k *keeper) GetAuthority() string {
	return k.authority
}

// BeginBlocker checks if prices are being updated and sources do not deviate from each other
// price for requested denom halts if any of the following conditions occur
// - the price have not been updated within UpdatePeriod
// - price deviation between multiple sources is more than TBD
func (k *keeper) BeginBlocker(_ context.Context) error {
	return nil
}

func (k *keeper) GetTWAP(ctx sdk.Context, denom string, window int64) (sdkmath.LegacyDec, error) {
	if denom == sdkutil.DenomAct {
		return sdkmath.LegacyOneDec(), nil
	}

	return sdkmath.LegacyZeroDec(), nil
}

func (k *keeper) SetPriceEntry(ctx sdk.Context, authority sdk.Address, entry types.PriceEntry) error {
	authorized := false
	for _, addr := range k.priceWriteAuthorities {
		if authority.String() == addr {
			authorized = true
			break
		}
	}

	ctx.Context()
	if !authorized {
		return types.ErrUnauthorizedWriterAddress
	}

	key, err := BuildPricePrefix(entry.ID.AssetDenom, entry.ID.BaseDenom, ctx.BlockHeight())
	if err != nil {
		return err
	}

	lkey, err := BuildPriceLatestHeightPrefix(entry.ID.AssetDenom, entry.ID.BaseDenom)
	if err != nil {
		return err
	}

	store := ctx.KVStore(k.skey)
	if store.Has(key) {
		return types.ErrPriceEntryExists
	}

	data := k.cdc.MustMarshal(&entry.State)
	store.Set(key, data)

	val := sdkmath.NewInt(ctx.BlockHeight())
	store.Set(lkey, val.BigInt().Bytes())

	return nil
}

// SetParams sets the x/oracle module parameters.
func (k *keeper) SetParams(ctx sdk.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}

	if err := k.Params.Set(ctx, p); err != nil {
		return err
	}

	// call hooks
	for _, hook := range k.hooks.onSetParams {
		hook(ctx, p)
	}

	return nil
}

// GetParams returns the current x/oracle module parameters.
func (k *keeper) GetParams(ctx sdk.Context) (types.Params, error) {
	return k.Params.Get(ctx)
}

func (k *keeper) AddOnSetParamsHook(hook SetParamsHook) Keeper {
	k.hooks.onSetParams = append(k.hooks.onSetParams, hook)

	return k
}

func (k *keeper) WithPriceEntries(ctx sdk.Context, fn func(types.PriceEntry) bool) {
	store := runtime.KVStoreAdapter(k.ssvc.OpenKVStore(ctx))
	iter := storetypes.KVStorePrefixIterator(store, PricesPrefix)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		id := MustParsePriceEntryID(append(PricesPrefix, iter.Key()...))

		val := types.PriceEntry{
			ID: id,
		}

		k.cdc.MustUnmarshal(iter.Value(), &val.State)
		if stop := fn(val); stop {
			break
		}
	}
}

func (k *keeper) WithLatestHeights(ctx sdk.Context, fn func(height types.PriceEntryID) bool) {
	store := runtime.KVStoreAdapter(k.ssvc.OpenKVStore(ctx))
	iter := storetypes.KVStorePrefixIterator(store, LatestPricesPrefix)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		height := big.NewInt(0)
		height = height.SetBytes(iter.Value())

		id := MustParseLatestPriceHeight(append(LatestPricesPrefix, iter.Key()...), height.Int64())
		if stop := fn(id); stop {
			break
		}
	}
}
