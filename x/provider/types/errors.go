package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	ErrInvalidProviderURI = sdkerrors.Register(ModuleName, 1, "invalid provider: empty host uri")
)
