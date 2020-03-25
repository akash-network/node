package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// msg types
const (
	typeMsgCreateBid  = "create_bid"
	typeMsgCloseBid   = "close_bid"
	typeMsgCloseOrder = "close_order"
)

// MsgCreateBid defines an SDK message for creating Bid
type MsgCreateBid struct {
	Order    OrderID        `json:"order"`
	Provider sdk.AccAddress `json:"provider"`
	Price    sdk.Coin       `json:"price"`
}

// Route implements the sdk.Msg interface
func (msg MsgCreateBid) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCreateBid) Type() string { return typeMsgCreateBid }

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
	if msg.Provider.Empty() {
		return ErrEmptyProvider
	}

	if msg.Provider.Equals(msg.Order.Owner) {
		return ErrSameAccount
	}

	if !msg.Price.IsValid() {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidCoins, msg.Price.String())
	}

	return msg.Order.Validate()
}

// MsgCloseBid defines an SDK message for closing bid
type MsgCloseBid struct {
	BidID `json:"id"`
}

// Route implements the sdk.Msg interface
func (msg MsgCloseBid) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCloseBid) Type() string { return typeMsgCloseBid }

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
func (msg MsgCloseOrder) Type() string { return typeMsgCloseOrder }

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
