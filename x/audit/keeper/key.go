package keeper

import (
	"bytes"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/x/audit/types"
)

var (
	prefixProviderID = []byte{0x01}
)

func providerKey(id types.ProviderID) []byte {
	buf := bytes.NewBuffer(prefixProviderID)
	if _, err := buf.Write(id.Owner.Bytes()); err != nil {
		panic(err)
	}

	if _, err := buf.Write(id.Auditor.Bytes()); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

func providerPrefix(id sdk.Address) []byte {
	buf := bytes.NewBuffer(prefixProviderID)
	if _, err := buf.Write(id.Bytes()); err != nil {
		panic(err)
	}

	return buf.Bytes()
}
