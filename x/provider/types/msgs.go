package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type MsgCreate Provider

func (msg MsgCreate) Route() string { return RouterKey }
func (msg MsgCreate) Type() string  { return "create" }
func (msg MsgCreate) ValidateBasic() error {
	switch {
	case len(msg.HostURI) == 0:
		// TODO: better uri validation
		return ErrInvalidProviderURI
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgCreate) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgCreate) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Owner}
}

type MsgUpdate Provider

func (msg MsgUpdate) Route() string { return RouterKey }
func (msg MsgUpdate) Type() string  { return "update" }
func (msg MsgUpdate) ValidateBasic() error {
	switch {
	case len(msg.HostURI) == 0:
		// TODO: better uri validation
		return ErrInvalidProviderURI
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgUpdate) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgUpdate) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Owner}
}

type MsgDelete struct {
	Owner sdk.AccAddress `json:"owner"`
}

func (msg MsgDelete) Route() string { return RouterKey }
func (msg MsgDelete) Type() string  { return "delete" }
func (msg MsgDelete) ValidateBasic() error {
	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgDelete) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgDelete) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Owner}
}
