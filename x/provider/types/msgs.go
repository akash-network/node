package types

import (
	"net/url"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// MsgCreate defines an SDK message for creating a provider
type MsgCreate Provider

// Route implements the sdk.Msg interface
func (msg MsgCreate) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCreate) Type() string { return "create" }

// ValidateBasic does basic validation of a HostURI
func (msg MsgCreate) ValidateBasic() error {
	if err := validateProviderURI(msg.HostURI); err != nil {
		return err
	}
	if msg.Owner.Empty() {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgCreate: Invalid Provider Address")
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

// MsgUpdate defines an SDK message for updating a provider
type MsgUpdate Provider

// Route implements the sdk.Msg interface
func (msg MsgUpdate) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgUpdate) Type() string { return "update" }

// ValidateBasic does basic validation of a ProviderURI
func (msg MsgUpdate) ValidateBasic() error {
	if err := validateProviderURI(msg.HostURI); err != nil {
		return err
	}
	if msg.Owner.Empty() {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgUpdate: Invalid Provider Address")
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

// MsgDelete defines an SDK message for deleting a provider
type MsgDelete struct {
	Owner sdk.AccAddress `json:"owner"`
}

// Route implements the sdk.Msg interface
func (msg MsgDelete) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgDelete) Type() string { return "delete" }

// ValidateBasic does basic validation
func (msg MsgDelete) ValidateBasic() error {
	if msg.Owner.Empty() {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgDelete: Invalid Provider Address")
	}
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

func validateProviderURI(val string) error {
	u, err := url.Parse(val)
	if err != nil {
		return ErrInvalidProviderURI
	}
	if !u.IsAbs() {
		return ErrNotAbsProviderURI
	}
	return nil
}
