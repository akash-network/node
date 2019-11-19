package rest

import (
	"fmt"
	"net/http"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/types/rest"
	"github.com/gorilla/mux"
	"github.com/ovrclk/akash/x/deployment/query"
)

func RegisterRoutes(ctx context.CLIContext, r *mux.Router, ns string) {
	r.HandleFunc(fmt.Sprintf("/%s/deployments", ns), listDeploymentsHandler(ctx, ns)).Methods("GET")
}

func listDeploymentsHandler(ctx context.CLIContext, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, _, err := ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", ns, query.DeploymentsPath()), nil)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
			return
		}
		rest.PostProcessResponse(w, ctx, res)
	}
}
