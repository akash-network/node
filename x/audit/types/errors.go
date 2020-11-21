package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const (
	errProviderNotFound uint32 = iota + 1
	errInvalidAddress
)

var (
	// ErrProviderNotFound provider not found
	ErrProviderNotFound = sdkerrors.Register(ModuleName, errProviderNotFound, "invalid provider: address not found")

	// ErrInvalidAddress invalid trusted auditor address
	ErrInvalidAddress = sdkerrors.Register(ModuleName, errInvalidAddress, "invalid address")
)
