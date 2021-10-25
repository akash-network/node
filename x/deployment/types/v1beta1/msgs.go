package v1beta1

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	MsgTypeCreateDeployment  = "create-deployment"
	MsgTypeDepositDeployment = "deposit-deployment"
	MsgTypeUpdateDeployment  = "update-deployment"
	MsgTypeCloseDeployment   = "close-deployment"
	MsgTypeCloseGroup        = "close-group"
	MsgTypePauseGroup        = "pause-group"
	MsgTypeStartGroup        = "start-group"
)

var (
	_, _, _, _ sdk.Msg = &MsgCreateDeployment{}, &MsgUpdateDeployment{}, &MsgCloseDeployment{}, &MsgCloseGroup{}
)

// NewMsgCreateDeployment creates a new MsgCreateDeployment instance
func NewMsgCreateDeployment(id DeploymentID, groups []GroupSpec, version []byte,
	deposit sdk.Coin, depositor sdk.AccAddress) *MsgCreateDeployment {
	return &MsgCreateDeployment{
		ID:      id,
		Groups:  groups,
		Version: version,
		Deposit: deposit,
	}
}

// Route implements the sdk.Msg interface
func (msg MsgCreateDeployment) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgCreateDeployment) Type() string { return MsgTypeCreateDeployment }

// GetSignBytes encodes the message for signing
func (msg MsgCreateDeployment) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
}

// GetSigners defines whose signature is required
func (msg MsgCreateDeployment) GetSigners() []sdk.AccAddress {
	owner, err := sdk.AccAddressFromBech32(msg.ID.Owner)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{owner}
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

	if len(msg.Version) != ManifestVersionLength {
		return ErrInvalidVersion
	}

	for _, gs := range msg.Groups {
		err := gs.ValidateBasic()
		if err != nil {
			return err
		}
	}

	return nil
}

// NewMsgDepositDeployment creates a new MsgDepositDeployment instance
func NewMsgDepositDeployment(id DeploymentID, amount sdk.Coin, depositor string) *MsgDepositDeployment {
	return &MsgDepositDeployment{
		ID:     id,
		Amount: amount,
	}
}

// Route implements the sdk.Msg interface
func (msg MsgDepositDeployment) Route() string { return RouterKey }

// Type implements the sdk.Msg interface
func (msg MsgDepositDeployment) Type() string { return MsgTypeDepositDeployment }

// GetSignBytes encodes the message for signing
func (msg MsgDepositDeployment) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
}

// GetSigners defines whose signature is required
func (msg MsgDepositDeployment) GetSigners() []sdk.AccAddress {
	owner, err := sdk.AccAddressFromBech32(msg.ID.Owner)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{owner}
}

// ValidateBasic does basic validation like check owner and groups length
func (msg MsgDepositDeployment) ValidateBasic() error {
	if err := msg.ID.Validate(); err != nil {
		return err
	}

	if msg.Amount.IsZero() {
		return ErrInvalidDeposit
	}

	return nil
}

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

	if len(msg.Version) != ManifestVersionLength {
		return ErrInvalidVersion
	}

	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgUpdateDeployment) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
}

// GetSigners defines whose signature is required
func (msg MsgUpdateDeployment) GetSigners() []sdk.AccAddress {
	owner, err := sdk.AccAddressFromBech32(msg.ID.Owner)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{owner}
}

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
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
}

// GetSigners defines whose signature is required
func (msg MsgCloseDeployment) GetSigners() []sdk.AccAddress {
	owner, err := sdk.AccAddressFromBech32(msg.ID.Owner)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{owner}
}

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
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
}

// GetSigners defines whose signature is required
func (msg MsgCloseGroup) GetSigners() []sdk.AccAddress {
	owner, err := sdk.AccAddressFromBech32(msg.ID.Owner)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{owner}
}

// NewMsgPauseGroup creates a new MsgPauseGroup instance
func NewMsgPauseGroup(id GroupID) *MsgPauseGroup {
	return &MsgPauseGroup{
		ID: id,
	}
}

// Route implements the sdk.Msg interface for routing
func (msg MsgPauseGroup) Route() string { return RouterKey }

// Type implements the sdk.Msg interface exposing message type
func (msg MsgPauseGroup) Type() string { return MsgTypePauseGroup }

// ValidateBasic calls underlying GroupID.Validate() check and returns result
func (msg MsgPauseGroup) ValidateBasic() error {
	if err := msg.ID.Validate(); err != nil {
		return err
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgPauseGroup) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
}

// GetSigners defines whose signature is required
func (msg MsgPauseGroup) GetSigners() []sdk.AccAddress {
	owner, err := sdk.AccAddressFromBech32(msg.ID.Owner)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{owner}
}

// NewMsgStartGroup creates a new MsgStartGroup instance
func NewMsgStartGroup(id GroupID) *MsgStartGroup {
	return &MsgStartGroup{
		ID: id,
	}
}

// Route implements the sdk.Msg interface for routing
func (msg MsgStartGroup) Route() string { return RouterKey }

// Type implements the sdk.Msg interface exposing message type
func (msg MsgStartGroup) Type() string { return MsgTypeStartGroup }

// ValidateBasic calls underlying GroupID.Validate() check and returns result
func (msg MsgStartGroup) ValidateBasic() error {
	if err := msg.ID.Validate(); err != nil {
		return err
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (msg MsgStartGroup) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&msg))
}

// GetSigners defines whose signature is required
func (msg MsgStartGroup) GetSigners() []sdk.AccAddress {
	owner, err := sdk.AccAddressFromBech32(msg.ID.Owner)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{owner}
}
