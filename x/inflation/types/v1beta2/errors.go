package v1beta2

import "errors"

var (
	// ErrInvalidParam indicates an invalid chain parameter
	ErrInvalidParam = errors.New("parameter invalid")
	// ErrInvalidInitialInflation indicates an invalid initial_inflation parameter
	ErrInvalidInitialInflation = errors.New("initial inflation parameter is invalid")
	// ErrInvalidVariance indicates an invalid variance parameter
	ErrInvalidVariance = errors.New("variance parameter is invalid")
)
