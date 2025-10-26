package keeper

import (
	"bytes"
	"encoding/hex"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"

	types "pkg.akt.dev/go/node/audit/v1"

	"pkg.akt.dev/node/util/validation"
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

	validation.AssertKeyAtLeastLength(key, len(types.PrefixProviderID())+1)
	if !bytes.HasPrefix(key, types.PrefixProviderID()) {
		panic(fmt.Sprintf("invalid key prefix. expected 0x%s, actual 0x%s", hex.EncodeToString(key[:1]), types.PrefixProviderID()))
	}

	// remove a prefix key
	key = key[len(types.PrefixProviderID()):]

	dataLen := int(key[0])
	key = key[1:]
	validation.AssertKeyAtLeastLength(key, dataLen)

	owner := make([]byte, dataLen)
	copy(owner, key[:dataLen])
	key = key[dataLen:]
	validation.AssertKeyAtLeastLength(key, 1)

	dataLen = int(key[0])
	key = key[1:]
	validation.AssertKeyLength(key, dataLen)
	auditor := make([]byte, dataLen)
	copy(auditor, key[:dataLen])

	return types.ProviderID{
		Owner:   sdk.AccAddress(owner),
		Auditor: sdk.AccAddress(auditor),
	}
}
