package keeper

import (
	"bytes"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/x/cert/types"
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
