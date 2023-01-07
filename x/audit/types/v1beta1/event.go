package v1beta1

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/akash-network/node/sdkutil"
)

const (
	evActionTrustedAuditorCreated = "audit-trusted-auditor-created"
	evActionTrustedAuditorDeleted = "audit-trusted-auditor-deleted"
	evOwnerKey                    = "owner"
	evAuditorKey                  = "auditor"
)

// EventTrustedAuditorCreated struct
type EventTrustedAuditorCreated struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	Owner   sdk.Address             `json:"owner"`
	Auditor sdk.Address             `json:"auditor"`
}

func NewEventTrustedAuditorCreated(owner sdk.Address, auditor sdk.Address) EventTrustedAuditorCreated {
	return EventTrustedAuditorCreated{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionTrustedAuditorCreated,
		},
		Owner:   owner,
		Auditor: auditor,
	}
}

// ToSDKEvent method creates new sdk event for EventProviderCreated struct
func (ev EventTrustedAuditorCreated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionTrustedAuditorCreated),
		}, TrustedAuditorEVAttributes(ev.Owner, ev.Auditor)...)...,
	)
}

// TrustedAuditorEVAttributes returns event attributes for given Provider
func TrustedAuditorEVAttributes(owner sdk.Address, auditor sdk.Address) []sdk.Attribute {
	return []sdk.Attribute{
		sdk.NewAttribute(evOwnerKey, owner.String()),
		sdk.NewAttribute(evAuditorKey, auditor.String()),
	}
}

// ParseEVTTrustedAuditor returns provider details for given event attributes
func ParseEVTTrustedAuditor(attrs []sdk.Attribute) (sdk.Address, sdk.Address, error) {
	owner, err := sdkutil.GetAccAddress(attrs, evOwnerKey)
	if err != nil {
		return nil, nil, err
	}

	auditor, err := sdkutil.GetAccAddress(attrs, evAuditorKey)
	if err != nil {
		return nil, nil, err
	}

	return owner, auditor, nil
}

type EventTrustedAuditorDeleted struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	Owner   sdk.Address             `json:"owner"`
	Auditor sdk.Address             `json:"auditor"`
}

func NewEventTrustedAuditorDeleted(owner sdk.Address, auditor sdk.Address) EventTrustedAuditorDeleted {
	return EventTrustedAuditorDeleted{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionTrustedAuditorDeleted,
		},
		Owner:   owner,
		Auditor: auditor,
	}
}

// ToSDKEvent method creates new sdk event for EventProviderCreated struct
func (ev EventTrustedAuditorDeleted) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionTrustedAuditorDeleted),
		}, TrustedAuditorEVAttributes(ev.Owner, ev.Auditor)...)...,
	)
}

// ParseEvent parses event and returns details of event and error if occurred
func ParseEvent(ev sdkutil.Event) (sdkutil.ModuleEvent, error) {
	if ev.Type != sdkutil.EventTypeMessage {
		return nil, sdkutil.ErrUnknownType
	}
	if ev.Module != ModuleName {
		return nil, sdkutil.ErrUnknownModule
	}
	switch ev.Action {
	case evActionTrustedAuditorCreated:
		owner, auditor, err := ParseEVTTrustedAuditor(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventTrustedAuditorCreated(owner, auditor), nil
	case evActionTrustedAuditorDeleted:
		owner, auditor, err := ParseEVTTrustedAuditor(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventTrustedAuditorDeleted(owner, auditor), nil
	default:
		return nil, sdkutil.ErrUnknownAction
	}
}
