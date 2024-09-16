package query

import (
	"errors"
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	v1 "pkg.akt.dev/go/node/market/v1"

	dpath "pkg.akt.dev/node/x/deployment/query"
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
func OrderPath(id v1.OrderID) string {
	return fmt.Sprintf("%s/%s", orderPath, orderParts(id))
}

// getBidsPath returns bids path for queries
func getBidsPath(bfilters BidFilters) string {
	return fmt.Sprintf("%s/%s/%v", bidsPath, bfilters.Owner, bfilters.StateFlagVal)
}

// getBidPath return bid path of given bid id for queries
func getBidPath(id v1.BidID) string {
	return fmt.Sprintf("%s/%s/%s", bidPath, orderParts(id.OrderID()), id.Provider)
}

// getLeasesPath returns leases path for queries
func getLeasesPath(lfilters LeaseFilters) string {
	return fmt.Sprintf("%s/%s/%v", leasesPath, lfilters.Owner, lfilters.StateFlagVal)
}

// LeasePath return lease path of given lease id for queries
func LeasePath(id v1.LeaseID) string {
	return fmt.Sprintf("%s/%s/%s", leasePath, orderParts(id.OrderID()), id.Provider)
}

func orderParts(id v1.OrderID) string {
	return fmt.Sprintf("%s/%v/%v/%v", id.Owner, id.DSeq, id.GSeq, id.OSeq)
}

// parseOrderPath returns orderID details with provided queries, and return
// error if occurred due to wrong query
func parseOrderPath(parts []string) (v1.OrderID, error) {
	if len(parts) < 4 {
		return v1.OrderID{}, ErrInvalidPath
	}

	did, err := dpath.ParseGroupPath(parts[0:3])
	if err != nil {
		return v1.OrderID{}, err
	}

	oseq, err := strconv.ParseUint(parts[3], 10, 32)
	if err != nil {
		return v1.OrderID{}, err
	}

	return v1.MakeOrderID(did, uint32(oseq)), nil
}

// parseBidPath returns bidID details with provided queries, and return
// error if occurred due to wrong query
func parseBidPath(parts []string) (v1.BidID, error) {
	if len(parts) < 5 {
		return v1.BidID{}, ErrInvalidPath
	}

	oid, err := parseOrderPath(parts[0:4])
	if err != nil {
		return v1.BidID{}, err
	}

	provider, err := sdk.AccAddressFromBech32(parts[4])
	if err != nil {
		return v1.BidID{}, err
	}

	return v1.MakeBidID(oid, provider), nil
}

// ParseLeasePath returns leaseID details with provided queries, and return
// error if occurred due to wrong query
func ParseLeasePath(parts []string) (v1.LeaseID, error) {
	bid, err := parseBidPath(parts)
	if err != nil {
		return v1.LeaseID{}, err
	}

	return v1.MakeLeaseID(bid), nil
}
