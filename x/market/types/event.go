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

// EventOrderCreated struct
type EventOrderCreated struct {
	ID OrderID
}

// ToSDKEvent method creates new sdk event for EventOrderCreated struct
func (e EventOrderCreated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionOrderCreated),
		}, OrderIDEVAttributes(e.ID)...)...,
	)
}

// EventOrderClosed struct
type EventOrderClosed struct {
	ID OrderID
}

// ToSDKEvent method creates new sdk event for EventOrderClosed struct
func (e EventOrderClosed) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionOrderClosed),
		}, OrderIDEVAttributes(e.ID)...)...,
	)
}

// EventBidCreated struct
type EventBidCreated struct {
	ID BidID
}

// ToSDKEvent method creates new sdk event for EventBidCreated struct
func (e EventBidCreated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionBidCreated),
		}, BidIDEVAttributes(e.ID)...)...,
	)
}

// EventBidClosed struct
type EventBidClosed struct {
	ID BidID
}

// ToSDKEvent method creates new sdk event for EventBidClosed struct
func (e EventBidClosed) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionBidClosed),
		}, BidIDEVAttributes(e.ID)...)...,
	)
}

// EventLeaseCreated struct
type EventLeaseCreated struct {
	ID LeaseID
}

// ToSDKEvent method creates new sdk event for EventLeaseCreated struct
func (e EventLeaseCreated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionLeaseCreated),
		}, LeaseIDEVAttributes(e.ID)...)...,
	)
}

// EventLeaseClosed struct
type EventLeaseClosed struct {
	ID LeaseID
}

// ToSDKEvent method creates new sdk event for EventLeaseClosed struct
func (e EventLeaseClosed) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionLeaseClosed),
		}, LeaseIDEVAttributes(e.ID)...)...,
	)
}

// OrderIDEVAttributes returns event attribues for given orderID
func OrderIDEVAttributes(id OrderID) []sdk.Attribute {
	return append(dtypes.GroupIDEVAttributes(id.GroupID()),
		sdk.NewAttribute(evOSeqKey, strconv.FormatUint(uint64(id.OSeq), 10)))
}

// ParseEVOrderID returns orderID for given event attributes
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

// BidIDEVAttributes returns event attribues for given bidID
func BidIDEVAttributes(id BidID) []sdk.Attribute {
	return append(OrderIDEVAttributes(id.OrderID()),
		sdk.NewAttribute(evProviderKey, id.Provider.String()))
}

// ParseEVBidID returns bidID for given event attributes
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

// LeaseIDEVAttributes returns event attribues for given LeaseID
func LeaseIDEVAttributes(id LeaseID) []sdk.Attribute {
	return append(OrderIDEVAttributes(id.OrderID()),
		sdk.NewAttribute(evProviderKey, id.Provider.String()))
}

// ParseEVLeaseID returns leaseID for given event attributes
func ParseEVLeaseID(attrs []sdk.Attribute) (LeaseID, error) {
	bid, err := ParseEVBidID(attrs)
	if err != nil {
		return LeaseID{}, err
	}
	return LeaseID(bid), nil
}

// ParseEvent parses event and returns details of event and error if occured
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
