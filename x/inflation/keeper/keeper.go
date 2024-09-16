package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"

	types "pkg.akt.dev/go/node/inflation/v1beta3"
)

type IKeeper interface {
	Codec() codec.BinaryCodec
	StoreKey() storetypes.StoreKey
	GetParams(ctx sdk.Context) (params types.Params)
	SetParams(ctx sdk.Context, params types.Params)
}

// Keeper of the deployment store
type Keeper struct {
	skey   storetypes.StoreKey
	cdc    codec.BinaryCodec
	pspace paramtypes.Subspace
}

// NewKeeper creates and returns an instance for deployment keeper
func NewKeeper(cdc codec.BinaryCodec, skey storetypes.StoreKey, pspace paramtypes.Subspace) IKeeper {
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

// GetParams returns the total set of deployment parameters.
func (k Keeper) GetParams(_ sdk.Context) (params types.Params) {
	return params
}

// SetParams sets the deployment parameters to the paramspace.
func (k Keeper) SetParams(_ sdk.Context, _ types.Params) {
}
