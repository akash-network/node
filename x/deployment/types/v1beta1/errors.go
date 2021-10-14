package v1beta1

import (
	"errors"
)

var (

	// ErrInvalidGroups is the error when groups are empty
	ErrInvalidGroups = errors.New("Invalid groups")
	// ErrInvalidDeploymentID is the error for invalid deployment id

	// ErrEmptyVersion is the error when version is empty
	ErrEmptyVersion = errors.New("Invalid: empty version")
	// ErrInvalidVersion is the error when version is invalid
	ErrInvalidVersion = errors.New("Invalid: deployment version")
	// ErrInternal is the error for internal error

	// ErrInvalidDeployment = is the error when deployment does not pass validation
	ErrInvalidDeployment = errors.New("Invalid deployment")

	// ErrGroupClosed is the error when deployment is closed
	ErrGroupClosed = errors.New("Group already closed")
	// ErrGroupOpen is the error when deployment is closed
	ErrGroupOpen = errors.New("Group open")
	// ErrGroupPaused is the error when deployment is closed
	ErrGroupPaused = errors.New("Group paused")

	// ErrInvalidDeposit indicates an invalid deposit
	ErrInvalidDeposit = errors.New("Deposit invalid")
	// ErrInvalidIDPath indicates an invalid ID path
	ErrInvalidIDPath = errors.New("ID path invalid")
	// ErrInvalidParam indicates an invalid chain parameter
	ErrInvalidParam = errors.New("parameter invalid")
)
