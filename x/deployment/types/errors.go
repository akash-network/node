package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// DefaultCodespace is the Module Name
const (
	DefaultCodespace sdk.CodespaceType = ModuleName

	CodeNameDoesNotExist sdk.CodeType = 101

	CodeInvalidRequest     sdk.CodeType = 102
	CodeDeploymentExists   sdk.CodeType = 103
	CodeDeploymentNotFound sdk.CodeType = 104
	CodeDeploymentClosed   sdk.CodeType = 105
)

// ErrNameDoesNotExist is the error for name not existing
func ErrNameDoesNotExist(codespace sdk.CodespaceType) sdk.Error {
	return sdk.NewError(codespace, CodeNameDoesNotExist, "Name does not exist")
}

func ErrDeploymentExists() sdk.Error {
	return sdk.NewError(DefaultCodespace, CodeDeploymentExists, "Deployment exists")
}

func ErrDeploymentNotFound() sdk.Error {
	return sdk.NewError(DefaultCodespace, CodeDeploymentNotFound, "Deployment not found")
}

func ErrDeploymentClosed() sdk.Error {
	return sdk.NewError(DefaultCodespace, CodeDeploymentClosed, "Deployment closed")
}
