package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// x/icaauth module errors
var (
	// ErrProtoMarshal defines an proto marshaing error
	ErrProtoMarshal = sdkerrors.Register(ModuleName, 1, "failed to proto marshal")
)
