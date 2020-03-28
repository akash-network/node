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

// OrdersPath returns orders path for queries
func OrdersPath() string {
	return ordersPath
}

// OrderPath return order path of given order id for queries
func OrderPath(id types.OrderID) string {
	return fmt.Sprintf("%s/%s", orderPath, orderParts(id))
}

//BidsPath returns bids path for queries
func BidsPath() string {
	return bidsPath
}

// BidPath return bid path of given bid id for queries
func BidPath(id types.BidID) string {
	return fmt.Sprintf("%s/%s/%s", bidPath, orderParts(id.OrderID()), id.Provider)
}

// LeasesPath returns leases path for queries
func LeasesPath() string {
	return leasesPath
}

// LeasePath return lease path of given lease id for queries
func LeasePath(id types.LeaseID) string {
	return fmt.Sprintf("%s/%s/%s", leasePath, orderParts(id.OrderID()), id.Provider)
}

func orderParts(id types.OrderID) string {
	return fmt.Sprintf("%s/%v/%v/%v", id.Owner, id.DSeq, id.GSeq, id.OSeq)
}

// ParseOrderPath returns orderID details with provided queries, and return
// error if occured due to wrong query
func ParseOrderPath(parts []string) (types.OrderID, error) {
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

// ParseBidPath returns bidID details with provided queries, and return
// error if occured due to wrong query
func ParseBidPath(parts []string) (types.BidID, error) {
	if len(parts) < 5 {
		return types.BidID{}, fmt.Errorf("invalid path")
	}

	oid, err := ParseOrderPath(parts[0:4])
	if err != nil {
		return types.BidID{}, err
	}

	provider, err := sdk.AccAddressFromBech32(parts[4])
	if err != nil {
		return types.BidID{}, err
	}

	return types.MakeBidID(oid, provider), nil
}

// ParseLeasePath returns leaseID details with provided queries, and return
// error if occured due to wrong query
func ParseLeasePath(parts []string) (types.LeaseID, error) {
	bid, err := ParseBidPath(parts)
	if err != nil {
		return types.LeaseID{}, err
	}

	return types.MakeLeaseID(bid), nil
}
