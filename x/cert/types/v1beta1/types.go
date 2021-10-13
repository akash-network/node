package v1beta1

import (
	"bytes"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	PemBlkTypeCertificate  = "CERTIFICATE"
	PemBlkTypeECPrivateKey = "EC PRIVATE KEY"
	PemBlkTypeECPublicKey  = "EC PUBLIC KEY"
)

type CertID struct {
	Owner  sdk.Address
	Serial big.Int
}

func ToCertID(id CertificateID) (CertID, error) {
	addr, err := sdk.AccAddressFromBech32(id.Owner)
	if err != nil {
		return CertID{}, err
	}

	serial, valid := new(big.Int).SetString(id.Serial, 10)
	if !valid {
		return CertID{}, ErrInvalidSerialNumber
	}

	return CertID{
		Owner:  addr,
		Serial: *serial,
	}, nil
}

// Certificates is the collection of Certificate
type Certificates []Certificate

type CertificatesResponse []CertificateResponse

// String implements the Stringer interface for a Certificates object.
func (obj Certificates) String() string {
	var buf bytes.Buffer

	const sep = "\n\n"

	for _, p := range obj {
		buf.WriteString(p.String())
		buf.WriteString(sep)
	}

	if len(obj) > 0 {
		buf.Truncate(buf.Len() - len(sep))
	}

	return buf.String()
}

func (obj Certificates) Contains(cert Certificate) bool {
	for _, c := range obj {
		// fixme is bytes.Equal right way to do it?
		if bytes.Equal(c.Cert, cert.Cert) {
			return true
		}
	}

	return false
}
