package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	// ErrInvalidOrder is the error when order id is invalid
	ErrInvalidOrder = sdkerrors.Register(ModuleName, 1, "invalid: order id")
	// ErrEmptyProvider is the error when provider is empty
	ErrEmptyProvider = sdkerrors.Register(ModuleName, 2, "empty provider")
	// ErrSameAccount is the error when owner and provider are the same account
	ErrSameAccount = sdkerrors.Register(ModuleName, 3, "owner and provider are the same account")
	// ErrInternal is the error for internal error
	ErrInternal = sdkerrors.Register(ModuleName, 4, "internal error")
	// ErrBidOverOrder is the error when bid price is above max order price
	ErrBidOverOrder = sdkerrors.Register(ModuleName, 5, "bid price above max order price")
	// ErrAtributeMismatch is the error for attribute mismatch
	ErrAtributeMismatch = sdkerrors.Register(ModuleName, 6, "atribute mismatch")
	// ErrUnknownBid is the error for unknown bid
	ErrUnknownBid = sdkerrors.Register(ModuleName, 7, "unknown bid")
	// ErrUnknownLeaseForBid is the error when lease is unknown for bid
	ErrUnknownLeaseForBid = sdkerrors.Register(ModuleName, 8, "unknown lease for bid")
	// ErrUnknownOrderForBid is the error when order is unknown for bid
	ErrUnknownOrderForBid = sdkerrors.Register(ModuleName, 9, "unknown order for bid")
	// ErrLeaseNotActive is the error when lease is not active
	ErrLeaseNotActive = sdkerrors.Register(ModuleName, 10, "lease not active")
	// ErrBidNotMatched is the error when bid is not matched
	ErrBidNotMatched = sdkerrors.Register(ModuleName, 11, "bid not matched")
	// ErrUnknownOrder is the error when order is unknown
	ErrUnknownOrder = sdkerrors.Register(ModuleName, 12, "unknown order")
	// ErrNoLeaseForOrder is the error when there is no lease for order
	ErrNoLeaseForOrder = sdkerrors.Register(ModuleName, 13, "no lease for order")
	// ErrOrderNotFound order not found
	ErrOrderNotFound = sdkerrors.Register(ModuleName, 14, "invalid order: order not found")
	// ErrBidNotFound bid not found
	ErrBidNotFound = sdkerrors.Register(ModuleName, 15, "invalid bid: bid not found")
	// ErrLeaseNotFound lease not found
	ErrLeaseNotFound = sdkerrors.Register(ModuleName, 16, "invalid lease: lease not found")
)
