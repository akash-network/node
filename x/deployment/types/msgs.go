package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// MsgCreate defines an SDK message for creating deployment
type MsgCreate struct {
	Owner sdk.AccAddress `json:"owner"`
	// Sequence uint64         `json:"sequence"`
	// Version []byte      `json:"version"`
	Groups []GroupSpec `json:"groups"`
}

// Route implements the sdk.Msg interface
func (msg MsgCreate) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCreate) Type() string { return "create" }

// GetSignBytes encodes the message for signing
func (msg MsgCreate) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgCreate) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Owner}
}

// ValidateBasic does basic validation like check owner and groups length
func (msg MsgCreate) ValidateBasic() error {
	switch {
	case msg.Owner.Empty():
		return ErrOwnerAcctMissing
	// case msg.Sequence == 0:
	// 	return sdk.NewError(DefaultCodespace, CodeInvalidRequest, "invalid sequence: 0")
	// TODO: version
	// case msg.Version.Empty():
	// 	return sdk.NewError(DefaultCodespace, CodeInvalidRequest, "invalid: empty version")
	case len(msg.Groups) == 0:
		return ErrEmptyGroups
	}
	return nil
}

// MsgUpdate defines an SDK message for updating deployment
type MsgUpdate struct {
	ID      DeploymentID
	Version sdk.AccAddress
}

// Route implements the sdk.Msg interface
func (msg MsgUpdate) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgUpdate) Type() string { return "update" }

// ValidateBasic does basic validation
func (msg MsgUpdate) ValidateBasic() error {
	if err := msg.ID.Validate(); err != nil {
		return err
	}
	switch {
	case msg.Version.Empty():
		return ErrEmptyVersion
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgUpdate) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgUpdate) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.ID.Owner}
}

// MsgClose defines an SDK message for closing deployment
type MsgClose struct {
	ID DeploymentID
}

// Route implements the sdk.Msg interface
func (msg MsgClose) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgClose) Type() string { return "update" }

// ValidateBasic does basic validation with deployment details
func (msg MsgClose) ValidateBasic() error {
	if err := msg.ID.Validate(); err != nil {
		return err
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgClose) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgClose) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.ID.Owner}
}
