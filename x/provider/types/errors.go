package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// ErrInvalidProviderURI - Register error code for invalid provider uri
var (
	ErrInvalidProviderURI = sdkerrors.Register(ModuleName, 1, "invalid provider: empty host uri")
)
