package keeper

import (
	"bytes"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"

	types "github.com/akash-network/akash-api/go/node/cert/v1beta3"
)

const (
	keyAddrPrefixLen = 2 // 1 byte for PrefixCertificateID followed by 1 byte for owner_address_len
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

func CertificateSerialFromKey(key []byte) big.Int {
	if len(key) < keyAddrPrefixLen {
		panic("invalid key size")
	}

	addrLen := int(key[keyAddrPrefixLen-1])
	if len(key) < keyAddrPrefixLen+addrLen+1 {
		panic("invalid key size")
	}

	return *new(big.Int).SetBytes(key[keyAddrPrefixLen+addrLen:])
}

func ParseCertID(prefix []byte, from []byte) (types.CertID, error) {
	res := types.CertID{
		Serial: *big.NewInt(0),
	}

	// skip prefix if set
	if len(prefix) > 0 {
		from = from[len(prefix):]
	}

	addLen := from[0]

	from = from[1:]

	addr := from[:addLen-1]
	serial := from[addLen:]

	err := sdk.VerifyAddressFormat(addr)
	if err != nil {
		return res, err
	}

	res.Owner = sdk.AccAddress(addr)
	res.Serial.SetBytes(serial)

	return res, nil
}
