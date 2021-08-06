package keeper

import (
	"bytes"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"

	"github.com/ovrclk/akash/x/cert/types"
)

const (
	keyAddrPrefixLen = 1 /*len(PrefixCertificateID)*/ + 1 /*owner_address_len (1 byte)*/
)

// certificateKey creates a store key of the format:
// prefix_bytes | owner_address_len (1 byte) | owner_address_bytes | serial_bytes
func certificateKey(id types.CertID) []byte {
	buf := bytes.NewBuffer(types.PrefixCertificateID)
	if _, err := buf.Write(address.MustLengthPrefix(id.Owner.Bytes())); err != nil {
		panic(err)
	}

	if _, err := buf.Write(id.Serial.Bytes()); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

func certificatePrefix(id sdk.Address) []byte {
	buf := bytes.NewBuffer(types.PrefixCertificateID)
	if _, err := buf.Write(address.MustLengthPrefix(id.Bytes())); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

func certificateSerialFromKey(key []byte) big.Int {
	if len(key) < keyAddrPrefixLen {
		panic("invalid key size")
	}

	addrLen := int(key[keyAddrPrefixLen-1])
	if len(key) < keyAddrPrefixLen+addrLen+1 {
		panic("invalid key size")
	}

	return *new(big.Int).SetBytes(key[keyAddrPrefixLen+addrLen:])
}
