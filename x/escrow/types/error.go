package types

import (
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const (
	errAccountExists uint32 = iota + 1
	errAccountClosed
	errAccountNotFound
	errAccountOverdrawn
	errInvalidDenomination
	errPaymentExists
	errPaymentClosed
	errPaymentNotFound
	errPaymentRateZero
	errInvalidPayment
	errInvalidSettlement
	errInvalidAccountID
	errInvalidAccount
)

var (
	ErrAccountExists       = sdkerrors.Register(ModuleName, errAccountExists, "account exists")
	ErrAccountClosed       = sdkerrors.Register(ModuleName, errAccountClosed, "account closed")
	ErrAccountNotFound     = sdkerrors.Register(ModuleName, errAccountNotFound, "account not found")
	ErrAccountOverdrawn    = sdkerrors.Register(ModuleName, errAccountOverdrawn, "account overdrawn")
	ErrInvalidDenomination = sdkerrors.Register(ModuleName, errInvalidDenomination, "invalid denomination")
	ErrPaymentExists       = sdkerrors.Register(ModuleName, errPaymentExists, "payment exists")
	ErrPaymentClosed       = sdkerrors.Register(ModuleName, errPaymentClosed, "payment closed")
	ErrPaymentNotFound     = sdkerrors.Register(ModuleName, errPaymentNotFound, "payment not found")
	ErrPaymentRateZero     = sdkerrors.Register(ModuleName, errPaymentRateZero, "payment rate zero")
	ErrInvalidPayment      = sdkerrors.Register(ModuleName, errInvalidPayment, "invalid payment")
	ErrInvalidSettlement   = sdkerrors.Register(ModuleName, errInvalidSettlement, "invalid settlement")
	ErrInvalidAccountID    = sdkerrors.Register(ModuleName, errInvalidAccountID, "invalid account ID")
	ErrInvalidAccount      = sdkerrors.Register(ModuleName, errInvalidAccount, "invalid account")
)
