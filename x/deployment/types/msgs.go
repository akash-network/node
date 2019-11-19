package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type MsgCreate struct {
	Owner sdk.AccAddress `json:"owner"`
	// Sequence uint64         `json:"sequence"`
	// Version []byte      `json:"version"`
	Groups []GroupSpec `json:"groups"`
}

func (msg MsgCreate) Route() string { return RouterKey }
func (msg MsgCreate) Type() string  { return "create" }

func (msg MsgCreate) GetSignBytes() []byte {
	return sdk.MustSortJSON(cdc.MustMarshalJSON(msg))
}

func (msg MsgCreate) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.Owner}
}

func (msg MsgCreate) ValidateBasic() sdk.Error {
	switch {
	case msg.Owner.Empty():
		return sdk.NewError(DefaultCodespace, CodeInvalidRequest, "owner account missing")
	// case msg.Sequence == 0:
	// 	return sdk.NewError(DefaultCodespace, CodeInvalidRequest, "invalid sequence: 0")
	// TODO: version
	// case msg.Version.Empty():
	// 	return sdk.NewError(DefaultCodespace, CodeInvalidRequest, "invalid: empty version")
	case len(msg.Groups) == 0:
		return sdk.NewError(DefaultCodespace, CodeInvalidRequest, "invalid: empty groups")
	}
	return nil
}

type MsgUpdate struct {
	ID      DeploymentID
	Version sdk.Address
}

func (msg MsgUpdate) Route() string { return RouterKey }
func (msg MsgUpdate) Type() string  { return "update" }

func (msg MsgUpdate) ValidateBasic() sdk.Error {
	if err := msg.ID.Validate(); err != nil {
		return sdk.NewError(DefaultCodespace, CodeInvalidRequest, "invalid deployment id")
	}
	switch {
	case msg.Version.Empty():
		return sdk.NewError(DefaultCodespace, CodeInvalidRequest, "invalid: empty version")
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

type MsgClose struct {
	ID DeploymentID
}

func (msg MsgClose) Route() string { return RouterKey }
func (msg MsgClose) Type() string  { return "update" }

func (msg MsgClose) ValidateBasic() sdk.Error {
	if err := msg.ID.Validate(); err != nil {
		return sdk.NewError(DefaultCodespace, CodeInvalidRequest, "invalid deployment id")
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
