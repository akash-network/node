package types

import (
	"net/url"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// msg types
const (
	TypeMsgCreateProvider = "create_provider"
	TypeMsgUpdateProvider = "update_provider"
	TypeMsgDeleteProvider = "delete_provider"
)

// MsgCreateProvider defines an SDK message for creating a provider
type MsgCreateProvider Provider

// Route implements the sdk.Msg interface
func (msg MsgCreateProvider) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCreateProvider) Type() string { return TypeMsgCreateProvider }

// ValidateBasic does basic validation of a HostURI
func (msg MsgCreateProvider) ValidateBasic() error {
	u, err := url.Parse(msg.HostURI)
	if err != nil {
		return sdkerrors.Wrap(ErrInvalidProviderURI, msg.HostURI)
	}
	if !u.IsAbs() {
		return sdkerrors.Wrap(ErrNotAbsProviderURI, msg.HostURI)
	}
	if msg.Owner.Empty() {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "address cannot be empty")
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

// MsgUpdateProvider defines an SDK message for updating a provider
type MsgUpdateProvider Provider

// Route implements the sdk.Msg interface
func (msg MsgUpdateProvider) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgUpdateProvider) Type() string { return TypeMsgUpdateProvider }

// ValidateBasic does basic validation of a ProviderURI
func (msg MsgUpdateProvider) ValidateBasic() error {
	u, err := url.Parse(msg.HostURI)
	if err != nil {
		return sdkerrors.Wrap(ErrInvalidProviderURI, msg.HostURI)
	}
	if !u.IsAbs() {
		return sdkerrors.Wrap(ErrNotAbsProviderURI, msg.HostURI)
	}
	if msg.Owner.Empty() {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "address cannot be empty")
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

// MsgDeleteProvider defines an SDK message for deleting a provider
type MsgDeleteProvider struct {
	Owner sdk.AccAddress `json:"owner"`
}

// Route implements the sdk.Msg interface
func (msg MsgDeleteProvider) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgDeleteProvider) Type() string { return TypeMsgDeleteProvider }

// ValidateBasic does basic validation
func (msg MsgDeleteProvider) ValidateBasic() error {
	if msg.Owner.Empty() {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "address cannot be empty")
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
