package v1beta1

import (
	"github.com/pkg/errors"
)

var (
	// ErrInvalidSerialNumber invalid serial number
	ErrInvalidSerialNumber = errors.New("invalid serial number")

	// ErrInvalidCertificateValue certificate content is not valid
	ErrInvalidCertificateValue = errors.New("invalid certificate value")

	// ErrInvalidPubkeyValue public key is not valid
	ErrInvalidPubkeyValue = errors.New("invalid pubkey value")

	// ErrInvalidState invalid certificate state
	ErrInvalidState = errors.New("invalid state")
)
