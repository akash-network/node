package types

import (
	"net/url"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const (
	msgTypeCreateProvider = "create-provider"
	msgTypeUpdateProvider = "update-provider"
	msgTypeDeleteProvider = "delete-provider"
)

// MsgCreate defines an SDK message for creating a provider
type MsgCreateProvider Provider

// Route implements the sdk.Msg interface
func (msg MsgCreateProvider) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCreateProvider) Type() string { return msgTypeCreateProvider }

// ValidateBasic does basic validation of a HostURI
func (msg MsgCreateProvider) ValidateBasic() error {
	if err := validateProviderURI(msg.HostURI); err != nil {
		return err
	}
	if msg.Owner.Empty() {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgCreate: Invalid Provider Address")
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgCreateProvider) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgCreateProvider) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Owner}
}

// MsgUpdate defines an SDK message for updating a provider
type MsgUpdateProvider Provider

// Route implements the sdk.Msg interface
func (msg MsgUpdateProvider) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgUpdateProvider) Type() string { return msgTypeUpdateProvider }

// ValidateBasic does basic validation of a ProviderURI
func (msg MsgUpdateProvider) ValidateBasic() error {
	if err := validateProviderURI(msg.HostURI); err != nil {
		return err
	}
	if msg.Owner.Empty() {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgUpdate: Invalid Provider Address")
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgUpdateProvider) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgUpdateProvider) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Owner}
}

// MsgDelete defines an SDK message for deleting a provider
type MsgDeleteProvider struct {
	Owner sdk.AccAddress `json:"owner"`
}

// Route implements the sdk.Msg interface
func (msg MsgDeleteProvider) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgDeleteProvider) Type() string { return msgTypeDeleteProvider }

// ValidateBasic does basic validation
func (msg MsgDeleteProvider) ValidateBasic() error {
	if msg.Owner.Empty() {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgDelete: Invalid Provider Address")
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgDeleteProvider) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgDeleteProvider) GetSigners() []sdk.AccAddress {
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
