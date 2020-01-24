package types

import (
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/sdkutil"
)

const (
	evActionDeploymentCreate = "deployment-create"
	evActionDeploymentUpdate = "deployment-update"
	evActionDeploymentClose  = "deployment-close"
	evOwnerKey               = "owner"
	evDSeqKey                = "dseq"
	evGSeqKey                = "gseq"
)

type EventDeploymentCreate struct {
	ID DeploymentID
}

func (ev EventDeploymentCreate) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionDeploymentCreate),
		}, DeploymentIDEVAttributes(ev.ID)...)...,
	)
}

type EventDeploymentUpdate struct {
	ID      DeploymentID
	Version []byte // TODO
}

func (ev EventDeploymentUpdate) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionDeploymentUpdate),
		}, DeploymentIDEVAttributes(ev.ID)...)...,
	)
}

type EventDeploymentClose struct {
	ID DeploymentID
}

func (ev EventDeploymentClose) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionDeploymentClose),
		}, DeploymentIDEVAttributes(ev.ID)...)...,
	)
}

func DeploymentIDEVAttributes(id DeploymentID) []sdk.Attribute {
	return []sdk.Attribute{
		sdk.NewAttribute(evOwnerKey, id.Owner.String()),
		sdk.NewAttribute(evDSeqKey, strconv.FormatUint(id.DSeq, 10)),
	}
}

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

func GroupIDEVAttributes(id GroupID) []sdk.Attribute {
	return append(DeploymentIDEVAttributes(id.DeploymentID()),
		sdk.NewAttribute(evGSeqKey, strconv.FormatUint(uint64(id.GSeq), 10)))
}

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

func ParseEvent(ev sdkutil.Event) (interface{}, error) {
	if ev.Type != sdk.EventTypeMessage {
		return nil, sdkutil.ErrUnknownType
	}
	if ev.Module != ModuleName {
		return nil, sdkutil.ErrUnknownModule
	}
	switch ev.Action {
	case evActionDeploymentCreate:
		did, err := ParseEVDeploymentID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return EventDeploymentCreate{ID: did}, nil
	case evActionDeploymentUpdate:
		did, err := ParseEVDeploymentID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return EventDeploymentUpdate{ID: did}, nil
	default:
		return nil, sdkutil.ErrUnknownAction
	}
}
