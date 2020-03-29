package query

import (
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
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

// getOrdersPath returns orders path for queries
func getOrdersPath() string {
	return ordersPath
}

// OrderPath return order path of given order id for queries
func OrderPath(id types.OrderID) string {
	return fmt.Sprintf("%s/%s", orderPath, orderParts(id))
}

//getBidsPath returns bids path for queries
func getBidsPath() string {
	return bidsPath
}

// getBidPath return bid path of given bid id for queries
func getBidPath(id types.BidID) string {
	return fmt.Sprintf("%s/%s/%s", bidPath, orderParts(id.OrderID()), id.Provider)
}

// getLeasesPath returns leases path for queries
func getLeasesPath() string {
	return leasesPath
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

// parseLeasePath returns leaseID details with provided queries, and return
// error if occured due to wrong query
func parseLeasePath(parts []string) (types.LeaseID, error) {
	bid, err := parseBidPath(parts)
	if err != nil {
		return types.LeaseID{}, err
	}

	return types.MakeLeaseID(bid), nil
}
