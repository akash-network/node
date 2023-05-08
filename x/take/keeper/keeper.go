package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"

	types "github.com/akash-network/akash-api/go/node/take/v1beta3"
)

type IKeeper interface {
	StoreKey() sdk.StoreKey
	Codec() codec.BinaryCodec
	GetParams(ctx sdk.Context) (params types.Params)
	SetParams(ctx sdk.Context, params types.Params)
	SubtractFees(ctx sdk.Context, amt sdk.Coin) (sdk.Coin, sdk.Coin, error)
}

// Keeper of the deployment store
type Keeper struct {
	skey   sdk.StoreKey
	cdc    codec.BinaryCodec
	pspace paramtypes.Subspace
}

// NewKeeper creates and returns an instance for deployment keeper
func NewKeeper(cdc codec.BinaryCodec, skey sdk.StoreKey, pspace paramtypes.Subspace) IKeeper {

	if !pspace.HasKeyTable() {
		pspace = pspace.WithKeyTable(types.ParamKeyTable())
	}

	return Keeper{
		skey:   skey,
		cdc:    cdc,
		pspace: pspace,
	}
}

// Codec returns keeper codec
func (k Keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

func (k Keeper) StoreKey() sdk.StoreKey {
	return k.skey
}

// GetParams returns the total set of deployment parameters.
func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	var params types.Params
	k.pspace.GetParamSet(ctx, &params)
	return params
}

// SetParams sets the deployment parameters to the paramspace.
func (k Keeper) SetParams(ctx sdk.Context, params types.Params) {
	k.pspace.SetParamSet(ctx, &params)
}

func (k Keeper) SubtractFees(ctx sdk.Context, amt sdk.Coin) (sdk.Coin, sdk.Coin, error) {
	topline := sdk.NewDecCoinFromCoin(amt)

	rate := k.findRate(ctx, topline.GetDenom())

	fees := topline.Amount.Mul(rate).TruncateInt()

	earnings := amt.SubAmount(fees)

	return earnings, sdk.NewCoin(amt.GetDenom(), fees), nil
}

func (k Keeper) findRate(ctx sdk.Context, denom string) sdk.Dec {
	params := k.GetParams(ctx)

	rate, ok := params.DenomTakeRates[denom]
	if !ok {
		rate = params.DefaultTakeRate
	}

	// return percentage.
	return sdk.NewDecFromInt(sdk.NewIntFromUint64(uint64(rate))).Quo(sdk.NewDec(100))
}
