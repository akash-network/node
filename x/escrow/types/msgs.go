package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const (
	MsgTypeAccountWithdraw = "account-withdraw"
	MsgTypePaymentWithdraw = "payment-withdraw"
)

var (
	_ sdk.Msg = &MsgAccountWithdraw{}
	_ sdk.Msg = &MsgPaymentWithdraw{}
)

// ====MsgAccountWithdraw====
// Route implements the sdk.Msg interface
func (m MsgAccountWithdraw) Route() string {
	return RouterKey
}

// Type implements the sdk.Msg interface
func (m MsgAccountWithdraw) Type() string {
	return MsgTypeAccountWithdraw
}

// ValidateBasic does basic validation
func (m MsgAccountWithdraw) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Owner); err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgCreate: Invalid Owner Address")
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (m MsgAccountWithdraw) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&m))
}

// GetSigners defines whose signature is required
func (m MsgAccountWithdraw) GetSigners() []sdk.AccAddress {
	owner, err := sdk.AccAddressFromBech32(m.Owner)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{owner}
}

// ====MsgPaymentWithdraw====
// Route implements the sdk.Msg interface
func (m MsgPaymentWithdraw) Route() string {
	return RouterKey
}

// Type implements the sdk.Msg interface
func (m MsgPaymentWithdraw) Type() string {
	return MsgTypePaymentWithdraw
}

// ValidateBasic does basic validation
func (m MsgPaymentWithdraw) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Owner); err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgCreate: Invalid Owner Address")
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (m MsgPaymentWithdraw) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&m))
}

// GetSigners defines whose signature is required
func (m MsgPaymentWithdraw) GetSigners() []sdk.AccAddress {
	owner, err := sdk.AccAddressFromBech32(m.Owner)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{owner}
}
