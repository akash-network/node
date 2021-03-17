package keeper

import (
	"bytes"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/x/cert/types"
)

const (
	keyAddrPrefixLen = 1 + sdk.AddrLen
)

var (
	prefixCertificateID = []byte{0x01}
)

func certificateKey(id types.CertID) []byte {
	buf := bytes.NewBuffer(prefixCertificateID)

	if _, err := buf.Write(id.Owner.Bytes()); err != nil {
		panic(err)
	}

	if _, err := buf.Write(id.Serial.Bytes()); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

func certificatePrefix(id sdk.Address) []byte {
	buf := bytes.NewBuffer(prefixCertificateID)
	if _, err := buf.Write(id.Bytes()); err != nil {
		panic(err)
	}

	return buf.Bytes()
}

func certificateSerialFromKey(key []byte) big.Int {
	if len(key) < keyAddrPrefixLen+1 {
		panic("invalid key size")
	}

	return *new(big.Int).SetBytes(key[keyAddrPrefixLen:])
}

func certificateOwnerFromKey(key []byte) sdk.Address {
	if len(key) < sdk.AddrLen {
		panic("invalid key size")
	}

	return sdk.AccAddress(key[0:sdk.AddrLen])
}
