package query

import (
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	dpath "github.com/ovrclk/akash/x/deployment/query"
	"github.com/ovrclk/akash/x/market/types"
)

const (
	ordersPath       = "orders"
	filterordersPath = "filter_orders"
	orderPath        = "order"
	bidsPath         = "bids"
	filterbidsPath   = "filter_bids"
	bidPath          = "bid"
	leasesPath       = "leases"
	filterleasesPath = "filter_leases"
	leasePath        = "lease"
)

// getOrdersPath returns orders path for queries
func getOrdersPath() string {
	return ordersPath
}

// getOrdersFilterPath returns orders path for queries with filter
func getOrdersFilterPath(id types.OrderFilters) string {
	return fmt.Sprintf("%s/%s/%v", filterordersPath, id.Owner, id.State)
}

// OrderPath return order path of given order id for queries
func OrderPath(id types.OrderID) string {
	return fmt.Sprintf("%s/%s", orderPath, orderParts(id))
}

//getBidsPath returns bids path for queries
func getBidsPath() string {
	return bidsPath
}

//getBidsFilterPath returns bids path for queries with filter
func getBidsFilterPath(id types.BidFilters) string {
	return fmt.Sprintf("%s/%s/%v", filterbidsPath, id.Owner, id.State)
}

// getBidPath return bid path of given bid id for queries
func getBidPath(id types.BidID) string {
	return fmt.Sprintf("%s/%s/%s", bidPath, orderParts(id.OrderID()), id.Provider)
}

// getLeasesPath returns leases path for queries
func getLeasesPath() string {
	return leasesPath
}

// getLeasesFilterPath returns leases path for queries with filter
func getLeasesFilterPath(id types.LeaseFilters) string {
	return fmt.Sprintf("%s/%s/%v", filterleasesPath, id.Owner, id.State)
}

// LeasePath return lease path of given lease id for queries
func LeasePath(id types.LeaseID) string {
	return fmt.Sprintf("%s/%s/%s", leasePath, orderParts(id.OrderID()), id.Provider)
}

func orderParts(id types.OrderID) string {
	return fmt.Sprintf("%s/%v/%v/%v", id.Owner, id.DSeq, id.GSeq, id.OSeq)
}

// parseOrderPath returns orderID details with provided queries, and return
// error if occured due to wrong query
func parseOrderPath(parts []string) (types.OrderID, error) {
	if len(parts) < 4 {
		return types.OrderID{}, fmt.Errorf("invalid path")
	}

	did, err := dpath.ParseGroupPath(parts[0:3])
	if err != nil {
		return types.OrderID{}, err
	}

	oseq, err := strconv.ParseUint(parts[3], 10, 32)

	return types.MakeOrderID(did, uint32(oseq)), nil
}

// parseOrderFiltersPath returns OrderFilters details with provided queries, and return
// error if occured due to wrong query
func parseOrderFiltersPath(parts []string) (types.OrderFilters, error) {
	if len(parts) < 2 {
		return types.OrderFilters{}, fmt.Errorf("invalid path")
	}

	prev, err := dpath.ParseDepFiltersPath(parts[0:2])
	if err != nil {
		return types.OrderFilters{}, err
	}

	return types.OrderFilters{
		Owner: prev.Owner,
		State: types.OrderState(prev.State),
	}, nil
}

// parseBidPath returns bidID details with provided queries, and return
// error if occured due to wrong query
func parseBidPath(parts []string) (types.BidID, error) {
	if len(parts) < 5 {
		return types.BidID{}, fmt.Errorf("invalid path")
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
// error if occured due to wrong query
func parseBidFiltersPath(parts []string) (types.BidFilters, error) {
	if len(parts) < 2 {
		return types.BidFilters{}, fmt.Errorf("invalid path")
	}

	prev, err := parseOrderFiltersPath(parts[0:2])
	if err != nil {
		return types.BidFilters{}, err
	}

	return types.BidFilters{
		Owner: prev.Owner,
		State: types.BidState(prev.State),
	}, nil
}

// parseLeasePath returns leaseID details with provided queries, and return
// error if occured due to wrong query
func parseLeasePath(parts []string) (types.LeaseID, error) {
	bid, err := parseBidPath(parts)
	if err != nil {
		return types.LeaseID{}, err
	}

	return types.MakeLeaseID(bid), nil
}

// parseLeaseFiltersPath returns LeaseFilters details with provided queries, and return
// error if occured due to wrong query
func parseLeaseFiltersPath(parts []string) (types.LeaseFilters, error) {
	if len(parts) < 2 {
		return types.LeaseFilters{}, fmt.Errorf("invalid path")
	}

	prev, err := parseOrderFiltersPath(parts[0:2])
	if err != nil {
		return types.LeaseFilters{}, err
	}

	return types.LeaseFilters{
		Owner: prev.Owner,
		State: types.LeaseState(prev.State),
	}, nil
}
