package types

import (
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/sdkutil"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

const (
	evActionOrderCreated = "order-created"
	evActionOrderClosed  = "order-closed"
	evActionBidCreated   = "bid-created"
	evActionBidClosed    = "bid-closed"
	evActionLeaseCreated = "lease-created"
	evActionLeaseClosed  = "lease-closed"

	evOSeqKey     = "oseq"
	evProviderKey = "provider"
)

type EventOrderCreated struct {
	ID OrderID
}

func (e EventOrderCreated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionOrderCreated),
		}, OrderIDEVAttributes(e.ID)...)...,
	)
}

type EventOrderClosed struct {
	ID OrderID
}

func (e EventOrderClosed) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionOrderClosed),
		}, OrderIDEVAttributes(e.ID)...)...,
	)
}

type EventBidCreated struct {
	ID BidID
}

func (e EventBidCreated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionBidCreated),
		}, BidIDEVAttributes(e.ID)...)...,
	)
}

type EventBidClosed struct {
	ID BidID
}

func (e EventBidClosed) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionBidClosed),
		}, BidIDEVAttributes(e.ID)...)...,
	)
}

type EventLeaseCreated struct {
	ID LeaseID
}

func (e EventLeaseCreated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionLeaseCreated),
		}, LeaseIDEVAttributes(e.ID)...)...,
	)
}

type EventLeaseClosed struct {
	ID LeaseID
}

func (e EventLeaseClosed) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionLeaseClosed),
		}, LeaseIDEVAttributes(e.ID)...)...,
	)
}

func OrderIDEVAttributes(id OrderID) []sdk.Attribute {
	return append(dtypes.GroupIDEVAttributes(id.GroupID()),
		sdk.NewAttribute(evOSeqKey, strconv.FormatUint(uint64(id.OSeq), 10)))
}

func ParseEVOrderID(attrs []sdk.Attribute) (OrderID, error) {
	gid, err := dtypes.ParseEVGroupID(attrs)
	if err != nil {
		return OrderID{}, err
	}
	oseq, err := sdkutil.GetUint64(attrs, evOSeqKey)
	if err != nil {
		return OrderID{}, err
	}

	return OrderID{
		Owner: gid.Owner,
		DSeq:  gid.DSeq,
		GSeq:  gid.GSeq,
		OSeq:  uint32(oseq),
	}, nil

}

func BidIDEVAttributes(id BidID) []sdk.Attribute {
	return append(OrderIDEVAttributes(id.OrderID()),
		sdk.NewAttribute(evProviderKey, id.Provider.String()))
}

func ParseEVBidID(attrs []sdk.Attribute) (BidID, error) {
	oid, err := ParseEVOrderID(attrs)
	if err != nil {
		return BidID{}, err
	}

	provider, err := sdkutil.GetAccAddress(attrs, evProviderKey)
	if err != nil {
		return BidID{}, err
	}

	return BidID{
		Owner:    oid.Owner,
		DSeq:     oid.DSeq,
		GSeq:     oid.GSeq,
		OSeq:     oid.OSeq,
		Provider: provider,
	}, nil
}

func LeaseIDEVAttributes(id LeaseID) []sdk.Attribute {
	return append(OrderIDEVAttributes(id.OrderID()),
		sdk.NewAttribute(evProviderKey, id.Provider.String()))
}

func ParseEVLeaseID(attrs []sdk.Attribute) (LeaseID, error) {
	bid, err := ParseEVBidID(attrs)
	if err != nil {
		return LeaseID{}, err
	}
	return LeaseID(bid), nil
}

func ParseEvent(ev sdkutil.Event) (interface{}, error) {
	if ev.Type != sdk.EventTypeMessage {
		return nil, sdkutil.ErrUnknownType
	}
	if ev.Module != ModuleName {
		return nil, sdkutil.ErrUnknownModule
	}
	switch ev.Action {

	case evActionOrderCreated:
		id, err := ParseEVOrderID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return EventOrderCreated{ID: id}, nil
	case evActionOrderClosed:
		id, err := ParseEVOrderID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return EventOrderClosed{ID: id}, nil

	case evActionBidCreated:
		id, err := ParseEVBidID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return EventBidCreated{ID: id}, nil
	case evActionBidClosed:
		id, err := ParseEVBidID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return EventBidClosed{ID: id}, nil

	case evActionLeaseCreated:
		id, err := ParseEVLeaseID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return EventLeaseCreated{ID: id}, nil
	case evActionLeaseClosed:
		id, err := ParseEVLeaseID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return EventLeaseClosed{ID: id}, nil

	default:
		return nil, sdkutil.ErrUnknownAction
	}
}
