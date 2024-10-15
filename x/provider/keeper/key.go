package keeper

import (
	"bytes"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	types "pkg.akt.dev/go/node/provider/v1beta4"
)

func ProviderKey(id sdk.Address) []byte {
	buf := bytes.NewBuffer(types.ProviderPrefix())
	buf.Write(address.MustLengthPrefix(id.Bytes()))

	return buf.Bytes()
}
