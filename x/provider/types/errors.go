package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const (
	errInvalidProviderURI uint32 = iota + 1
	errNotAbsProviderURI
	errProviderNotFound
	errProviderExists
	errInvalidAddress
	errAttributes
	errIncompatibleAttributes
	errInvalidInfoWebsite
)

var (
	// ErrInvalidProviderURI register error code for invalid provider uri
	ErrInvalidProviderURI = sdkerrors.Register(ModuleName, errInvalidProviderURI, "invalid provider: invalid host uri")

	// ErrNotAbsProviderURI register error code for not absolute provider uri
	ErrNotAbsProviderURI = sdkerrors.Register(ModuleName, errNotAbsProviderURI, "invalid provider: not absolute host uri")

	// ErrProviderNotFound provider not found
	ErrProviderNotFound = sdkerrors.Register(ModuleName, errProviderNotFound, "invalid provider: address not found")

	// ErrProviderExists provider already exists
	ErrProviderExists = sdkerrors.Register(ModuleName, errProviderExists, "invalid provider: already exists")

	// ErrInvalidAddress invalid provider address
	ErrInvalidAddress = sdkerrors.Register(ModuleName, errInvalidAddress, "invalid address")

	// ErrAttributes error code for provider attribute problems
	ErrAttributes = sdkerrors.Register(ModuleName, errAttributes, "attribute specification error")

	// ErrIncompatibleAttributes error code for attributes update
	ErrIncompatibleAttributes = sdkerrors.Register(ModuleName, errIncompatibleAttributes, "attributes cannot be changed")

	// ErrInvalidInfoWebsite register error code for invalid info website
	ErrInvalidInfoWebsite = sdkerrors.Register(ModuleName, errInvalidInfoWebsite, "invalid provider: invalid info website")
)
