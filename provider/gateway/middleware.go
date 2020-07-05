package gateway

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/context"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/tendermint/tendermint/libs/log"

	dquery "github.com/ovrclk/akash/x/deployment/query"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mquery "github.com/ovrclk/akash/x/market/query"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

type contextKey int

const (
	leaseContextKey contextKey = iota + 1
	deploymentContextKey
	logFollowContextKey
	tailLinesContextKey
	serviceContextKey
)

func requestLeaseID(req *http.Request) mtypes.LeaseID {
	return context.Get(req, leaseContextKey).(mtypes.LeaseID)
}

func requestDeploymentID(req *http.Request) dtypes.DeploymentID {
	return context.Get(req, deploymentContextKey).(dtypes.DeploymentID)
}

func requestLogFollow(req *http.Request) bool {
	return context.Get(req, logFollowContextKey).(bool)
}

func requestLogTailLines(req *http.Request) *int64 {
	return context.Get(req, tailLinesContextKey).(*int64)
}

func requestService(req *http.Request) string {
	return context.Get(req, serviceContextKey).(string)
}

func requireDeploymentID(_ log.Logger) mux.MiddlewareFunc {
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

func requireLeaseID(_ log.Logger) mux.MiddlewareFunc {
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

func requireService() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			vars := mux.Vars(req)

			svc := vars["serviceName"]
			if svc == "" {
				http.Error(w, "empty service name", http.StatusBadRequest)
				return
			}

			fmt.Printf("param service %s\n", svc)
			context.Set(req, serviceContextKey, svc)
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

func requestLogParams() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			vars := req.URL.Query()

			var err error

			defer func() {
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}()

			follow := false
			var tailLines *int64

			val := vars.Get("follow")
			if val == "" {
				err = errors.Errorf("query must contain \"follow\" key")
				return
			}

			follow, err = strconv.ParseBool(val)
			if err != nil {
				return
			}

			val = vars.Get("tail")
			if val == "" {
				err = errors.Errorf("query must contain \"tail\" key")
				return
			}

			vl := new(int64)
			*vl, err = strconv.ParseInt(val, 10, 32)
			if err != nil {
				return
			}

			if *vl < -1 {
				err = errors.Errorf("parameter \"tail\" contains invalid value")
				return
			}

			if *vl > -1 {
				tailLines = vl
			}

			context.Set(req, logFollowContextKey, follow)
			context.Set(req, tailLinesContextKey, tailLines)

			next.ServeHTTP(w, req)
		})
	}
}
