package v1beta1

import (
	"errors"
)

var (
	// ErrEmptyProvider is the error when provider is empty
	ErrEmptyProvider = errors.New("empty provider")
	// ErrSameAccount is the error when owner and provider are the same account
	ErrSameAccount = errors.New("owner and provider are the same account")
	// ErrBidZeroPrice zero price
	ErrBidZeroPrice = errors.New("invalid bid: zero price")
	// ErrOrderActive order active
	ErrOrderActive = errors.New("order active")
	// ErrOrderClosed order closed
	ErrOrderClosed = errors.New("order closed")
	// ErrInvalidParam indicates an invalid chain parameter
	ErrInvalidParam = errors.New("parameter invalid")
	// ErrInvalidBid indicates an invalid chain parameter
	ErrInvalidBid = errors.New("unknown provider")
)
