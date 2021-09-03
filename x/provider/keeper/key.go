package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
)

func providerKey(id sdk.Address) []byte {
	return address.MustLengthPrefix(id.Bytes())
}
