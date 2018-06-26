package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gorilla/mux"
	"github.com/ovrclk/akash/provider/cluster/kube"
	"github.com/ovrclk/akash/provider/manifest"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tmlibs/log"
)

const (
	contentType      = "application/json"
	manifestPath     = "/manifest"
	statusPathPrefix = "/status/"
	statusPath       = statusPathPrefix + "{lease-id}"
	leaseStatusPath  = statusPathPrefix + "{lease-id}/{service-name}"
)

func errorResponse(w http.ResponseWriter, log log.Logger, status int, message string) {
	log.Error("error", "status", status, "message", message)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"message": message})
}

func manifestHandler(log log.Logger, phandler manifest.Handler) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			errorResponse(w,
				log,
				http.StatusMethodNotAllowed,
				http.StatusText(http.StatusMethodNotAllowed))
			return
		}
		if r.Header.Get("Content-Type") != contentType {
			errorResponse(w,
				log,
				http.StatusUnsupportedMediaType,
				fmt.Sprintf("Content-Type '%v' required", contentType))
			return
		}
		if r.Body == nil {
			errorResponse(w, log, http.StatusBadRequest, "Empty request body")
			return
		}

		obj := &types.ManifestRequest{}
		if err := jsonpb.Unmarshal(r.Body, obj); err != nil {
			errorResponse(w, log, http.StatusBadRequest, "Error decoding body")
			return
		}
		r.Body.Close()

		log.Debug(fmt.Sprintf("%+v", obj))

		if err := phandler.HandleManifest(obj); err != nil {
			errorResponse(w, log, http.StatusBadRequest, "Invalid manifest")
			return
		}

		// respond with success
		w.WriteHeader(http.StatusOK)
	}
}

func requestLogger(log log.Logger) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Info(r.Method, "path", r.URL.Path)
			next.ServeHTTP(w, r)
		})
	}
}

func newStatusHandler(log log.Logger, phandler manifest.Handler, client kube.Client) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=us-ascii")

		leaseID := strings.ToLower(strings.TrimPrefix(r.URL.RequestURI(), statusPathPrefix))
		fmt.Println(leaseID)

		deployments, err := client.KubeDeployments(leaseID)
		if err != nil {
			errorResponse(w, log, http.StatusBadRequest, "internal error")
			return
		}
		if deployments == nil {
			errorResponse(w, log, http.StatusBadRequest, "no deployments found for lease")
			return
		}
		response := make(map[string]string)
		for _, deployment := range deployments.Items {
			response[deployment.Name] = fmt.Sprintf("available replicas: %v/%v", deployment.Status.AvailableReplicas, deployment.Status.Replicas)
		}
		json.NewEncoder(w).Encode(response)
		w.WriteHeader(http.StatusOK)
	}
}

func createHandlers(log log.Logger, handler manifest.Handler, client kube.Client) http.Handler {
	r := mux.NewRouter()
	r.HandleFunc(manifestPath, manifestHandler(log, handler))
	r.HandleFunc(statusPath, newStatusHandler(log, handler, client))
	r.Use(requestLogger(log))
	return r
}

func RunServer(ctx context.Context, log log.Logger, port string, handler manifest.Handler, client kube.Client) error {

	address := fmt.Sprintf(":%v", port)

	server := &http.Server{
		Addr:    address,
		Handler: createHandlers(log, handler, client),
	}

	ctx, cancel := context.WithCancel(ctx)

	donech := make(chan struct{})

	go func() {
		defer close(donech)
		<-ctx.Done()
		log.Info("Shutting down server")
		server.Shutdown(context.Background())
	}()

	log.Info("Starting server", "address", address)
	err := server.ListenAndServe()
	cancel()

	<-donech

	log.Info("Server shutdown")

	return err
}
