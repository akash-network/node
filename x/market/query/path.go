package query

import (
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	dpath "github.com/ovrclk/akash/x/deployment/query"
	"github.com/ovrclk/akash/x/market/types"
)

const (
	ordersPath = "orders"
	orderPath  = "order"
	bidsPath   = "bids"
	bidPath    = "bid"
	leasesPath = "leases"
	leasePath  = "lease"
)

var (
	ErrInvalidPath = errors.New("query: invalid path")
	ErrOwnerValue  = errors.New("query: invalid owner value")
	ErrStateValue  = errors.New("query: invalid state value")
)

// getOrdersPath returns orders path for queries
func getOrdersPath(ofilters OrderFilters) string {
	return fmt.Sprintf("%s/%s/%v", ordersPath, ofilters.Owner, ofilters.StateFlagVal)
}

// OrderPath return order path of given order id for queries
func OrderPath(id types.OrderID) string {
	return fmt.Sprintf("%s/%s", orderPath, orderParts(id))
}

//getBidsPath returns bids path for queries
func getBidsPath(bfilters BidFilters) string {
	return fmt.Sprintf("%s/%s/%v", bidsPath, bfilters.Owner, bfilters.StateFlagVal)
}

// getBidPath return bid path of given bid id for queries
func getBidPath(id types.BidID) string {
	return fmt.Sprintf("%s/%s/%s", bidPath, orderParts(id.OrderID()), id.Provider)
}

// getLeasesPath returns leases path for queries
func getLeasesPath(lfilters LeaseFilters) string {
	return fmt.Sprintf("%s/%s/%v", leasesPath, lfilters.Owner, lfilters.StateFlagVal)
}

// LeasePath return lease path of given lease id for queries
func LeasePath(id types.LeaseID) string {
	return fmt.Sprintf("%s/%s/%s", leasePath, orderParts(id.OrderID()), id.Provider)
}

func orderParts(id types.OrderID) string {
	return fmt.Sprintf("%s/%v/%v/%v", id.Owner, id.DSeq, id.GSeq, id.OSeq)
}

// parseOrderPath returns orderID details with provided queries, and return
// error if occurred due to wrong query
func parseOrderPath(parts []string) (types.OrderID, error) {
	if len(parts) < 4 {
		return types.OrderID{}, ErrInvalidPath
	}

	did, err := dpath.ParseGroupPath(parts[0:3])
	if err != nil {
		return types.OrderID{}, err
	}

	oseq, err := strconv.ParseUint(parts[3], 10, 32)
	if err != nil {
		return types.OrderID{}, err
	}

	return types.MakeOrderID(did, uint32(oseq)), nil
}

// parseOrderFiltersPath returns OrderFilters details with provided queries, and return
// error if occurred due to wrong query
func parseOrderFiltersPath(parts []string) (OrderFilters, bool, error) {
	if len(parts) < 2 {
		return OrderFilters{}, false, ErrInvalidPath
	}

	owner, err := sdk.AccAddressFromBech32(parts[0])
	if err != nil {
		return OrderFilters{}, false, err
	}

	if !owner.Empty() && sdk.VerifyAddressFormat(owner) != nil {
		return OrderFilters{}, false, ErrOwnerValue
	}

	state, ok := types.Order_State_value[parts[1]]

	if !ok && (parts[1] != "") {
		return OrderFilters{}, false, ErrStateValue
	}

	return OrderFilters{
		Owner:        owner,
		StateFlagVal: parts[1],
		State:        types.Order_State(state),
	}, ok, nil
}

// parseBidPath returns bidID details with provided queries, and return
// error if occurred due to wrong query
func parseBidPath(parts []string) (types.BidID, error) {
	if len(parts) < 5 {
		return types.BidID{}, ErrInvalidPath
	}

	oid, err := parseOrderPath(parts[0:4])
	if err != nil {
		return types.BidID{}, err
	}

	provider, err := sdk.AccAddressFromBech32(parts[4])
	if err != nil {
		return types.BidID{}, err
	}

	return types.MakeBidID(oid, provider), nil
}

// parseBidFiltersPath returns BidFilters details with provided queries, and return
// error if occurred due to wrong query
func parseBidFiltersPath(parts []string) (BidFilters, bool, error) {
	if len(parts) < 2 {
		return BidFilters{}, false, ErrInvalidPath
	}

	owner, err := sdk.AccAddressFromBech32(parts[0])
	if err != nil {
		return BidFilters{}, false, err
	}

	if !owner.Empty() && sdk.VerifyAddressFormat(owner) != nil {
		return BidFilters{}, false, ErrOwnerValue
	}

	state, ok := types.Bid_State_value[parts[1]]

	if !ok && (parts[1] != "") {
		return BidFilters{}, false, ErrStateValue
	}

	return BidFilters{
		Owner:        owner,
		StateFlagVal: parts[1],
		State:        types.Bid_State(state),
	}, ok, nil
}

// parseLeasePath returns leaseID details with provided queries, and return
// error if occurred due to wrong query
func ParseLeasePath(parts []string) (types.LeaseID, error) {
	bid, err := parseBidPath(parts)
	if err != nil {
		return types.LeaseID{}, err
	}

	return types.MakeLeaseID(bid), nil
}

// parseLeaseFiltersPath returns LeaseFilters details with provided queries, and return
// error if occurred due to wrong query
func parseLeaseFiltersPath(parts []string) (LeaseFilters, bool, error) {
	if len(parts) < 2 {
		return LeaseFilters{}, false, ErrInvalidPath
	}

	owner, err := sdk.AccAddressFromBech32(parts[0])
	if err != nil {
		return LeaseFilters{}, false, err
	}

	if !owner.Empty() && sdk.VerifyAddressFormat(owner) != nil {
		return LeaseFilters{}, false, ErrOwnerValue
	}

	state, ok := types.Lease_State_value[parts[1]]

	if !ok && (parts[1] != "") {
		return LeaseFilters{}, false, ErrStateValue
	}

	return LeaseFilters{
		Owner:        owner,
		StateFlagVal: parts[1],
		State:        types.Lease_State(state),
	}, ok, nil
}
