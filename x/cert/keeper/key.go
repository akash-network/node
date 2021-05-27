package keeper

import (
	"bytes"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/cert/types"
)

const (
	// AddrLen defines a valid address length
	AddrLen          = 20
	keyAddrPrefixLen = 1 + AddrLen
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
