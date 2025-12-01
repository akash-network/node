package keeper

import (
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/oracle/v1"
)

type SetParamsHook func(sdk.Context, types.Params)

type Keeper interface {
	StoreKey() storetypes.StoreKey
	Codec() codec.BinaryCodec
	GetParams(ctx sdk.Context) (params types.Params)
	SetParams(ctx sdk.Context, params types.Params) error

	NewQuerier() Querier
	GetAuthority() string
}

// Keeper of the deployment store
type keeper struct {
	skey  storetypes.StoreKey
	cdc   codec.BinaryCodec
	hooks struct {
		onSetParams []SetParamsHook
	}

	// The address capable of executing an MsgUpdateParams message.
	// This should be the x/gov module account.
	authority string
}

// NewKeeper creates and returns an instance of take keeper
func NewKeeper(cdc codec.BinaryCodec, skey storetypes.StoreKey, authority string) Keeper {
	return &keeper{
		skey:      skey,
		cdc:       cdc,
		authority: authority,
	}
}

// Codec returns keeper codec
func (k *keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

func (k *keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

func (k *keeper) NewQuerier() Querier {
	return Querier{k}
}

// GetAuthority returns the x/mint module's authority.
func (k *keeper) GetAuthority() string {
	return k.authority
}

// SetParams sets the x/take module parameters.
func (k *keeper) SetParams(ctx sdk.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}

	store := ctx.KVStore(k.skey)
	bz := k.cdc.MustMarshal(&p)
	store.Set(types.ParamsPrefix(), bz)

	// call hooks
	for _, hook := range k.hooks.onSetParams {
		hook(ctx, p)
	}

	return nil
}

// GetParams returns the current x/take module parameters.
func (k *keeper) GetParams(ctx sdk.Context) (p types.Params) {
	store := ctx.KVStore(k.skey)
	bz := store.Get(types.ParamsPrefix())
	if bz == nil {
		return p
	}

	k.cdc.MustUnmarshal(bz, &p)

	return p
}

func (k *keeper) AddOnSetParamsHook(hook SetParamsHook) Keeper {
	k.hooks.onSetParams = append(k.hooks.onSetParams, hook)

	return k
}
