package rest

import (
	"fmt"
	"net/http"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/gorilla/mux"
	// "pkg.akt.dev/node/x/market/query"
)

// RegisterRoutes registers all query routes
func RegisterRoutes(ctx client.Context, r *mux.Router, ns string) {
	// Get all orders
	r.HandleFunc(fmt.Sprintf("/%s/order/list", ns), listOrdersHandler(ctx, ns)).Methods("GET")

	// Get single order info
	r.HandleFunc(fmt.Sprintf("/%s/order/info", ns), getOrderHandler(ctx, ns)).Methods("GET")

	// Get all bids
	r.HandleFunc(fmt.Sprintf("/%s/bid/list", ns), listBidsHandler(ctx, ns)).Methods("GET")

	// Get single bid info
	r.HandleFunc(fmt.Sprintf("/%s/bid/info", ns), getBidHandler(ctx, ns)).Methods("GET")

	// Get all leases
	r.HandleFunc(fmt.Sprintf("/%s/lease/list", ns), listLeasesHandler(ctx, ns)).Methods("GET")

	// Get single order info
	r.HandleFunc(fmt.Sprintf("/%s/lease/info", ns), getLeaseHandler(ctx, ns)).Methods("GET")
}

func listOrdersHandler(_ client.Context, _ string) http.HandlerFunc {
	return func(_ http.ResponseWriter, _ *http.Request) {
		// ofilters, errMsg := OrderFiltersFromRequest(r)
		//
		// if len(errMsg) != 0 {
		// 	rest.WriteErrorResponse(w, http.StatusBadRequest, errMsg)
		// 	return
		// }
		//
		// res, err := query.NewRawClient(ctx, ns).Orders(ofilters)
		// if err != nil {
		// 	rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
		// 	return
		// }
		// rest.PostProcessResponse(w, ctx, res)
	}
}

func listBidsHandler(_ client.Context, _ string) http.HandlerFunc {
	return func(_ http.ResponseWriter, _ *http.Request) {
		// bfilters, errMsg := BidFiltersFromRequest(r)
		//
		// if len(errMsg) != 0 {
		// 	rest.WriteErrorResponse(w, http.StatusBadRequest, errMsg)
		// 	return
		// }
		//
		// res, err := query.NewRawClient(ctx, ns).Bids(bfilters)
		// if err != nil {
		// 	rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
		// 	return
		// }
		// rest.PostProcessResponse(w, ctx, res)
	}
}

func listLeasesHandler(_ client.Context, _ string) http.HandlerFunc {
	return func(_ http.ResponseWriter, _ *http.Request) {
		// lfilters, errMsg := LeaseFiltersFromRequest(r)
		//
		// if len(errMsg) != 0 {
		// 	rest.WriteErrorResponse(w, http.StatusBadRequest, errMsg)
		// 	return
		// }
		// res, err := query.NewRawClient(ctx, ns).Leases(lfilters)
		// if err != nil {
		// 	rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
		// 	return
		// }
		// rest.PostProcessResponse(w, ctx, res)
	}
}

func getOrderHandler(_ client.Context, _ string) http.HandlerFunc {
	return func(_ http.ResponseWriter, _ *http.Request) {
		// id, errMsg := OrderIDFromRequest(r)
		//
		// if len(errMsg) != 0 {
		// 	rest.WriteErrorResponse(w, http.StatusBadRequest, errMsg)
		// 	return
		// }
		//
		// res, err := query.NewRawClient(ctx, ns).Order(id)
		// if err != nil {
		// 	rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
		// 	return
		// }
		// rest.PostProcessResponse(w, ctx, res)
	}
}

func getBidHandler(_ client.Context, _ string) http.HandlerFunc {
	return func(_ http.ResponseWriter, _ *http.Request) {
		// id, errMsg := BidIDFromRequest(r)
		//
		// if len(errMsg) != 0 {
		// 	rest.WriteErrorResponse(w, http.StatusBadRequest, errMsg)
		// 	return
		// }
		//
		// res, err := query.NewRawClient(ctx, ns).Bid(id)
		// if err != nil {
		// 	rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
		// 	return
		// }
		// rest.PostProcessResponse(w, ctx, res)
	}
}

func getLeaseHandler(_ client.Context, _ string) http.HandlerFunc {
	return func(_ http.ResponseWriter, _ *http.Request) {
		// id, errMsg := LeaseIDFromRequest(r)
		//
		// if len(errMsg) != 0 {
		// 	rest.WriteErrorResponse(w, http.StatusBadRequest, errMsg)
		// 	return
		// }
		//
		// res, err := query.NewRawClient(ctx, ns).Lease(id)
		// if err != nil {
		// 	rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
		// 	return
		// }
		// rest.PostProcessResponse(w, ctx, res)
	}
}
