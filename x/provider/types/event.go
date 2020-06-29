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
	Context sdkutil.BaseModuleEvent `json:"context"`
	Owner   sdk.AccAddress          `json:"owner"`
}

func NewEventProviderCreate(owner sdk.AccAddress) EventProviderCreate {
	return EventProviderCreate{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionProviderCreate,
		},
		Owner: owner,
	}
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
	Context sdkutil.BaseModuleEvent `json:"context"`
	Owner   sdk.AccAddress          `json:"owner"`
}

func NewEventProviderUpdate(owner sdk.AccAddress) EventProviderUpdate {
	return EventProviderUpdate{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionProviderUpdate,
		},
		Owner: owner,
	}
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
	Context sdkutil.BaseModuleEvent `json:"context"`
	Owner   sdk.AccAddress          `json:"owner"`
}

func NewEventProviderDelete(owner sdk.AccAddress) EventProviderDelete {
	return EventProviderDelete{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionProviderDelete,
		},
		Owner: owner,
	}
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
		return NewEventProviderCreate(owner), nil
	case evActionProviderUpdate:
		owner, err := ParseEVProvider(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventProviderUpdate(owner), nil
	case evActionProviderDelete:
		owner, err := ParseEVProvider(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventProviderDelete(owner), nil
	default:
		return nil, sdkutil.ErrUnknownAction
	}
}
