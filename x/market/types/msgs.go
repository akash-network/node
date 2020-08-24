package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	MsgTypeCreateBid  = "create-bid"
	MsgTypeCloseBid   = "close-bid"
	MsgTypeCloseOrder = "close-order"
)

var (
	_, _, _ sdk.Msg = &MsgCreateBid{}, &MsgCloseBid{}, &MsgCloseOrder{}
)

// MsgCreateBid defines an SDK message for creating Bid
// type MsgCreateBid struct {
// 	Order    OrderID        `json:"order"`
// 	Provider sdk.AccAddress `json:"owner"`
// 	Price    sdk.Coin       `json:"price"`
// }

// NewMsgCreateBid creates a new MsgCreateBid instance
func NewMsgCreateBid(id OrderID, provider sdk.AccAddress, price sdk.Coin) *MsgCreateBid {
	return &MsgCreateBid{
		Order:    id,
		Provider: provider,
		Price:    price,
	}
}

// Route implements the sdk.Msg interface
func (msg MsgCreateBid) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCreateBid) Type() string { return MsgTypeCreateBid }

// GetSignBytes encodes the message for signing
func (msg MsgCreateBid) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
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
// type MsgCloseBid struct {
// 	BidID `json:"id"`
// }

// NewMsgCloseBid creates a new MsgCloseBid instance
func NewMsgCloseBid(id BidID) *MsgCloseBid {
	return &MsgCloseBid{
		BidID: id,
	}
}

// Route implements the sdk.Msg interface
func (msg MsgCloseBid) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCloseBid) Type() string { return MsgTypeCloseBid }

// GetSignBytes encodes the message for signing
func (msg MsgCloseBid) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
}

// GetSigners defines whose signature is required
func (msg MsgCloseBid) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.BidID.Provider}
}

// ValidateBasic method for MsgCloseBid
func (msg MsgCloseBid) ValidateBasic() error {
	return msg.BidID.Validate()
}

// MsgCloseOrder defines an SDK message for closing order
// type MsgCloseOrder struct {
// 	OrderID `json:"id"`
// }

// NewMsgCloseOrder creates a new MsgCloseOrder instance
func NewMsgCloseOrder(id OrderID) *MsgCloseOrder {
	return &MsgCloseOrder{
		OrderID: id,
	}
}

// Route implements the sdk.Msg interface
func (msg MsgCloseOrder) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCloseOrder) Type() string { return MsgTypeCloseOrder }

// GetSignBytes encodes the message for signing
func (msg MsgCloseOrder) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
}

// GetSigners defines whose signature is required
func (msg MsgCloseOrder) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.OrderID.Owner}
}

// ValidateBasic method for MsgCloseOrder
func (msg MsgCloseOrder) ValidateBasic() error {
	return msg.OrderID.Validate()
}
