package types

import (
	"fmt"
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

	evOSeqKey        = "oseq"
	evProviderKey    = "provider"
	evPriceDenomKey  = "price-denom"
	evPriceAmountKey = "price-amount"
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
		}, orderIDEVAttributes(e.ID)...)...,
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
		}, orderIDEVAttributes(e.ID)...)...,
	)
}

// EventBidCreated struct
type EventBidCreated struct {
	ID    BidID
	Price sdk.Coin
}

// ToSDKEvent method creates new sdk event for EventBidCreated struct
func (e EventBidCreated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append(
			append([]sdk.Attribute{
				sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
				sdk.NewAttribute(sdk.AttributeKeyAction, evActionBidCreated),
			}, bidIDEVAttributes(e.ID)...),
			priceEVAttributes(e.Price)...)...,
	)
}

// EventBidClosed struct
type EventBidClosed struct {
	ID    BidID
	Price sdk.Coin
}

// ToSDKEvent method creates new sdk event for EventBidClosed struct
func (e EventBidClosed) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append(
			append([]sdk.Attribute{
				sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
				sdk.NewAttribute(sdk.AttributeKeyAction, evActionBidClosed),
			}, bidIDEVAttributes(e.ID)...),
			priceEVAttributes(e.Price)...)...,
	)
}

// EventLeaseCreated struct
type EventLeaseCreated struct {
	ID    LeaseID
	Price sdk.Coin
}

// ToSDKEvent method creates new sdk event for EventLeaseCreated struct
func (e EventLeaseCreated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append(
			append([]sdk.Attribute{
				sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
				sdk.NewAttribute(sdk.AttributeKeyAction, evActionLeaseCreated),
			}, leaseIDEVAttributes(e.ID)...),
			priceEVAttributes(e.Price)...)...)
}

// EventLeaseClosed struct
type EventLeaseClosed struct {
	ID    LeaseID
	Price sdk.Coin
}

// ToSDKEvent method creates new sdk event for EventLeaseClosed struct
func (e EventLeaseClosed) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdk.EventTypeMessage,
		append(
			append([]sdk.Attribute{
				sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
				sdk.NewAttribute(sdk.AttributeKeyAction, evActionLeaseClosed),
			}, leaseIDEVAttributes(e.ID)...),
			priceEVAttributes(e.Price)...)...)
}

// orderIDEVAttributes returns event attribues for given orderID
func orderIDEVAttributes(id OrderID) []sdk.Attribute {
	return append(dtypes.GroupIDEVAttributes(id.GroupID()),
		sdk.NewAttribute(evOSeqKey, strconv.FormatUint(uint64(id.OSeq), 10)))
}

// parseEVOrderID returns orderID for given event attributes
func parseEVOrderID(attrs []sdk.Attribute) (OrderID, error) {
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

// bidIDEVAttributes returns event attribues for given bidID
func bidIDEVAttributes(id BidID) []sdk.Attribute {
	return append(orderIDEVAttributes(id.OrderID()),
		sdk.NewAttribute(evProviderKey, id.Provider.String()))
}

// parseEVBidID returns bidID for given event attributes
func parseEVBidID(attrs []sdk.Attribute) (BidID, error) {
	oid, err := parseEVOrderID(attrs)
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

// leaseIDEVAttributes returns event attribues for given LeaseID
func leaseIDEVAttributes(id LeaseID) []sdk.Attribute {
	return append(orderIDEVAttributes(id.OrderID()),
		sdk.NewAttribute(evProviderKey, id.Provider.String()))
}

// parseEVLeaseID returns leaseID for given event attributes
func parseEVLeaseID(attrs []sdk.Attribute) (LeaseID, error) {
	bid, err := parseEVBidID(attrs)
	if err != nil {
		return LeaseID{}, err
	}
	return LeaseID(bid), nil
}

func priceEVAttributes(price sdk.Coin) []sdk.Attribute {
	return []sdk.Attribute{
		sdk.NewAttribute(evPriceDenomKey, price.Denom),
		sdk.NewAttribute(evPriceAmountKey, price.Amount.String()),
	}
}

func parseEVPriceAttributes(attrs []sdk.Attribute) (sdk.Coin, error) {
	denom, err := sdkutil.GetString(attrs, evPriceDenomKey)
	if err != nil {
		return sdk.Coin{}, err
	}

	amounts, err := sdkutil.GetString(attrs, evPriceAmountKey)
	if err != nil {
		return sdk.Coin{}, err
	}

	amount, ok := sdk.NewIntFromString(amounts)
	if !ok {
		return sdk.Coin{}, fmt.Errorf("error parsing price")
	}

	return sdk.NewCoin(denom, amount), nil
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
		id, err := parseEVOrderID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return EventOrderCreated{ID: id}, nil
	case evActionOrderClosed:
		id, err := parseEVOrderID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return EventOrderClosed{ID: id}, nil

	case evActionBidCreated:
		id, err := parseEVBidID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		price, err := parseEVPriceAttributes(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return EventBidCreated{ID: id, Price: price}, nil
	case evActionBidClosed:
		id, err := parseEVBidID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		// optional price
		price, _ := parseEVPriceAttributes(ev.Attributes)
		return EventBidClosed{ID: id, Price: price}, nil

	case evActionLeaseCreated:
		id, err := parseEVLeaseID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		price, err := parseEVPriceAttributes(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return EventLeaseCreated{ID: id, Price: price}, nil
	case evActionLeaseClosed:
		id, err := parseEVLeaseID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		// optional price
		price, _ := parseEVPriceAttributes(ev.Attributes)
		return EventLeaseClosed{ID: id, Price: price}, nil

	default:
		return nil, sdkutil.ErrUnknownAction
	}
}
