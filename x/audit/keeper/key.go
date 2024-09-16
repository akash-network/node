package keeper

import (
	"bytes"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"

	types "pkg.akt.dev/go/node/audit/v1"
)

func ProviderKey(id types.ProviderID) []byte {
	buf := bytes.NewBuffer(types.PrefixProviderID())
	if _, err := buf.Write(address.MustLengthPrefix(id.Owner.Bytes())); err != nil {
		panic(err)
	}

	if _, err := buf.Write(address.MustLengthPrefix(id.Auditor.Bytes())); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

func ProviderPrefix(id sdk.Address) []byte {
	buf := bytes.NewBuffer(types.PrefixProviderID())
	if _, err := buf.Write(address.MustLengthPrefix(id.Bytes())); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

func ParseIDFromKey(key []byte) types.ProviderID {
	// skip prefix if set
	key = key[len(types.PrefixProviderID()):]

	addrLen := key[0]

	owner := make([]byte, addrLen)
	offset := copy(owner, key[1:addrLen+1])
	key = key[offset+1:]
	addrLen = key[0]
	auditor := make([]byte, addrLen)
	copy(auditor, key[1:addrLen+1])

	key = key[addrLen+1:]

	if len(key) != 0 {
		panic("provider key must not have bytes left after key parse")
	}

	return types.ProviderID{
		Owner:   sdk.AccAddress(owner),
		Auditor: sdk.AccAddress(auditor),
	}
}

// func parseAuditorFromKey(key []byte) sdk.AccAddress {
// 	addrLen := key[0]
//
// 	auditor := make([]byte, addrLen)
// 	copy(auditor, key[1:addrLen+1])
//
// 	key = key[addrLen+1:]
//
// 	if len(key) != 0 {
// 		panic("auditor key must not have bytes left after key parse")
// 	}
//
// 	return auditor
// }
