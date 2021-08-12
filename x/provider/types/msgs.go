package types

import (
	"fmt"
	"net/url"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/types"
)

const (
	MsgTypeCreateProvider = "create-provider"
	MsgTypeUpdateProvider = "update-provider"
	MsgTypeDeleteProvider = "delete-provider"
)

var (
	_, _, _ sdk.Msg = &MsgCreateProvider{}, &MsgUpdateProvider{}, &MsgDeleteProvider{}
)

var (
	ErrInvalidStorageClass  = errors.New("provider: invalid storage class")
	ErrUnsupportedAttribute = errors.New("provider: unsupported attribute")
)

var allowedStorageClasses = map[string]bool{
	"default": true,
	"beta1":   true,
	"beta2":   true,
	"beta3":   true,
}

// NewMsgCreateProvider creates a new MsgCreateProvider instance
func NewMsgCreateProvider(owner sdk.AccAddress, hostURI string, attributes types.Attributes) *MsgCreateProvider {
	return &MsgCreateProvider{
		Owner:      owner.String(),
		HostURI:    hostURI,
		Attributes: attributes,
	}
}

// Route implements the sdk.Msg interface
func (msg MsgCreateProvider) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCreateProvider) Type() string { return MsgTypeCreateProvider }

// ValidateBasic does basic validation of a HostURI
func (msg MsgCreateProvider) ValidateBasic() error {
	if err := validateProviderURI(msg.HostURI); err != nil {
		return err
	}
	if _, err := sdk.AccAddressFromBech32(msg.Owner); err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgCreate: Invalid Provider Address")
	}
	if err := msg.Attributes.Validate(); err != nil {
		return err
	}
	if err := validateProviderAttributes(msg.Attributes); err != nil {
		return err
	}
	if err := msg.Info.Validate(); err != nil {
		return err
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgCreateProvider) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
}

// GetSigners defines whose signature is required
func (msg MsgCreateProvider) GetSigners() []sdk.AccAddress {
	owner, err := sdk.AccAddressFromBech32(msg.Owner)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{owner}
}

// NewMsgUpdateProvider creates a new MsgUpdateProvider instance
func NewMsgUpdateProvider(owner sdk.AccAddress, hostURI string, attributes types.Attributes) *MsgUpdateProvider {
	return &MsgUpdateProvider{
		Owner:      owner.String(),
		HostURI:    hostURI,
		Attributes: attributes,
	}
}

// Route implements the sdk.Msg interface
func (msg MsgUpdateProvider) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgUpdateProvider) Type() string { return MsgTypeUpdateProvider }

// ValidateBasic does basic validation of a ProviderURI
func (msg MsgUpdateProvider) ValidateBasic() error {
	if err := validateProviderURI(msg.HostURI); err != nil {
		return err
	}
	if _, err := sdk.AccAddressFromBech32(msg.Owner); err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgUpdate: Invalid Provider Address")
	}
	if err := msg.Attributes.Validate(); err != nil {
		return err
	}
	if err := validateProviderAttributes(msg.Attributes); err != nil {
		return err
	}
	if err := msg.Info.Validate(); err != nil {
		return err
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgUpdateProvider) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
}

// GetSigners defines whose signature is required
func (msg MsgUpdateProvider) GetSigners() []sdk.AccAddress {
	owner, err := sdk.AccAddressFromBech32(msg.Owner)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{owner}
}

// NewMsgDeleteProvider creates a new MsgDeleteProvider instance
func NewMsgDeleteProvider(owner sdk.AccAddress) *MsgDeleteProvider {
	return &MsgDeleteProvider{
		Owner: owner.String(),
	}
}

// Route implements the sdk.Msg interface
func (msg MsgDeleteProvider) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgDeleteProvider) Type() string { return MsgTypeDeleteProvider }

// ValidateBasic does basic validation
func (msg MsgDeleteProvider) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Owner); err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgDelete: Invalid Provider Address")
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgDeleteProvider) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
}

// GetSigners defines whose signature is required
func (msg MsgDeleteProvider) GetSigners() []sdk.AccAddress {
	owner, err := sdk.AccAddressFromBech32(msg.Owner)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{owner}
}

func validateProviderURI(val string) error {
	u, err := url.Parse(val)
	if err != nil {
		return ErrInvalidProviderURI
	}
	if !u.IsAbs() {
		return errors.Wrapf(ErrNotAbsProviderURI, "validating %q for absolute URI", val)
	}

	if u.Scheme != "https" {
		return errors.Wrapf(ErrInvalidProviderURI, "scheme in %q should be https", val)
	}

	if u.Host == "" {
		return errors.Wrapf(ErrInvalidProviderURI, "validating %q for valid host", val)
	}

	if u.Path != "" {
		return errors.Wrapf(ErrInvalidProviderURI, "path in %q should be empty", val)
	}

	return nil
}

func validateProviderAttributes(attrs types.Attributes) error {
	storage := attrs.GetCapabilitiesGroup("storage")
	for _, group := range storage {
		for _, attr := range group {
			switch attr.Key {
			case "persistent":
				if _, err := strconv.ParseBool(attr.Value); err != nil {
					return err
				}
			case "class":
				if _, valid := allowedStorageClasses[attr.Value]; !valid {
					return errors.Wrap(ErrInvalidStorageClass, attr.Value)
				}
			default:
				return errors.Wrap(ErrUnsupportedAttribute, fmt.Sprintf("%s for capability group storage", attr.Key))
			}
		}
	}

	return nil
}
