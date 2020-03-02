package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	// ErrNameDoesNotExist is the error when name does not exist
	ErrNameDoesNotExist = sdkerrors.Register(ModuleName, 1, "Name does not exist")
	// ErrInvalidRequest is the error for invalid request
	ErrInvalidRequest = sdkerrors.Register(ModuleName, 2, "Invalid request")
	// ErrDeploymentExists is the error when already deployment exists
	ErrDeploymentExists = sdkerrors.Register(ModuleName, 3, "Deployment exists")
	// ErrDeploymentNotFound is the error when deployment not found
	ErrDeploymentNotFound = sdkerrors.Register(ModuleName, 4, "Deployment not found")
	// ErrDeploymentClosed is the error when deployment is closed
	ErrDeploymentClosed = sdkerrors.Register(ModuleName, 5, "Deployment closed")
	// ErrOwnerAcctMissing is the error for owner account missing
	ErrOwnerAcctMissing = sdkerrors.Register(ModuleName, 6, "Owner account missing")
	// ErrEmptyGroups is the error when groups are empty
	ErrEmptyGroups = sdkerrors.Register(ModuleName, 7, "Invalid: empty groups")
	// ErrInvalidDeploymentID is the error for invalid deployment id
	ErrInvalidDeploymentID = sdkerrors.Register(ModuleName, 8, "Invalid: deployment id")
	// ErrEmptyVersion is the error when version is empty
	ErrEmptyVersion = sdkerrors.Register(ModuleName, 9, "Invalid: empty version")
)
