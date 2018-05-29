package manifest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	pmanifest "github.com/ovrclk/akash/provider/manifest"
	"github.com/tendermint/tmlibs/log"
)

const (
	contentType = "application/json"
)

type handler struct {
	phandler pmanifest.Handler
	log      log.Logger
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		h.error(w,
			http.StatusMethodNotAllowed,
			http.StatusText(http.StatusMethodNotAllowed))
		return
	}
	if r.Header.Get("Content-Type") != contentType {
		h.error(w,
			http.StatusUnsupportedMediaType,
			fmt.Sprintf("Content-Type '%v' required", contentType))
		return
	}
	if r.Body == nil {
		h.error(w, http.StatusBadRequest, "Empty request body")
		return
	}

	obj, err := unmarshalRequest(r.Body)
	if err != nil {
		h.error(w, http.StatusBadRequest, "Error decoding body")
		return
	}

	h.log.Debug(fmt.Sprintf("%+v", obj))

	h.phandler.HandleManifest(obj)

	// XXX check if signer is tenant of lease

	// respond with success
	w.WriteHeader(http.StatusOK)
}

func (h *handler) error(w http.ResponseWriter, status int, message string) {
	h.log.Error("error", "status", status, "message", message)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"message": message})
}

func newHandler(log log.Logger, phandler pmanifest.Handler) http.Handler {
	return &handler{
		log:      log,
		phandler: phandler,
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

func createHandlers(log log.Logger, handler pmanifest.Handler) http.Handler {
	r := mux.NewRouter()
	r.Handle("/manifest", newHandler(log, handler))
	r.Use(requestLogger(log))
	return r
}

func RunServer(ctx context.Context, log log.Logger, port string, handler pmanifest.Handler) error {

	address := fmt.Sprintf(":%v", port)

	server := &http.Server{
		Addr:    address,
		Handler: createHandlers(log, handler),
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
