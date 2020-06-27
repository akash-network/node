package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const (
	errNameDoesNotExist uint32 = iota + 1
	errInvalidRequest
	errDeploymentExists
	errDeploymentNotFound
	errDeploymentClosed
	errOwnerAcctMissing
	errInvalidGroups
	errInvalidDeploymentID
	errEmptyVersion
	errInternal
	errInvalidDeployment
	errInvalidGroupID
	errGroupNotFound
	errGroupClosed
	errGroupNotOpen
)

var (
	// ErrNameDoesNotExist is the error when name does not exist
	ErrNameDoesNotExist = sdkerrors.Register(ModuleName, errNameDoesNotExist, "Name does not exist")
	// ErrInvalidRequest is the error for invalid request
	ErrInvalidRequest = sdkerrors.Register(ModuleName, errInvalidRequest, "Invalid request")
	// ErrDeploymentExists is the error when already deployment exists
	ErrDeploymentExists = sdkerrors.Register(ModuleName, errDeploymentExists, "Deployment exists")
	// ErrDeploymentNotFound is the error when deployment not found
	ErrDeploymentNotFound = sdkerrors.Register(ModuleName, errDeploymentNotFound, "Deployment not found")
	// ErrDeploymentClosed is the error when deployment is closed
	ErrDeploymentClosed = sdkerrors.Register(ModuleName, errDeploymentClosed, "Deployment closed")
	// ErrOwnerAcctMissing is the error for owner account missing
	ErrOwnerAcctMissing = sdkerrors.Register(ModuleName, errOwnerAcctMissing, "Owner account missing")
	// ErrInvalidGroups is the error when groups are empty
	ErrInvalidGroups = sdkerrors.Register(ModuleName, errInvalidGroups, "Invalid groups")
	// ErrInvalidDeploymentID is the error for invalid deployment id
	ErrInvalidDeploymentID = sdkerrors.Register(ModuleName, errInvalidDeploymentID, "Invalid: deployment id")
	// ErrEmptyVersion is the error when version is empty
	ErrEmptyVersion = sdkerrors.Register(ModuleName, errEmptyVersion, "Invalid: empty version")
	// ErrInternal is the error for internal error
	ErrInternal = sdkerrors.Register(ModuleName, errInternal, "internal error")
	// ErrInvalidDeployment = is the error when deployment does not pass validation
	ErrInvalidDeployment = sdkerrors.Register(ModuleName, errInvalidDeployment, "Invalid deployment")
	// ErrInvalidGroupID is the error when already deployment exists
	ErrInvalidGroupID = sdkerrors.Register(ModuleName, errInvalidGroupID, "Deployment exists")
	// ErrGroupNotFound is the keeper's error for not finding a group
	ErrGroupNotFound = sdkerrors.Register(ModuleName, errGroupNotFound, "Group not found")
	// ErrGroupClosed is the error when deployment is closed
	ErrGroupClosed = sdkerrors.Register(ModuleName, errGroupClosed, "Group already closed")
	// ErrGroupNotOpen indicates the Group state has progressed beyond initial Open.
	ErrGroupNotOpen = sdkerrors.Register(ModuleName, errGroupNotOpen, "Group not open")
)
