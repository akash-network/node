package errors

import "fmt"

type ArgumentError struct {
	arg string
	msg string
}

func (e *ArgumentError) Error() string {
	return fmt.Sprintf(e.msg, e.arg)
}

func (e *ArgumentError) WithMessage(msg string) *ArgumentError {
	e.msg = msg
	return e
}

func NewArgumentError(arg string) *ArgumentError {
	e := &ArgumentError{arg: arg, msg: "invalid or missing argument: %s"}
	return e
}
