package v1beta1

import (
	"errors"
)

var (
	ErrInvalidPayment   = errors.New("invalid payment")
	ErrInvalidAccountID = errors.New("invalid account ID")
	ErrInvalidAccount   = errors.New("invalid account")
)
