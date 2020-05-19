package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	// ErrInvalidProviderURI register error code for invalid provider uri
	ErrInvalidProviderURI = sdkerrors.Register(ModuleName, 1, "invalid provider: invalid host uri")

	// ErrNotAbsProviderURI register error code for not absolute provider uri
	ErrNotAbsProviderURI = sdkerrors.Register(ModuleName, 2, "invalid provider: not absolute host uri")

	// ErrProviderNotFound provider not found
	ErrProviderNotFound = sdkerrors.Register(ModuleName, 3, "invalid provider: address not found")

	// ErrInvalidAddress invalid provider address
	ErrInvalidAddress = sdkerrors.Register(ModuleName, 4, "invalid address")
)
