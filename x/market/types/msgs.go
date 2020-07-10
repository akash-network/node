package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	msgTypeCreateBid  = "create-bid"
	msgTypeCloseBid   = "close-bid"
	msgTypeCloseOrder = "close-order"
)

// MsgCreateBid defines an SDK message for creating Bid
type MsgCreateBid struct {
	Order         OrderID         `json:"order"`
	Provider      sdk.AccAddress  `json:"owner"`
	Price         sdk.Coin        `json:"price"`
	RequiredAttrs []sdk.Attribute `json:"attributes"`
}

// Route implements the sdk.Msg interface
func (msg MsgCreateBid) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCreateBid) Type() string { return msgTypeCreateBid }

// GetSignBytes encodes the message for signing
func (msg MsgCreateBid) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgCreateBid) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Provider}
}

// ValidateBasic does basic validation of provider and order
func (msg MsgCreateBid) ValidateBasic() error {
	if err := msg.Order.Validate(); err != nil {
		return err
	}

	if err := sdk.VerifyAddressFormat(msg.Provider); err != nil {
		return ErrEmptyProvider
	}

	if msg.Provider.Equals(msg.Order.Owner) {
		return ErrSameAccount
	}

	return nil
}

// MsgCloseBid defines an SDK message for closing bid
type MsgCloseBid struct {
	BidID `json:"id"`
}

// Route implements the sdk.Msg interface
func (msg MsgCloseBid) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCloseBid) Type() string { return msgTypeCloseBid }

// GetSignBytes encodes the message for signing
func (msg MsgCloseBid) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgCloseBid) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Provider}
}

// ValidateBasic method for MsgCloseBid
func (msg MsgCloseBid) ValidateBasic() error {
	return msg.BidID.Validate()
}

// MsgCloseOrder defines an SDK message for closing order
type MsgCloseOrder struct {
	OrderID `json:"id"`
}

// Route implements the sdk.Msg interface
func (msg MsgCloseOrder) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCloseOrder) Type() string { return msgTypeCloseOrder }

// GetSignBytes encodes the message for signing
func (msg MsgCloseOrder) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgCloseOrder) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Owner}
}

// ValidateBasic method for MsgCloseOrder
func (msg MsgCloseOrder) ValidateBasic() error {
	return msg.OrderID.Validate()
}
