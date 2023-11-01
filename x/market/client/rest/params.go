package rest

import (
	"net/http"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "github.com/akash-network/akash-api/go/node/market/v1beta4"

	drest "github.com/akash-network/node/x/deployment/client/rest"
	"github.com/akash-network/node/x/market/query"
)

// OrderIDFromRequest returns OrderID from parsing request
func OrderIDFromRequest(r *http.Request) (types.OrderID, string) {
	gID, errMsg := drest.GroupIDFromRequest(r)
	if len(errMsg) != 0 {
		return types.OrderID{}, errMsg
	}
	oseqNo := r.URL.Query().Get("oseq")

	var oseq uint32

	if len(oseqNo) != 0 {
		num, err := strconv.ParseUint(oseqNo, 10, 32)
		if err != nil {
			return types.OrderID{}, err.Error()
		}
		oseq = uint32(num)
	} else {
		return types.OrderID{}, "Missing oseq query param"
	}
	return types.MakeOrderID(gID, oseq), ""
}

// OrderFiltersFromRequest  returns OrderFilters with given params in request
func OrderFiltersFromRequest(r *http.Request) (query.OrderFilters, string) {
	gfilters, errMsg := drest.GroupFiltersFromRequest(r)
	if len(errMsg) != 0 {
		return query.OrderFilters{}, errMsg
	}

	ofilters := query.OrderFilters{
		Owner:        gfilters.Owner,
		StateFlagVal: gfilters.StateFlagVal,
	}
	return ofilters, ""
}

// BidIDFromRequest returns BidID from parsing request
func BidIDFromRequest(r *http.Request) (types.BidID, string) {
	oID, errMsg := OrderIDFromRequest(r)
	if len(errMsg) != 0 {
		return types.BidID{}, errMsg
	}
	providerAddr := r.URL.Query().Get("provider")

	var provider sdk.AccAddress

	if len(providerAddr) != 0 {
		addr, err := sdk.AccAddressFromBech32(providerAddr)
		if err != nil {
			return types.BidID{}, err.Error()
		}
		provider = addr
	} else {
		return types.BidID{}, "Missing provider query param"
	}

	return types.MakeBidID(oID, provider), ""
}

// BidFiltersFromRequest  returns BidFilters with given params in request
func BidFiltersFromRequest(r *http.Request) (query.BidFilters, string) {
	ofilters, errMsg := drest.GroupFiltersFromRequest(r)
	if len(errMsg) != 0 {
		return query.BidFilters{}, errMsg
	}

	bfilters := query.BidFilters{
		Owner:        ofilters.Owner,
		StateFlagVal: ofilters.StateFlagVal,
	}
	return bfilters, ""
}

// LeaseIDFromRequest returns LeaseID from parsing request
func LeaseIDFromRequest(r *http.Request) (types.LeaseID, string) {
	bID, errMsg := BidIDFromRequest(r)
	if len(errMsg) != 0 {
		return types.LeaseID{}, errMsg
	}

	return types.MakeLeaseID(bID), ""
}

// LeaseFiltersFromRequest  returns LeaseFilters with given params in request
func LeaseFiltersFromRequest(r *http.Request) (query.LeaseFilters, string) {
	bfilters, errMsg := drest.GroupFiltersFromRequest(r)
	if len(errMsg) != 0 {
		return query.LeaseFilters{}, errMsg
	}

	lfilters := query.LeaseFilters{
		Owner:        bfilters.Owner,
		StateFlagVal: bfilters.StateFlagVal,
	}
	return lfilters, ""
}
