package types

import (
	"encoding/hex"
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
	evVersionKey              = "version"
	encodedVersionHexLen      = 64
)

// EventDeploymentCreated struct
type EventDeploymentCreated struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	ID      DeploymentID            `json:"id"`
	Version []byte                  `json:"version"`
}

// NewEventDeploymentCreated initializes creation event.
func NewEventDeploymentCreated(id DeploymentID, version []byte) EventDeploymentCreated {
	return EventDeploymentCreated{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionDeploymentCreated,
		},
		ID:      id,
		Version: version,
	}
}

// ToSDKEvent method creates new sdk event for EventDeploymentCreated struct
func (ev EventDeploymentCreated) ToSDKEvent() sdk.Event {
	version := encodeHex(ev.Version)
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionDeploymentCreated),
			sdk.NewAttribute(evVersionKey, string(version)),
		}, DeploymentIDEVAttributes(ev.ID)...)...,
	)
}

// EventDeploymentUpdated struct
type EventDeploymentUpdated struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	ID      DeploymentID            `json:"id"`
	Version []byte                  `json:"version"`
}

// NewEventDeploymentUpdated initializes SDK type
func NewEventDeploymentUpdated(id DeploymentID, version []byte) EventDeploymentUpdated {
	return EventDeploymentUpdated{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionDeploymentUpdated,
		},
		ID:      id,
		Version: version,
	}
}

// ToSDKEvent method creates new sdk event for EventDeploymentUpdated struct
func (ev EventDeploymentUpdated) ToSDKEvent() sdk.Event {
	version := encodeHex(ev.Version)
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionDeploymentUpdated),
			sdk.NewAttribute(evVersionKey, string(version)),
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
		sdk.NewAttribute(evOwnerKey, id.Owner),
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
		Owner: owner.String(),
		DSeq:  dseq,
	}, nil
}

// ParseEVDeploymentVersion returns the Deployment's SDL sha256 sum
func ParseEVDeploymentVersion(attrs []sdk.Attribute) ([]byte, error) {
	v, err := sdkutil.GetString(attrs, evVersionKey)
	if err != nil {
		return nil, err
	}
	return decodeHex([]byte(v))
}

func encodeHex(src []byte) []byte {
	dst := make([]byte, hex.EncodedLen(len(src)))
	hex.Encode(dst, src)
	return dst
}

func decodeHex(src []byte) ([]byte, error) {
	dst := make([]byte, hex.DecodedLen(len(src)))
	if _, err := hex.Decode(dst, src); err != nil {
		return []byte{}, err
	}
	return dst, nil
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
		ver, err := ParseEVDeploymentVersion(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventDeploymentCreated(did, ver), nil
	case evActionDeploymentUpdated:
		did, err := ParseEVDeploymentID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		ver, err := ParseEVDeploymentVersion(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventDeploymentUpdated(did, ver), nil
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
