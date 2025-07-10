package keeper

import (
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	types "pkg.akt.dev/go/node/staking/v1beta3"
)

type IKeeper interface {
	Codec() codec.BinaryCodec
	StoreKey() storetypes.StoreKey
	SetParams(ctx sdk.Context, params types.Params) error
	GetParams(ctx sdk.Context) (params types.Params)
	MinCommissionRate(ctx sdk.Context) sdkmath.LegacyDec

	NewQuerier() Querier
	GetAuthority() string
}

// Keeper of the provider store
type Keeper struct {
	skey storetypes.StoreKey
	cdc  codec.BinaryCodec

	// The address capable of executing a MsgUpdateParams message.
	// This should be the x/gov module account.
	authority string
}

// NewKeeper creates and returns an instance for akash staking keeper
func NewKeeper(cdc codec.BinaryCodec, skey storetypes.StoreKey, authority string) IKeeper {
	return Keeper{
		skey:      skey,
		cdc:       cdc,
		authority: authority,
	}
}

func (k Keeper) NewQuerier() Querier {
	return Querier{k}
}

// Codec returns keeper codec
func (k Keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

func (k Keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

// GetAuthority returns the x/mint module's authority.
func (k Keeper) GetAuthority() string {
	return k.authority
}

// SetParams sets the x/take module parameters.
func (k Keeper) SetParams(ctx sdk.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}

	store := ctx.KVStore(k.skey)
	bz := k.cdc.MustMarshal(&p)
	store.Set(types.ParamsPrefix(), bz)

	return nil
}

// GetParams returns the current x/take module parameters.
func (k Keeper) GetParams(ctx sdk.Context) (p types.Params) {
	store := ctx.KVStore(k.skey)
	bz := store.Get(types.ParamsPrefix())
	if bz == nil {
		return p
	}

	k.cdc.MustUnmarshal(bz, &p)
	return p
}

func (k Keeper) MinCommissionRate(ctx sdk.Context) sdkmath.LegacyDec {
	params := k.GetParams(ctx)
	return params.MinCommissionRate
}
