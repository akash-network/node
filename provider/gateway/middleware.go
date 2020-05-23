package gateway

import (
	"net/http"

	"github.com/gorilla/context"
	"github.com/gorilla/mux"
	dquery "github.com/ovrclk/akash/x/deployment/query"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mquery "github.com/ovrclk/akash/x/market/query"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/tendermint/tendermint/libs/log"
)

type contextKey int

const (
	leaseContextKey      contextKey = 1
	deploymentContextKey contextKey = 2
)

func requestLeaseID(req *http.Request) mtypes.LeaseID {
	return context.Get(req, leaseContextKey).(mtypes.LeaseID)
}

func requestDeploymentID(req *http.Request) dtypes.DeploymentID {
	return context.Get(req, deploymentContextKey).(dtypes.DeploymentID)
}

func requireDeploymentID(log log.Logger) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id, err := parseDeploymentID(req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			context.Set(req, deploymentContextKey, id)
			next.ServeHTTP(w, req)
		})
	}
}

func requireLeaseID(log log.Logger) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id, err := parseLeaseID(req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			context.Set(req, leaseContextKey, id)
			next.ServeHTTP(w, req)
		})
	}
}

func parseDeploymentID(req *http.Request) (dtypes.DeploymentID, error) {
	vars := mux.Vars(req)
	return dquery.ParseDeploymentPath([]string{
		vars["owner"],
		vars["dseq"],
	})
}

func parseLeaseID(req *http.Request) (mtypes.LeaseID, error) {
	vars := mux.Vars(req)
	return mquery.ParseLeasePath([]string{
		vars["owner"],
		vars["dseq"],
		vars["gseq"],
		vars["oseq"],
		vars["provider"],
	})
}
