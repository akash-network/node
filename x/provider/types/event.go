package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/sdkutil"
)

const (
	evActionProviderCreated = "provider-created"
	evActionProviderUpdated = "provider-updated"
	evActionProviderDeleted = "provider-deleted"
	evOwnerKey              = "owner"
)

// EventProviderCreated struct
type EventProviderCreated struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	Owner   sdk.AccAddress          `json:"owner"`
}

func NewEventProviderCreated(owner sdk.AccAddress) EventProviderCreated {
	return EventProviderCreated{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionProviderCreated,
		},
		Owner: owner,
	}
}

// ToSDKEvent method creates new sdk event for EventProviderCreated struct
func (ev EventProviderCreated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionProviderCreated),
		}, ProviderEVAttributes(ev.Owner)...)...,
	)
}

// EventProviderUpdated struct
type EventProviderUpdated struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	Owner   sdk.AccAddress          `json:"owner"`
}

func NewEventProviderUpdated(owner sdk.AccAddress) EventProviderUpdated {
	return EventProviderUpdated{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionProviderUpdated,
		},
		Owner: owner,
	}
}

// ToSDKEvent method creates new sdk event for EventProviderUpdated struct
func (ev EventProviderUpdated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionProviderUpdated),
		}, ProviderEVAttributes(ev.Owner)...)...,
	)
}

// EventProviderDeleted struct
type EventProviderDeleted struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	Owner   sdk.AccAddress          `json:"owner"`
}

func NewEventProviderDeleted(owner sdk.AccAddress) EventProviderDeleted {
	return EventProviderDeleted{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionProviderDeleted,
		},
		Owner: owner,
	}
}

// ToSDKEvent method creates new sdk event for EventProviderDeleted struct
func (ev EventProviderDeleted) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionProviderDeleted),
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
	case evActionProviderCreated:
		owner, err := ParseEVProvider(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventProviderCreated(owner), nil
	case evActionProviderUpdated:
		owner, err := ParseEVProvider(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventProviderUpdated(owner), nil
	case evActionProviderDeleted:
		owner, err := ParseEVProvider(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventProviderDeleted(owner), nil
	default:
		return nil, sdkutil.ErrUnknownAction
	}
}
