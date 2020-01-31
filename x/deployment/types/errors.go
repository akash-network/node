package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// x/deployment module errors
var (
	ErrNameDoesNotExist    = sdkerrors.Register(ModuleName, 1, "Name does not exist")
	ErrInvalidRequest      = sdkerrors.Register(ModuleName, 2, "Invalid request")
	ErrDeploymentExists    = sdkerrors.Register(ModuleName, 3, "Deployment exists")
	ErrDeploymentNotFound  = sdkerrors.Register(ModuleName, 4, "Deployment not found")
	ErrDeploymentClosed    = sdkerrors.Register(ModuleName, 5, "Deployment closed")
	ErrOwnerAcctMissing    = sdkerrors.Register(ModuleName, 6, "Owner account missing")
	ErrEmptyGroups         = sdkerrors.Register(ModuleName, 7, "Invalid: empty groups")
	ErrInvalidDeploymentID = sdkerrors.Register(ModuleName, 8, "Invalid: deployment id")
	ErrEmptyVersion        = sdkerrors.Register(ModuleName, 9, "Invalid: empty version")
)
