package accounts

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/errors"
)

var (
	errMissingData   = fmt.Errorf("All tx fields must be filled")
	errNoAccount     = fmt.Errorf("No such account")
	errAccountExists = fmt.Errorf("Account already exists")

	malformed      = errors.CodeTypeEncodingErr
	unknownAddress = errors.CodeTypeBaseUnknownAddress
	unauthorized   = errors.CodeTypeUnauthorized
)

//nolint
func ErrMissingData() errors.TMError {
	return errors.WithCode(errMissingData, malformed)
}
func IsMissingDataErr(err error) bool {
	return errors.IsSameError(errMissingData, err)
}

func ErrNoAccount() errors.TMError {
	return errors.WithCode(errNoAccount, unknownAddress)
}

func ErrAccountExists() errors.TMError {
	return errors.WithCode(errAccountExists, unauthorized)
}
