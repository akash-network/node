package rest

// import (
// 	"net/http"
// 	"strconv"
//
// 	sdk "github.com/cosmos/cosmos-sdk/types"
// 	"pkg.akt.dev/go/node/market/v1"
// 	"pkg.akt.dev/go/node/market/v1beta5"
//
// 	drest "pkg.akt.dev/node/x/deployment/client/rest"
// )
//
// // OrderIDFromRequest returns OrderID from parsing request
// func OrderIDFromRequest(r *http.Request) (v1.OrderID, string) {
// 	gID, errMsg := drest.GroupIDFromRequest(r)
// 	if len(errMsg) != 0 {
// 		return v1.OrderID{}, errMsg
// 	}
// 	oseqNo := r.URL.Query().Get("oseq")
//
// 	var oseq uint32
//
// 	if len(oseqNo) != 0 {
// 		num, err := strconv.ParseUint(oseqNo, 10, 32)
// 		if err != nil {
// 			return v1.OrderID{}, err.Error()
// 		}
// 		oseq = uint32(num)
// 	} else {
// 		return v1.OrderID{}, "Missing oseq query param"
// 	}
// 	return v1.MakeOrderID(gID, oseq), ""
// }
//
// // OrderFiltersFromRequest  returns OrderFilters with given params in request
// func OrderFiltersFromRequest(r *http.Request) (v1beta5.OrderFilters, string) {
// 	gfilters, errMsg := drest.GroupFiltersFromRequest(r)
// 	if len(errMsg) != 0 {
// 		return v1beta5.OrderFilters{}, errMsg
// 	}
//
// 	ofilters := v1beta5.OrderFilters{
// 		Owner: gfilters.Owner,
// 		State: gfilters.State,
// 	}
// 	return ofilters, ""
// }
//
// // BidIDFromRequest returns BidID from parsing request
// func BidIDFromRequest(r *http.Request) (v1.BidID, string) {
// 	oID, errMsg := OrderIDFromRequest(r)
// 	if len(errMsg) != 0 {
// 		return v1.BidID{}, errMsg
// 	}
// 	providerAddr := r.URL.Query().Get("provider")
//
// 	var provider sdk.AccAddress
//
// 	if len(providerAddr) != 0 {
// 		addr, err := sdk.AccAddressFromBech32(providerAddr)
// 		if err != nil {
// 			return v1.BidID{}, err.Error()
// 		}
// 		provider = addr
// 	} else {
// 		return v1.BidID{}, "Missing provider query param"
// 	}
//
// 	return v1.MakeBidID(oID, provider), ""
// }
//
// // BidFiltersFromRequest  returns BidFilters with given params in request
// func BidFiltersFromRequest(r *http.Request) (v1beta5.BidFilters, string) {
// 	ofilters, errMsg := drest.GroupFiltersFromRequest(r)
// 	if len(errMsg) != 0 {
// 		return v1beta5.BidFilters{}, errMsg
// 	}
//
// 	bfilters := v1beta5.BidFilters{
// 		Owner: ofilters.Owner,
// 		State: ofilters.State,
// 	}
// 	return bfilters, ""
// }
//
// // LeaseIDFromRequest returns LeaseID from parsing request
// func LeaseIDFromRequest(r *http.Request) (v1.LeaseID, string) {
// 	bID, errMsg := BidIDFromRequest(r)
// 	if len(errMsg) != 0 {
// 		return v1.LeaseID{}, errMsg
// 	}
//
// 	return v1.MakeLeaseID(bID), ""
// }
//
// // LeaseFiltersFromRequest  returns LeaseFilters with given params in request
// func LeaseFiltersFromRequest(r *http.Request) (v1.LeaseFilters, string) {
// 	bfilters, errMsg := drest.GroupFiltersFromRequest(r)
// 	if len(errMsg) != 0 {
// 		return v1.LeaseFilters{}, errMsg
// 	}
//
// 	lfilters := v1.LeaseFilters{
// 		Owner: bfilters.Owner,
// 		State: bfilters.State,
// 	}
// 	return lfilters, ""
// }
