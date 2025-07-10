package keeper

import (
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/take/v1"
)

type IKeeper interface {
	StoreKey() storetypes.StoreKey
	Codec() codec.BinaryCodec
	GetParams(ctx sdk.Context) (params types.Params)
	SetParams(ctx sdk.Context, params types.Params) error
	SubtractFees(ctx sdk.Context, amt sdk.Coin) (sdk.Coin, sdk.Coin, error)

	NewQuerier() Querier
	GetAuthority() string
}

// Keeper of the deployment store
type Keeper struct {
	skey storetypes.StoreKey
	cdc  codec.BinaryCodec
	// The address capable of executing a MsgUpdateParams message.
	// This should be the x/gov module account.
	authority string
}

// NewKeeper creates and returns an instance of take keeper
func NewKeeper(cdc codec.BinaryCodec, skey storetypes.StoreKey, authority string) IKeeper {
	return Keeper{
		skey:      skey,
		cdc:       cdc,
		authority: authority,
	}
}

// Codec returns keeper codec
func (k Keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

func (k Keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

func (k Keeper) NewQuerier() Querier {
	return Querier{k}
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

func (k Keeper) SubtractFees(ctx sdk.Context, amt sdk.Coin) (sdk.Coin, sdk.Coin, error) {
	topline := sdk.NewDecCoinFromCoin(amt)

	rate := k.findRate(ctx, topline.GetDenom())

	fees := topline.Amount.Mul(rate).TruncateInt()

	earnings := amt.SubAmount(fees)

	return earnings, sdk.NewCoin(amt.GetDenom(), fees), nil
}

func (k Keeper) findRate(ctx sdk.Context, denom string) sdkmath.LegacyDec {
	params := k.GetParams(ctx)

	rate := params.DefaultTakeRate

	for _, denomRate := range params.DenomTakeRates {
		if denom == denomRate.Denom {
			rate = denomRate.Rate
			break
		}
	}

	// return percentage.
	return sdkmath.LegacyNewDecFromInt(sdkmath.NewIntFromUint64(uint64(rate))).Quo(sdkmath.LegacyNewDec(100))
}
