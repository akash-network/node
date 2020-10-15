package types

import (
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

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

var (
	ErrParsingPrice = errors.New("error parsing price")
)

// EventOrderCreated struct
type EventOrderCreated struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	ID      OrderID                 `json:"id"`
}

func NewEventOrderCreated(id OrderID) EventOrderCreated {
	return EventOrderCreated{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionOrderCreated,
		},
		ID: id,
	}
}

// ToSDKEvent method creates new sdk event for EventOrderCreated struct
func (e EventOrderCreated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionOrderCreated),
		}, orderIDEVAttributes(e.ID)...)...,
	)
}

// EventOrderClosed struct
type EventOrderClosed struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	ID      OrderID                 `json:"id"`
}

func NewEventOrderClosed(id OrderID) EventOrderClosed {
	return EventOrderClosed{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionOrderClosed,
		},
		ID: id,
	}
}

// ToSDKEvent method creates new sdk event for EventOrderClosed struct
func (e EventOrderClosed) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append([]sdk.Attribute{
			sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
			sdk.NewAttribute(sdk.AttributeKeyAction, evActionOrderClosed),
		}, orderIDEVAttributes(e.ID)...)...,
	)
}

// EventBidCreated struct
type EventBidCreated struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	ID      BidID                   `json:"id"`
	Price   sdk.Coin                `json:"price"`
}

func NewEventBidCreated(id BidID, price sdk.Coin) EventBidCreated {
	return EventBidCreated{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionBidCreated,
		},
		ID:    id,
		Price: price,
	}
}

// ToSDKEvent method creates new sdk event for EventBidCreated struct
func (e EventBidCreated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
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
	Context sdkutil.BaseModuleEvent `json:"context"`
	ID      BidID                   `json:"id"`
	Price   sdk.Coin                `json:"price"`
}

func NewEventBidClosed(id BidID, price sdk.Coin) EventBidClosed {
	return EventBidClosed{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionBidClosed,
		},
		ID:    id,
		Price: price,
	}
}

// ToSDKEvent method creates new sdk event for EventBidClosed struct
func (e EventBidClosed) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
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
	Context sdkutil.BaseModuleEvent `json:"context"`
	ID      LeaseID                 `json:"id"`
	Price   sdk.Coin                `json:"price"`
}

func NewEventLeaseCreated(id LeaseID, price sdk.Coin) EventLeaseCreated {
	return EventLeaseCreated{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionLeaseCreated,
		},
		ID:    id,
		Price: price,
	}
}

// ToSDKEvent method creates new sdk event for EventLeaseCreated struct
func (e EventLeaseCreated) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
		append(
			append([]sdk.Attribute{
				sdk.NewAttribute(sdk.AttributeKeyModule, ModuleName),
				sdk.NewAttribute(sdk.AttributeKeyAction, evActionLeaseCreated),
			}, leaseIDEVAttributes(e.ID)...),
			priceEVAttributes(e.Price)...)...)
}

// EventLeaseClosed struct
type EventLeaseClosed struct {
	Context sdkutil.BaseModuleEvent `json:"context"`
	ID      LeaseID                 `json:"id"`
	Price   sdk.Coin                `json:"price"`
}

func NewEventLeaseClosed(id LeaseID, price sdk.Coin) EventLeaseClosed {
	return EventLeaseClosed{
		Context: sdkutil.BaseModuleEvent{
			Module: ModuleName,
			Action: evActionLeaseClosed,
		},
		ID:    id,
		Price: price,
	}
}

// ToSDKEvent method creates new sdk event for EventLeaseClosed struct
func (e EventLeaseClosed) ToSDKEvent() sdk.Event {
	return sdk.NewEvent(sdkutil.EventTypeMessage,
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
		sdk.NewAttribute(evProviderKey, id.Provider))
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
		Provider: provider.String(),
	}, nil
}

// leaseIDEVAttributes returns event attribues for given LeaseID
func leaseIDEVAttributes(id LeaseID) []sdk.Attribute {
	return append(orderIDEVAttributes(id.OrderID()),
		sdk.NewAttribute(evProviderKey, id.Provider))
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
		return sdk.Coin{}, ErrParsingPrice
	}

	return sdk.NewCoin(denom, amount), nil
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

	case evActionOrderCreated:
		id, err := parseEVOrderID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventOrderCreated(id), nil
	case evActionOrderClosed:
		id, err := parseEVOrderID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventOrderClosed(id), nil

	case evActionBidCreated:
		id, err := parseEVBidID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		price, err := parseEVPriceAttributes(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventBidCreated(id, price), nil
	case evActionBidClosed:
		id, err := parseEVBidID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		// optional price
		price, _ := parseEVPriceAttributes(ev.Attributes)
		return NewEventBidClosed(id, price), nil

	case evActionLeaseCreated:
		id, err := parseEVLeaseID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		price, err := parseEVPriceAttributes(ev.Attributes)
		if err != nil {
			return nil, err
		}
		return NewEventLeaseCreated(id, price), nil
	case evActionLeaseClosed:
		id, err := parseEVLeaseID(ev.Attributes)
		if err != nil {
			return nil, err
		}
		// optional price
		price, _ := parseEVPriceAttributes(ev.Attributes)
		return NewEventLeaseClosed(id, price), nil

	default:
		return nil, sdkutil.ErrUnknownAction
	}
}
