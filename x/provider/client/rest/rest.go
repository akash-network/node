package rest

import (
	"fmt"
	"net/http"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/gorilla/mux"
)

// RegisterRoutes registers all query routes
func RegisterRoutes(ctx client.Context, r *mux.Router, ns string) {
	// Get providers list
	r.HandleFunc(fmt.Sprintf("/%s/list", ns), listProvidersHandler(ctx, ns)).Methods("GET")

	// Get single provider info
	r.HandleFunc(fmt.Sprintf("/%s/info/{providerOwner}", ns), getProviderHandler(ctx, ns)).Methods("GET")
}

func listProvidersHandler(_ client.Context, _ string) http.HandlerFunc {
	return func(_ http.ResponseWriter, _ *http.Request) {
		// res, err := query.NewRawClient(ctx, ns).Providers()
		// if err != nil {
		// 	rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
		// 	return
		// }
		// rest.PostProcessResponse(w, ctx, res)
	}
}

func getProviderHandler(_ client.Context, _ string) http.HandlerFunc {
	return func(_ http.ResponseWriter, _ *http.Request) {
		// bech32Addr := mux.Vars(r)["providerOwner"]
		//
		// id, err := sdk.AccAddressFromBech32(bech32Addr)
		// if err != nil {
		// 	rest.WriteErrorResponse(w, http.StatusBadRequest, "Invalid address")
		// 	return
		// }
		// res, err := query.NewRawClient(ctx, ns).Provider(id)
		// if err != nil {
		// 	rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
		// 	return
		// }
		// rest.PostProcessResponse(w, ctx, res)
	}
}
