package v1beta2

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
)

const (
	MsgTypeCreateBid     = "create-bid"
	MsgTypeCloseBid      = "close-bid"
	MsgTypeCreateLease   = "create-lease"
	MsgTypeWithdrawLease = "withdraw-lease"
	MsgTypeCloseLease    = "close-lease"
)

var (
	_ sdk.Msg = &MsgCreateBid{}
	_ sdk.Msg = &MsgCloseBid{}
	_ sdk.Msg = &MsgCreateLease{}
	_ sdk.Msg = &MsgWithdrawLease{}
	_ sdk.Msg = &MsgCloseLease{}
)

// NewMsgCreateBid creates a new MsgCreateBid instance
func NewMsgCreateBid(id OrderID, provider sdk.AccAddress, price sdk.DecCoin, deposit sdk.Coin) *MsgCreateBid {
	return &MsgCreateBid{
		Order:    id,
		Provider: provider.String(),
		Price:    price,
		Deposit:  deposit,
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
	provider, err := sdk.AccAddressFromBech32(msg.Provider)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{provider}
}

// ValidateBasic does basic validation of provider and order
func (msg MsgCreateBid) ValidateBasic() error {
	if err := msg.Order.Validate(); err != nil {
		return err
	}

	provider, err := sdk.AccAddressFromBech32(msg.Provider)
	if err != nil {
		return ErrEmptyProvider
	}

	owner, err := sdk.AccAddressFromBech32(msg.Order.Owner)
	if err != nil {
		return errors.Wrap(ErrInvalidBid, "empty owner")
	}

	if provider.Equals(owner) {
		return ErrSameAccount
	}

	if msg.Price.IsZero() {
		return ErrBidZeroPrice
	}

	return nil
}

// NewMsgWithdrawLease creates a new MsgWithdrawLease instance
func NewMsgWithdrawLease(id LeaseID) *MsgWithdrawLease {
	return &MsgWithdrawLease{
		LeaseID: id,
	}
}

// Route implements the sdk.Msg interface
func (msg MsgWithdrawLease) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgWithdrawLease) Type() string { return MsgTypeWithdrawLease }

// GetSignBytes encodes the message for signing
func (msg MsgWithdrawLease) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
}

// GetSigners defines whose signature is required
func (msg MsgWithdrawLease) GetSigners() []sdk.AccAddress {
	provider, err := sdk.AccAddressFromBech32(msg.GetLeaseID().Provider)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{provider}
}

// ValidateBasic does basic validation of provider and order
func (msg MsgWithdrawLease) ValidateBasic() error {
	if err := msg.LeaseID.Validate(); err != nil {
		return err
	}
	return nil
}

// NewMsgCreateLease creates a new MsgCreateLease instance
func NewMsgCreateLease(id BidID) *MsgCreateLease {
	return &MsgCreateLease{
		BidID: id,
	}
}

// Route implements the sdk.Msg interface
func (msg MsgCreateLease) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCreateLease) Type() string { return MsgTypeCreateLease }

// GetSignBytes encodes the message for signing
func (msg MsgCreateLease) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
}

// GetSigners defines whose signature is required
func (msg MsgCreateLease) GetSigners() []sdk.AccAddress {
	provider, err := sdk.AccAddressFromBech32(msg.BidID.Owner)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{provider}
}

// ValidateBasic method for MsgCreateLease
func (msg MsgCreateLease) ValidateBasic() error {
	return msg.BidID.Validate()
}

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
	provider, err := sdk.AccAddressFromBech32(msg.BidID.Provider)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{provider}
}

// ValidateBasic method for MsgCloseBid
func (msg MsgCloseBid) ValidateBasic() error {
	return msg.BidID.Validate()
}

// NewMsgCloseLease creates a new MsgCloseLease instance
func NewMsgCloseLease(id LeaseID) *MsgCloseLease {
	return &MsgCloseLease{
		LeaseID: id,
	}
}

// Route implements the sdk.Msg interface
func (msg MsgCloseLease) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCloseLease) Type() string { return MsgTypeCloseLease }

// GetSignBytes encodes the message for signing
func (msg MsgCloseLease) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
}

// GetSigners defines whose signature is required
func (msg MsgCloseLease) GetSigners() []sdk.AccAddress {
	owner, err := sdk.AccAddressFromBech32(msg.LeaseID.Owner)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{owner}
}

// ValidateBasic method for MsgCloseLease
func (msg MsgCloseLease) ValidateBasic() error {
	return msg.LeaseID.Validate()
}
