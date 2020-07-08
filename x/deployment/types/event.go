package types

import (
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/sdkutil"
)

const (
	evActionDeploymentCreated = "deployment-created"
	evActionDeploymentUpdated = "deployment-updated"
	evActionDeploymentClosed  = "deployment-closed"
	evActionGroupClosed       = "group-closed"
	evOwnerKey                = "owner"
	evDSeqKey                 = "dseq"
	evGSeqKey                 = "gseq"
)

// EventDeploymentCreated struct
type EventDeploymentCreated struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	ID      DeploymentID            `json:"id"`
}

func NewEventDeploymentCreated(id DeploymentID) EventDeploymentCreated {
	return EventDeploymentCreated{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionDeploymentCreated,
		},
		ID: id,
	}
}

// ToSDKEvent method creates new sdk event for EventDeploymentCreated struct
func (ev EventDeploymentCreated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionDeploymentCreated),
		}, DeploymentIDEVAttributes(ev.ID)...)...,
	)
}

// EventDeploymentUpdated struct
type EventDeploymentUpdated struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	ID      DeploymentID            `json:"id"`
	Version []byte                  `json:"version,omitempty"` // TODO: #565
}

func NewEventDeploymentUpdated(id DeploymentID) EventDeploymentUpdated {
	return EventDeploymentUpdated{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionDeploymentUpdated,
		},
		ID: id,
	}
}

// ToSDKEvent method creates new sdk event for EventDeploymentUpdated struct
func (ev EventDeploymentUpdated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionDeploymentUpdated),
		}, DeploymentIDEVAttributes(ev.ID)...)...,
	)
}

// EventDeploymentClosed struct
type EventDeploymentClosed struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	ID      DeploymentID            `json:"id"`
}

func NewEventDeploymentClosed(id DeploymentID) EventDeploymentClosed {
	return EventDeploymentClosed{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionDeploymentClosed,
		},
		ID: id,
	}
}

// ToSDKEvent method creates new sdk event for EventDeploymentClosed struct
func (ev EventDeploymentClosed) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionDeploymentClosed),
		}, DeploymentIDEVAttributes(ev.ID)...)...,
	)
}

// DeploymentIDEVAttributes returns event attribues for given DeploymentID
func DeploymentIDEVAttributes(id DeploymentID) []sdk.Attribute {
	return []sdk.Attribute{
		sdk.NewAttribute(evOwnerKey, id.Owner.String()),
		sdk.NewAttribute(evDSeqKey, strconv.FormatUint(id.DSeq, 10)),
	}
}

// ParseEVDeploymentID returns deploymentID details for given event attributes
func ParseEVDeploymentID(attrs []sdk.Attribute) (DeploymentID, error) {
	owner, err := sdkutil.GetAccAddress(attrs, evOwnerKey)
	if err != nil {
		return DeploymentID{}, err
	}
	dseq, err := sdkutil.GetUint64(attrs, evDSeqKey)
	if err != nil {
		return DeploymentID{}, err
	}

	return DeploymentID{
		Owner: owner,
		DSeq:  dseq,
	}, nil
}

// EventGroupClosed provides SDK event to signal group termination
type EventGroupClosed struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	ID      GroupID                 `json:"id"`
}

func NewEventGroupClosed(id GroupID) EventGroupClosed {
	return EventGroupClosed{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionGroupClosed,
		},
		ID: id,
	}
}

// ToSDKEvent produces the SDK notification for Event
func (ev EventGroupClosed) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionGroupClosed),
		}, GroupIDEVAttributes(ev.ID)...)...,
	)
}

// GroupIDEVAttributes returns event attribues for given GroupID
func GroupIDEVAttributes(id GroupID) []sdk.Attribute {
	return append(DeploymentIDEVAttributes(id.DeploymentID()),
		sdk.NewAttribute(evGSeqKey, strconv.FormatUint(uint64(id.GSeq), 10)))
}

// ParseEVGroupID returns GroupID details for given event attributes
func ParseEVGroupID(attrs []sdk.Attribute) (GroupID, error) {
	did, err := ParseEVDeploymentID(attrs)
	if err != nil {
		return GroupID{}, err
	}

	gseq, err := sdkutil.GetUint64(attrs, evGSeqKey)
	if err != nil {
		return GroupID{}, err
	}

	return GroupID{
		Owner: did.Owner,
		DSeq:  did.DSeq,
		GSeq:  uint32(gseq),
	}, nil
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
	case evActionDeploymentCreated:
		did, err := ParseEVDeploymentID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventDeploymentCreated(did), nil
	case evActionDeploymentUpdated:
		did, err := ParseEVDeploymentID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventDeploymentUpdated(did), nil
	case evActionDeploymentClosed:
		did, err := ParseEVDeploymentID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventDeploymentClosed(did), nil
	case evActionGroupClosed:
		gid, err := ParseEVGroupID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventGroupClosed(gid), nil
	default:
		return nil, sdkutil.ErrUnknownAction
	}
}
