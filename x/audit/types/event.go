package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/sdkutil"
)

const (
	evActionTrustedAuditorCreated = "audit-trusted-auditor-created"
	evOwnerKey                    = "owner"
)

// EventTrustedAuditorCreated struct
type EventTrustedAuditorCreated struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	Owner   sdk.AccAddress          `json:"owner"`
}

func NewEventTrustedAuditorCreated(owner sdk.AccAddress) EventTrustedAuditorCreated {
	return EventTrustedAuditorCreated{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionTrustedAuditorCreated,
		},
		Owner: owner,
	}
}

// ToSDKEvent method creates new sdk event for EventProviderCreated struct
func (ev EventTrustedAuditorCreated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionTrustedAuditorCreated),
		}, TrustedAuditorEVAttributes(ev.Owner)...)...,
	)
}

// TrustedAuditorEVAttributes returns event attributes for given Provider
func TrustedAuditorEVAttributes(owner sdk.AccAddress) []sdk.Attribute {
	return []sdk.Attribute{
		sdk.NewAttribute(evOwnerKey, owner.String()),
	}
}

// ParseEVTTrustedAuditor returns provider details for given event attributes
func ParseEVTTrustedAuditor(attrs []sdk.Attribute) (sdk.AccAddress, error) {
	owner, err := sdkutil.GetAccAddress(attrs, evOwnerKey)
	if err != nil {
		return sdk.AccAddress{}, err
	}

	return owner, nil
}

// ParseEvent parses event and returns details of event and error if occurred
// TODO: Enable returning actual events.
func ParseEvent(ev sdkutil.Event) (sdkutil.ModuleEvent, error) {
	if ev.Type != sdkutil.EventTypeMessage {
		return nil, sdkutil.ErrUnknownType
	}
	if ev.Module != ModuleName {
		return nil, sdkutil.ErrUnknownModule
	}
	switch ev.Action {
	case evActionTrustedAuditorCreated:
		owner, err := ParseEVTTrustedAuditor(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventTrustedAuditorCreated(owner), nil
	default:
		return nil, sdkutil.ErrUnknownAction
	}
}
