package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	ErrInvalidOrder       = sdkerrors.Register(ModuleName, 1, "invalid: order id")
	ErrEmptyProvider      = sdkerrors.Register(ModuleName, 2, "empty provider")
	ErrSameAccount        = sdkerrors.Register(ModuleName, 3, "owner and provider are the same account")
	ErrInternal           = sdkerrors.Register(ModuleName, 4, "internal error")
	ErrBidOverOrder       = sdkerrors.Register(ModuleName, 5, "bid price above max order price")
	ErrAtributeMismatch   = sdkerrors.Register(ModuleName, 6, "atribute mismatch")
	ErrUnknownBid         = sdkerrors.Register(ModuleName, 7, "unknown bid")
	ErrUnknownLeaseForBid = sdkerrors.Register(ModuleName, 8, "unknown lease for bid")
	ErrUnknownOrderForBid = sdkerrors.Register(ModuleName, 9, "unknown order for bid")
	ErrLeaseNotActive     = sdkerrors.Register(ModuleName, 10, "lease not active")
	ErrBidNotMatched      = sdkerrors.Register(ModuleName, 11, "bid not matched")
	ErrUnknownOrder       = sdkerrors.Register(ModuleName, 12, "unknown order")
	ErrNoLeaseForOrder    = sdkerrors.Register(ModuleName, 13, "no lease for order")
)
