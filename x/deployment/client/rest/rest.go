package rest

import (
	"fmt"
	"net/http"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/types/rest"
	"github.com/gorilla/mux"
	"github.com/ovrclk/akash/x/deployment/query"
)

// RegisterRoutes registers all query routes
func RegisterRoutes(ctx context.CLIContext, r *mux.Router, ns string) {
	// Get all deployments
	r.HandleFunc(fmt.Sprintf("/%s/list", ns), listDeploymentsHandler(ctx, ns)).Methods("GET")

	// Get single deployment info
	r.HandleFunc(fmt.Sprintf("/%s/info", ns), getDeploymentHandler(ctx, ns)).Methods("GET")

	// Get single group info
	r.HandleFunc(fmt.Sprintf("/%s/group/info", ns), getGroupHandler(ctx, ns)).Methods("GET")
}

func listDeploymentsHandler(ctx context.CLIContext, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dfilters, errMsg := DepFiltersFromRequest(r)

		if len(errMsg) != 0 {
			rest.WriteErrorResponse(w, http.StatusBadRequest, errMsg)
			return
		}

		res, err := query.NewRawClient(ctx, ns).Deployments(dfilters)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
			return
		}
		rest.PostProcessResponse(w, ctx, res)
	}
}

func getDeploymentHandler(ctx context.CLIContext, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		id, errMsg := DeploymentIDFromRequest(r)

		if len(errMsg) != 0 {
			rest.WriteErrorResponse(w, http.StatusBadRequest, errMsg)
			return
		}

		res, err := query.NewRawClient(ctx, ns).Deployment(id)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
			return
		}
		rest.PostProcessResponse(w, ctx, res)
	}
}

func getGroupHandler(ctx context.CLIContext, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		id, errMsg := GroupIDFromRequest(r)

		if len(errMsg) != 0 {
			rest.WriteErrorResponse(w, http.StatusBadRequest, errMsg)
			return
		}

		res, err := query.NewRawClient(ctx, ns).Group(id)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
			return
		}
		rest.PostProcessResponse(w, ctx, res)
	}
}
