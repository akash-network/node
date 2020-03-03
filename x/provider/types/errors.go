package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	// ErrInvalidProviderURI register error code for invalid provider uri
	ErrInvalidProviderURI = sdkerrors.Register(ModuleName, 1, "invalid provider: invalid host uri")
	// ErrNotAbsProviderURI register error code for not absolute provider uri
	ErrNotAbsProviderURI = sdkerrors.Register(ModuleName, 2, "invalid provider: not absoulte host uri")
)
