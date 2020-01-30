package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type MsgCreateBid struct {
	Order    OrderID        `json:"order"`
	Provider sdk.AccAddress `json:"owner"`
	Price    sdk.Coin       `json:"price"`
}

func (msg MsgCreateBid) Route() string { return RouterKey }
func (msg MsgCreateBid) Type() string  { return "create-bid" }
func (msg MsgCreateBid) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}
func (msg MsgCreateBid) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Provider}
}

func (msg MsgCreateBid) ValidateBasic() error {
	if err := msg.Order.Validate(); err != nil {
		return ErrInvalidOrder
	}

	if msg.Provider.Empty() {
		return ErrEmptyProvider
	}

	if msg.Provider.Equals(msg.Order.Owner) {
		return ErrSameAccount
	}

	return nil
}

type MsgCloseBid struct {
	BidID `json:"id"`
}

func (msg MsgCloseBid) Route() string { return RouterKey }
func (msg MsgCloseBid) Type() string  { return "close-bid" }
func (msg MsgCloseBid) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}
func (msg MsgCloseBid) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Provider}
}
func (msg MsgCloseBid) ValidateBasic() error {
	return nil
}

type MsgCloseOrder struct {
	OrderID `json:"id"`
}

func (msg MsgCloseOrder) Route() string { return RouterKey }
func (msg MsgCloseOrder) Type() string  { return "close-order" }
func (msg MsgCloseOrder) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}
func (msg MsgCloseOrder) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Owner}
}
func (msg MsgCloseOrder) ValidateBasic() error {
	return nil
}
