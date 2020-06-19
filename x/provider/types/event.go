package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/sdkutil"
)

const (
	evActionProviderCreate = "provider-create"
	evActionProviderUpdate = "provider-update"
	evActionProviderDelete = "provider-delete"
	evOwnerKey             = "owner"
)

// EventProviderCreate struct
type EventProviderCreate struct {
	Owner sdk.AccAddress
}

// ToSDKEvent method creates new sdk event for EventProviderCreate struct
func (ev EventProviderCreate) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionProviderCreate),
		}, ProviderEVAttributes(ev.Owner)...)...,
	)
}

// EventProviderUpdate struct
type EventProviderUpdate struct {
	Owner sdk.AccAddress
}

// ToSDKEvent method creates new sdk event for EventProviderUpdate struct
func (ev EventProviderUpdate) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionProviderUpdate),
		}, ProviderEVAttributes(ev.Owner)...)...,
	)
}

// EventProviderDelete struct
type EventProviderDelete struct {
	Owner sdk.AccAddress
}

// ToSDKEvent method creates new sdk event for EventProviderDelete struct
func (ev EventProviderDelete) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionProviderDelete),
		}, ProviderEVAttributes(ev.Owner)...)...,
	)
}

// ProviderEVAttributes returns event attribues for given Provider
func ProviderEVAttributes(owner sdk.AccAddress) []sdk.Attribute {
	return []sdk.Attribute{
		sdk.NewAttribute(evOwnerKey, owner.String()),
	}
}

// ParseEVProvider returns provider details for given event attributes
func ParseEVProvider(attrs []sdk.Attribute) (sdk.AccAddress, error) {
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
	case evActionProviderCreate:
		owner, err := ParseEVProvider(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return EventProviderCreate{Owner: owner}, nil
	case evActionProviderUpdate:
		owner, err := ParseEVProvider(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return EventProviderUpdate{Owner: owner}, nil
	case evActionProviderDelete:
		owner, err := ParseEVProvider(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return EventProviderDelete{Owner: owner}, nil
	default:
		return nil, sdkutil.ErrUnknownAction
	}
}
