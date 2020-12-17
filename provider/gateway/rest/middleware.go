package rest

import (
	"net/http"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/gorilla/context"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"

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
	ownerContextKey
	providerContextKey
)

func requestLeaseID(req *http.Request) mtypes.LeaseID {
	return context.Get(req, leaseContextKey).(mtypes.LeaseID)
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

func requestProvider(req *http.Request) sdk.Address {
	return context.Get(req, providerContextKey).(sdk.Address)
}

func requestOwner(req *http.Request) sdk.Address {
	return context.Get(req, ownerContextKey).(sdk.Address)
}

func requestDeploymentID(req *http.Request) dtypes.DeploymentID {
	return context.Get(req, deploymentContextKey).(dtypes.DeploymentID)
}

func requireOwner() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
				http.Error(w, "", http.StatusUnauthorized)
				return
			}

			// at this point client certificate has been validated
			// so only thing left to do is get account id stored in the CommonName
			owner, err := sdk.AccAddressFromBech32(r.TLS.PeerCertificates[0].Subject.CommonName)
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}

			context.Set(r, ownerContextKey, owner)
			next.ServeHTTP(w, r)
		})
	}
}

func requireDeploymentID() mux.MiddlewareFunc {
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

func requireLeaseID() mux.MiddlewareFunc {
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

			context.Set(req, serviceContextKey, svc)
			next.ServeHTTP(w, req)
		})
	}
}

func parseDeploymentID(req *http.Request) (dtypes.DeploymentID, error) {
	var parts []string
	parts = append(parts, requestOwner(req).String())
	parts = append(parts, mux.Vars(req)["dseq"])
	return dtypes.ParseDeploymentPath(parts)
}

func parseLeaseID(req *http.Request) (mtypes.LeaseID, error) {
	vars := mux.Vars(req)

	parts := []string{
		requestOwner(req).String(),
		vars["dseq"],
		vars["gseq"],
		vars["oseq"],
		requestProvider(req).String(),
	}

	return mquery.ParseLeasePath(parts)
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

			var follow bool
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
