package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"

	types "pkg.akt.dev/go/node/staking/v1beta3"
)

type IKeeper interface {
	Codec() codec.BinaryCodec
	StoreKey() storetypes.StoreKey
	SetParams(ctx sdk.Context, params types.Params) error
	GetParams(ctx sdk.Context) (params types.Params)
	MinCommissionRate(ctx sdk.Context) sdk.Dec
}

// Keeper of the provider store
type Keeper struct {
	skey   storetypes.StoreKey
	cdc    codec.BinaryCodec
	pspace paramtypes.Subspace
}

// NewKeeper creates and returns an instance for Provider keeper
func NewKeeper(cdc codec.BinaryCodec, skey storetypes.StoreKey, pspace paramtypes.Subspace) IKeeper {
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

func (k Keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

func (k Keeper) MinCommissionRate(ctx sdk.Context) sdk.Dec {
	res := sdk.NewDec(0)
	k.pspace.Get(ctx, types.KeyMinCommissionRate, &res)
	return res
}

// SetParams sets the deployment parameters to the paramspace.
func (k Keeper) SetParams(ctx sdk.Context, params types.Params) error {
	k.pspace.SetParamSet(ctx, &params)
	return nil
}

// GetParams returns the total set of deployment parameters.
func (k Keeper) GetParams(ctx sdk.Context) (params types.Params) {
	return types.NewParams(
		k.MinCommissionRate(ctx),
	)
}
