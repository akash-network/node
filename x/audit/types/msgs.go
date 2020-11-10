package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const (
	MsgTypeSignProviderAttributes   = "audit-sign-provider-attributes"
	MsgTypeDeleteProviderAttributes = "audit-delete-provider-attributes"
)

var (
	_ sdk.Msg = &MsgSignProviderAttributes{}
	_ sdk.Msg = &MsgDeleteProviderAttributes{}
)

// ====MsgSignProviderAttributes====
// Route implements the sdk.Msg interface
func (m MsgSignProviderAttributes) Route() string {
	return RouterKey
}

// Type implements the sdk.Msg interface
func (m MsgSignProviderAttributes) Type() string {
	return MsgTypeSignProviderAttributes
}

// ValidateBasic does basic validation
func (m MsgSignProviderAttributes) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Owner); err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgCreate: Invalid Owner Address")
	}

	if _, err := sdk.AccAddressFromBech32(m.Validator); err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgCreate: Invalid Validator Address")
	}

	return nil
}

// GetSignBytes encodes the message for signing
func (m MsgSignProviderAttributes) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&m))
}

// GetSigners defines whose signature is required
func (m MsgSignProviderAttributes) GetSigners() []sdk.AccAddress {
	validator, err := sdk.AccAddressFromBech32(m.Validator)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{validator}
}

// ====MsgRevokeProviderAttributes====
// Route implements the sdk.Msg interface
func (m MsgDeleteProviderAttributes) Route() string {
	return RouterKey
}

// Type implements the sdk.Msg interface
func (m MsgDeleteProviderAttributes) Type() string {
	return MsgTypeDeleteProviderAttributes
}

// ValidateBasic does basic validation
func (m MsgDeleteProviderAttributes) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Owner); err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgCreate: Invalid Owner Address")
	}

	if _, err := sdk.AccAddressFromBech32(m.Validator); err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgCreate: Invalid Validator Address")
	}

	return nil
}

// GetSignBytes encodes the message for signing
func (m MsgDeleteProviderAttributes) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&m))
}

// GetSigners defines whose signature is required
func (m MsgDeleteProviderAttributes) GetSigners() []sdk.AccAddress {
	validator, err := sdk.AccAddressFromBech32(m.Validator)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{validator}
}
