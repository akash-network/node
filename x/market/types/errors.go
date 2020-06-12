package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const (
	errCodeInvalidOrder uint32 = iota + 1
	errCodeEmptyProvider
	errCodeSameAccount
	errCodeInternal
	errCodeOverOrder
	errCodeAttributeMismatch
	errCodeUnknownBid
	errCodeUnknownLeaseForOrder
	errCodeUnknownOrderForBid
	errCodeLeaseNotActive
	errCodeBidNotMatched
	errCodeUnknownOrder
	errCodeNoLeaseForOrder
	errCodeOrderNotFound
	errCodeBidNotFound
	errCodeLeaseNotFound
	errCodeBidExists
	errCodeInvalidPrice
)

var (
	// ErrInvalidOrder is the error when order id is invalid
	ErrInvalidOrder = sdkerrors.Register(ModuleName, errCodeInvalidOrder, "invalid: order id")
	// ErrEmptyProvider is the error when provider is empty
	ErrEmptyProvider = sdkerrors.Register(ModuleName, errCodeEmptyProvider, "empty provider")
	// ErrSameAccount is the error when owner and provider are the same account
	ErrSameAccount = sdkerrors.Register(ModuleName, errCodeSameAccount, "owner and provider are the same account")
	// ErrInternal is the error for internal error
	ErrInternal = sdkerrors.Register(ModuleName, errCodeInternal, "internal error")
	// ErrBidOverOrder is the error when bid price is above max order price
	ErrBidOverOrder = sdkerrors.Register(ModuleName, errCodeOverOrder, "bid price above max order price")
	// ErrAttributeMismatch is the error for attribute mismatch
	ErrAttributeMismatch = sdkerrors.Register(ModuleName, errCodeAttributeMismatch, "attribute mismatch")
	// ErrUnknownBid is the error for unknown bid
	ErrUnknownBid = sdkerrors.Register(ModuleName, errCodeUnknownBid, "unknown bid")
	// ErrUnknownLeaseForBid is the error when lease is unknown for bid
	ErrUnknownLeaseForBid = sdkerrors.Register(ModuleName, errCodeUnknownLeaseForOrder, "unknown lease for bid")
	// ErrUnknownOrderForBid is the error when order is unknown for bid
	ErrUnknownOrderForBid = sdkerrors.Register(ModuleName, errCodeUnknownOrderForBid, "unknown order for bid")
	// ErrLeaseNotActive is the error when lease is not active
	ErrLeaseNotActive = sdkerrors.Register(ModuleName, errCodeLeaseNotActive, "lease not active")
	// ErrBidNotMatched is the error when bid is not matched
	ErrBidNotMatched = sdkerrors.Register(ModuleName, errCodeBidNotMatched, "bid not matched")
	// ErrUnknownOrder is the error when order is unknown
	ErrUnknownOrder = sdkerrors.Register(ModuleName, errCodeUnknownOrder, "unknown order")
	// ErrNoLeaseForOrder is the error when there is no lease for order
	ErrNoLeaseForOrder = sdkerrors.Register(ModuleName, errCodeNoLeaseForOrder, "no lease for order")
	// ErrOrderNotFound order not found
	ErrOrderNotFound = sdkerrors.Register(ModuleName, errCodeOrderNotFound, "invalid order: order not found")
	// ErrBidNotFound bid not found
	ErrBidNotFound = sdkerrors.Register(ModuleName, errCodeBidNotFound, "invalid bid: bid not found")
	// ErrLeaseNotFound lease not found
	ErrLeaseNotFound = sdkerrors.Register(ModuleName, errCodeLeaseNotFound, "invalid lease: lease not found")
	// ErrBidExists bid exists
	ErrBidExists = sdkerrors.Register(ModuleName, errCodeBidExists, "invalid bid: bid exists from provider")
	// ErrBidInvalidPrice bid invalid price
	ErrBidInvalidPrice = sdkerrors.Register(ModuleName, errCodeInvalidPrice, "bid price is invalid")
)
