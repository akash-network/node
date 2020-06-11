package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/tendermint/tendermint/libs/log"

	"github.com/gorilla/mux"
	"github.com/ovrclk/akash/provider"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/manifest"
)

const (
	contentTypeJSON = "application/json; charset=UTF-8"
)

func newRouter(log log.Logger, pclient provider.Client) *mux.Router {
	router := mux.NewRouter()

	// GET /status
	router.HandleFunc("/status",
		createStatusHandler(log, pclient)).
		Methods("GET")

	// PUT /deployment/<deployment-id>/manifest
	drouter := router.PathPrefix(deploymentPathPrefix).Subrouter()
	drouter.Use(requireDeploymentID(log))
	drouter.HandleFunc("/manifest",
		createManifestHandler(log, pclient.Manifest())).
		Methods("PUT")

	lrouter := router.PathPrefix(leasePathPrefix).Subrouter()
	lrouter.Use(requireLeaseID(log))

	// GET /lease/<lease-id>/status
	lrouter.HandleFunc("/status",
		leaseStatusHandler(log, pclient.Cluster())).
		Methods("GET")

	// GET /lease/<lease-id>/service/<service-name>/status
	lrouter.HandleFunc("/service/{serviceName}/status",
		leaseServiceStatusHandler(log, pclient.Cluster())).
		Methods("GET")

	// GET /lease/<lease-id>/service/<service-name>/logs
	lrouter.HandleFunc("/service/{serviceName}/logs",
		leaseServiceLogsHandler(log, pclient.Cluster())).
		Methods("GET")

	return router
}

func createStatusHandler(log log.Logger, sclient provider.StatusClient) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		status, err := sclient.Status(req.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(log, w, status)
	}
}

func createManifestHandler(_ log.Logger, mclient manifest.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var mreq manifest.SubmitRequest

		decoder := json.NewDecoder(req.Body)
		defer req.Body.Close()

		if err := decoder.Decode(&mreq); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}

		if !requestDeploymentID(req).Equals(mreq.Deployment) {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		if err := mclient.Submit(req.Context(), &mreq); err != nil {
			// TODO: surface unauthorized, etc...
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func leaseStatusHandler(log log.Logger, cclient cluster.ReadClient) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		status, err := cclient.LeaseStatus(req.Context(), requestLeaseID(req))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(log, w, status)
	}
}

func leaseServiceStatusHandler(log log.Logger, cclient cluster.ReadClient) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		service := mux.Vars(req)["serviceName"]
		if service == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		status, err := cclient.ServiceStatus(req.Context(), requestLeaseID(req), service)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(log, w, status)
	}
}

func leaseServiceLogsHandler(log log.Logger, cclient cluster.ReadClient) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "unimplemented", http.StatusNotImplemented)
	}
}

func writeJSON(log log.Logger, w http.ResponseWriter, obj interface{}) {
	bytes, err := json.Marshal(obj)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentTypeJSON)

	_, err = w.Write(bytes)
	if err != nil {
		log.Error("error writing response", "err", err)
		return
	}
}
