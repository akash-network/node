package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	MsgTypeCreateDeployment = "create-deployment"
	MsgTypeUpdateDeployment = "update-deployment"
	MsgTypeCloseDeployment  = "close-deployment"
	MsgTypeCloseGroup       = "close-group"
)

var (
	_, _, _, _ sdk.Msg = &MsgCreateDeployment{}, &MsgUpdateDeployment{}, &MsgCloseDeployment{}, &MsgCloseGroup{}
)

// MsgCreateDeployment defines an SDK message for creating deployment
// type MsgCreateDeployment struct {
// 	ID      DeploymentID `json:"id"`
// 	Groups  []GroupSpec  `json:"groups"`
// 	Version []byte       `json:"version"`
// }

// NewMsgCreateDeployment creates a new MsgCreateDeployment instance
func NewMsgCreateDeployment(id DeploymentID, groups []GroupSpec, version []byte) *MsgCreateDeployment {
	return &MsgCreateDeployment{
		ID:      id,
		Groups:  groups,
		Version: version,
	}
}

// Route implements the sdk.Msg interface
func (msg MsgCreateDeployment) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCreateDeployment) Type() string { return MsgTypeCreateDeployment }

// GetSignBytes encodes the message for signing
func (msg MsgCreateDeployment) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgCreateDeployment) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.ID.Owner}
}

// ValidateBasic does basic validation like check owner and groups length
func (msg MsgCreateDeployment) ValidateBasic() error {
	if err := msg.ID.Validate(); err != nil {
		return err
	}
	if len(msg.Groups) == 0 {
		return ErrInvalidGroups
	}
	if len(msg.Version) == 0 {
		return ErrEmptyVersion
	}
	return nil
}

// MsgUpdateDeployment defines an SDK message for updating deployment
// type MsgUpdateDeployment struct {
// 	ID      DeploymentID
// 	Groups  []GroupSpec
// 	Version []byte
// }

// NewMsgUpdateDeployment creates a new MsgUpdateDeployment instance
func NewMsgUpdateDeployment(id DeploymentID, groups []GroupSpec, version []byte) *MsgUpdateDeployment {
	return &MsgUpdateDeployment{
		ID:      id,
		Groups:  groups,
		Version: version,
	}
}

// Route implements the sdk.Msg interface
func (msg MsgUpdateDeployment) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgUpdateDeployment) Type() string { return MsgTypeUpdateDeployment }

// ValidateBasic does basic validation
func (msg MsgUpdateDeployment) ValidateBasic() error {
	if err := msg.ID.Validate(); err != nil {
		return err
	}

	if len(msg.Version) == 0 {
		return ErrEmptyVersion
	}

	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgUpdateDeployment) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgUpdateDeployment) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.ID.Owner}
}

// MsgCloseDeployment defines an SDK message for closing deployment
// type MsgCloseDeployment struct {
// 	ID DeploymentID
// }

// NewMsgCloseDeployment creates a new MsgCloseDeployment instance
func NewMsgCloseDeployment(id DeploymentID) *MsgCloseDeployment {
	return &MsgCloseDeployment{
		ID: id,
	}
}

// Route implements the sdk.Msg interface
func (msg MsgCloseDeployment) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCloseDeployment) Type() string { return MsgTypeCloseDeployment }

// ValidateBasic does basic validation with deployment details
func (msg MsgCloseDeployment) ValidateBasic() error {
	if err := msg.ID.Validate(); err != nil {
		return err
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgCloseDeployment) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgCloseDeployment) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.ID.Owner}
}

// MsgCloseGroup defines SDK message to close a single Group within a Deployment.
// type MsgCloseGroup struct {
// 	ID GroupID
// }

// NewMsgCloseGroup creates a new MsgCloseGroup instance
func NewMsgCloseGroup(id GroupID) *MsgCloseGroup {
	return &MsgCloseGroup{
		ID: id,
	}
}

// Route implements the sdk.Msg interface for routing
func (msg MsgCloseGroup) Route() string { return RouterKey }

// Type implements the sdk.Msg interface exposing message type
func (msg MsgCloseGroup) Type() string { return MsgTypeCloseGroup }

// ValidateBasic calls underlying GroupID.Validate() check and returns result
func (msg MsgCloseGroup) ValidateBasic() error {
	if err := msg.ID.Validate(); err != nil {
		return err
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgCloseGroup) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(msg))
}

// GetSigners defines whose signature is required
func (msg MsgCloseGroup) GetSigners() []sdk.AccAddress {
	return []sdk.AccAddress{msg.ID.Owner}
}
