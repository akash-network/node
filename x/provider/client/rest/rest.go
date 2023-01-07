package rest

import (
	"fmt"
	"net/http"

	"github.com/akash-network/node/x/provider/query"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/rest"
	"github.com/gorilla/mux"
)

// RegisterRoutes registers all query routes
func RegisterRoutes(ctx client.Context, r *mux.Router, ns string) {
	// Get providers list
	r.HandleFunc(fmt.Sprintf("/%s/list", ns), listProvidersHandler(ctx, ns)).Methods("GET")

	// Get single provider info
	r.HandleFunc(fmt.Sprintf("/%s/info/{providerOwner}", ns), getProviderHandler(ctx, ns)).Methods("GET")
}

func listProvidersHandler(ctx client.Context, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := query.NewRawClient(ctx, ns).Providers()
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
			return
		}
		rest.PostProcessResponse(w, ctx, res)
	}
}

func getProviderHandler(ctx client.Context, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bech32Addr := mux.Vars(r)["providerOwner"]

		id, err := sdk.AccAddressFromBech32(bech32Addr)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusBadRequest, "Invalid address")
			return
		}
		res, err := query.NewRawClient(ctx, ns).Provider(id)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
			return
		}
		rest.PostProcessResponse(w, ctx, res)
	}
}
