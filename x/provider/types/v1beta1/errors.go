package v1beta1

import (
	"errors"
)

var (
	// ErrInvalidProviderURI register error code for invalid provider uri
	ErrInvalidProviderURI = errors.New("invalid provider: invalid host uri")

	// ErrNotAbsProviderURI register error code for not absolute provider uri
	ErrNotAbsProviderURI = errors.New("invalid provider: not absolute host uri")

	// ErrInvalidInfoWebsite register error code for invalid info website
	ErrInvalidInfoWebsite = errors.New("invalid provider: invalid info website")
)
