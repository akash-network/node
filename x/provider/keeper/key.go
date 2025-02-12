package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
)

func ProviderKey(id sdk.Address) []byte {
	return address.MustLengthPrefix(id.Bytes())
}
