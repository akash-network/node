package keeper

import (
	"bytes"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	"github.com/cosmos/cosmos-sdk/types/kv"

	types "github.com/akash-network/akash-api/go/node/cert/v1beta3"
)

// CertificateKey creates a store key of the format:
// prefix_bytes | owner_address_len (1 byte) | owner_address_bytes | serial_bytes
func CertificateKey(id types.CertID) []byte {
	buf := bytes.NewBuffer(types.PrefixCertificateID())
	if _, err := buf.Write(address.MustLengthPrefix(id.Owner.Bytes())); err != nil {
		panic(err)
	}

	if _, err := buf.Write(id.Serial.Bytes()); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

func CertificatePrefix(id sdk.Address) []byte {
	buf := bytes.NewBuffer(types.PrefixCertificateID())
	if _, err := buf.Write(address.MustLengthPrefix(id.Bytes())); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

func ParseCertID(prefix []byte, from []byte) (types.CertID, error) {
	res := types.CertID{
		Serial: *big.NewInt(0),
	}

	kv.AssertKeyAtLeastLength(from, len(prefix))

	// skip prefix if set
	from = from[len(prefix):]

	kv.AssertKeyAtLeastLength(from, 1)

	addrLen := from[0]
	from = from[1:]

	kv.AssertKeyAtLeastLength(from, int(addrLen))

	addr := from[:addrLen]
	err := sdk.VerifyAddressFormat(addr)
	if err != nil {
		return res, err
	}

	// todo add length prefix
	from = from[addrLen:]

	serial := from

	res.Owner = sdk.AccAddress(addr)
	res.Serial.SetBytes(serial)

	return res, nil
}
