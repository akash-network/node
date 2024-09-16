package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"

	types "pkg.akt.dev/go/node/gov/v1beta3"
)

type IKeeper interface {
	Codec() codec.BinaryCodec
	StoreKey() storetypes.StoreKey
	SetDepositParams(ctx sdk.Context, params types.DepositParams) error
	GetDepositParams(ctx sdk.Context) (params types.DepositParams)
}

// Keeper of the provider store
type Keeper struct {
	skey   storetypes.StoreKey
	cdc    codec.BinaryCodec
	pspace paramtypes.Subspace
}

// NewKeeper creates and returns an instance for Provider keeper
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

// SetDepositParams sets the deployment parameters to the paramspace.
func (k Keeper) SetDepositParams(_ sdk.Context, _ types.DepositParams) error {
	// k.pspace.Set(ctx, types.KeyDepositParams, &params)

	return nil
}

// GetDepositParams returns the total set of x/gov parameters.
func (k Keeper) GetDepositParams(_ sdk.Context) types.DepositParams {
	var params types.DepositParams

	// k.pspace.Get(ctx, types.KeyDepositParams, &params)
	return params
}
