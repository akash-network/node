package rest

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/types/rest"
	"github.com/ovrclk/akash/x/supply/query"
	"github.com/ovrclk/akash/x/supply/types"
)

// RegisterRoutes registers all query routes
func RegisterRoutes(ctx context.CLIContext, r *mux.Router) {
	r.HandleFunc("/supply/circulating", circulatingSupplyHandler(ctx)).Methods("GET")
}

func circulatingSupplyHandler(ctx context.CLIContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := query.NewRawClient(ctx, types.ModuleName).CirculatingSupply()
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		rest.PostProcessResponse(w, ctx, res)
	}
}
