package rest

import (
	"fmt"
	"net/http"

	"github.com/cosmos/cosmos-sdk/client/context"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/rest"
	"github.com/gorilla/mux"
	"github.com/ovrclk/akash/x/provider/query"
)

// RegisterRoutes registers all query routes
func RegisterRoutes(ctx context.CLIContext, r *mux.Router, ns string) {
	// Get providers list
	r.HandleFunc(fmt.Sprintf("/%s/list", ns), listProvidersHandler(ctx, ns)).Methods("GET")

	// Get single provider info
	r.HandleFunc(fmt.Sprintf("/%s/{providerOwner}/info", ns), getProviderHandler(ctx, ns)).Methods("GET")
}

func listProvidersHandler(ctx context.CLIContext, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := query.NewRawClient(ctx, ns).Providers()
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
			return
		}
		rest.PostProcessResponse(w, ctx, res)
	}
}

func getProviderHandler(ctx context.CLIContext, ns string) http.HandlerFunc {
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
