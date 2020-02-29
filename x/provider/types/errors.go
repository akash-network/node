package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	// ErrInvalidProviderURI register error code for invalid provider uri
	ErrInvalidProviderURI = sdkerrors.Register(ModuleName, 1, "invalid provider: empty host uri")
)
