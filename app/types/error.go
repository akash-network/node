package types

import "github.com/ovrclk/akash/types/code"

type Error interface {
	Error() string
	Code() uint32
}

type error_ struct {
	code    uint32
	message string
}

func WrapError(code uint32, err error) Error {
	return NewError(code, err.Error())
}

func NewError(code uint32, message string) Error {
	return error_{code, message}
}

func (e error_) Error() string {
	return e.message
}

func (e error_) Code() uint32 {
	return e.code
}

func ErrUnknownTransaction() Error {
	return error_{code.UNKNOWN_TRANSACTION, "unknown transaction"}
}
