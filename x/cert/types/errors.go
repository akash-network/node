package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const (
	errCertificateNotFound uint32 = iota + 1
	errInvalidAddress
	errCertificateExists
	errCertificateAlreadyRevoked
	errInvalidSerialNumber
	errInvalidCertificateValue
	errInvalidPubkeyValue
	errInvalidState
	errInvalidKeySize
)

var (
	// ErrCertificateNotFound certificate not found
	ErrCertificateNotFound = sdkerrors.Register(ModuleName, errCertificateNotFound, "certificate not found")

	// ErrInvalidAddress invalid trusted auditor address
	ErrInvalidAddress = sdkerrors.Register(ModuleName, errInvalidAddress, "invalid address")

	// ErrCertificateExists certificate already exists
	ErrCertificateExists = sdkerrors.Register(ModuleName, errCertificateExists, "certificate exists")

	// ErrCertificateAlreadyRevoked certificate already revoked
	ErrCertificateAlreadyRevoked = sdkerrors.Register(ModuleName, errCertificateAlreadyRevoked, "certificate already revoked")

	// ErrInvalidSerialNumber invalid serial number
	ErrInvalidSerialNumber = sdkerrors.Register(ModuleName, errInvalidSerialNumber, "invalid serial number")

	// ErrInvalidCertificateValue certificate content is not valid
	ErrInvalidCertificateValue = sdkerrors.Register(ModuleName, errInvalidCertificateValue, "invalid certificate value")

	// ErrInvalidPubkeyValue public key is not valid
	ErrInvalidPubkeyValue = sdkerrors.Register(ModuleName, errInvalidPubkeyValue, "invalid pubkey value")

	// ErrInvalidState invalid certificate state
	ErrInvalidState = sdkerrors.Register(ModuleName, errInvalidState, "invalid state")

	// ErrInvalidState invalid certificate state
	ErrInvalidKeySize = sdkerrors.Register(ModuleName, errInvalidKeySize, "invalid key size")
)
