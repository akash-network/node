package v1beta2

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
)

func ParseAndValidateCertificate(owner sdk.Address, crt, pub []byte) (*x509.Certificate, error) {
	blk, rest := pem.Decode(pub)
	if blk == nil || len(rest) > 0 {
		return nil, ErrInvalidPubkeyValue
	}

	if blk.Type != PemBlkTypeECPublicKey {
		return nil, errors.Wrap(ErrInvalidPubkeyValue, "invalid pem block type")
	}

	blk, rest = pem.Decode(crt)
	if blk == nil || len(rest) > 0 {
		return nil, ErrInvalidCertificateValue
	}

	if blk.Type != PemBlkTypeCertificate {
		return nil, errors.Wrap(ErrInvalidCertificateValue, "invalid pem block type")
	}

	cert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return nil, err
	}

	cowner, err := sdk.AccAddressFromBech32(cert.Subject.CommonName)
	if err != nil {
		return nil, errors.Wrap(ErrInvalidCertificateValue, err.Error())
	}

	if !owner.Equals(cowner) {
		return nil, errors.Wrap(ErrInvalidCertificateValue, "CommonName does not match owner")
	}

	return cert, nil
}

func (m *CertificateID) String() string {
	return fmt.Sprintf("%s/%s", m.Owner, m.Serial)
}

func (m *CertificateID) Equals(val CertificateID) bool {
	return (m.Owner == val.Owner) && (m.Serial == val.Serial)
}

func (m Certificate) Validate(owner sdk.Address) error {
	if val, exists := Certificate_State_name[int32(m.State)]; !exists || val == "invalid" {
		return ErrInvalidState
	}

	_, err := ParseAndValidateCertificate(owner, m.Cert, m.Pubkey)
	if err != nil {
		return err
	}

	return nil
}

func (m Certificate) IsState(state Certificate_State) bool {
	return m.State == state
}
