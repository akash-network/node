package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"

	types "github.com/akash-network/akash-api/go/node/gov/v1beta3"
)

type IKeeper interface {
	Codec() codec.BinaryCodec
	StoreKey() sdk.StoreKey
	SetDepositParams(ctx sdk.Context, params types.DepositParams) error
	GetDepositParams(ctx sdk.Context) (params types.DepositParams)
}

// Keeper of the provider store
type Keeper struct {
	skey   sdk.StoreKey
	cdc    codec.BinaryCodec
	pspace paramtypes.Subspace
}

// NewKeeper creates and returns an instance for Provider keeper
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

// SetDepositParams sets the deployment parameters to the paramspace.
func (k Keeper) SetDepositParams(ctx sdk.Context, params types.DepositParams) error {
	k.pspace.Set(ctx, types.KeyDepositParams, &params)

	return nil
}

// GetDepositParams returns the total set of x/gov parameters.
func (k Keeper) GetDepositParams(ctx sdk.Context) types.DepositParams {
	var params types.DepositParams

	k.pspace.Get(ctx, types.KeyDepositParams, &params)
	return params
}
