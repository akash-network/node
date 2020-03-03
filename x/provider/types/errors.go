package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	ErrInvalidProviderURI = sdkerrors.Register(ModuleName, 1, "invalid provider: invalid host uri")
	ErrNotAbsProviderURI  = sdkerrors.Register(ModuleName, 2, "invalid provider: not absoulte host uri")
)
