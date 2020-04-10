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
	r.HandleFunc(fmt.Sprintf("/%s/info/{providerOwner}", ns), getProviderHandler(ctx, ns)).Methods("GET")

	// Get provider status
	r.HandleFunc(fmt.Sprintf("/%s/{providerOwner}/status", ns), getProviderStatus(ctx, ns)).Methods("GET")

	// Get deployment manifest of provider
	r.HandleFunc(fmt.Sprintf("/%s/{providerOwner}/deployment/{deploymentID}/manifest", ns),
		getManifestHandler(ctx, ns)).Methods("GET")

	// Get lease status of provider
	r.HandleFunc(fmt.Sprintf("/%s/{providerOwner}/lease/{leaseID}/status", ns),
		getLeaseStatus(ctx, ns)).Methods("GET")

	// Get lease service status of provider
	r.HandleFunc(fmt.Sprintf("/%s/{providerOwner}/lease/{leaseID}/service/{serviceName}/status", ns),
		getServiceStatus(ctx, ns)).Methods("GET")

	// Get lease service logs of provider
	r.HandleFunc(fmt.Sprintf("/%s/{providerOwner}/lease/{leaseID}/service/{serviceName}/logs", ns),
		getServiceLogs(ctx, ns)).Methods("GET")

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

func getProviderStatus(ctx context.CLIContext, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bech32Addr := mux.Vars(r)["providerOwner"]

		id, err := sdk.AccAddressFromBech32(bech32Addr)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusBadRequest, "Invalid address")
			return
		}
		provider, err := query.NewClient(ctx, ns).Provider(id)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Provider not found")
			return
		}

		body, status, fetchErr := fetchURL(fmt.Sprintf("%s/status", provider.HostURI))
		if fetchErr != "" {
			rest.WriteErrorResponse(w, status, fetchErr)
			return
		}

		rest.PostProcessResponseBare(w, ctx, body)
	}
}

func getManifestHandler(ctx context.CLIContext, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bech32Addr := mux.Vars(r)["providerOwner"]
		deploymentID := mux.Vars(r)["deploymentID"]

		id, err := sdk.AccAddressFromBech32(bech32Addr)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusBadRequest, "Invalid address")
			return
		}
		provider, err := query.NewClient(ctx, ns).Provider(id)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Provider not found")
			return
		}

		body, status, fetchErr := fetchURL(fmt.Sprintf("%s/deployment/%s/manifest", provider.HostURI, deploymentID))
		if fetchErr != "" {
			rest.WriteErrorResponse(w, status, fetchErr)
			return
		}

		rest.PostProcessResponseBare(w, ctx, body)
	}
}

func getLeaseStatus(ctx context.CLIContext, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bech32Addr := mux.Vars(r)["providerOwner"]
		leaseID := mux.Vars(r)["leaseID"]

		id, err := sdk.AccAddressFromBech32(bech32Addr)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusBadRequest, "Invalid address")
			return
		}
		provider, err := query.NewClient(ctx, ns).Provider(id)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Provider not found")
			return
		}

		body, status, fetchErr := fetchURL(fmt.Sprintf("%s/lease/%s/status", provider.HostURI, leaseID))
		if fetchErr != "" {
			rest.WriteErrorResponse(w, status, fetchErr)
			return
		}

		rest.PostProcessResponseBare(w, ctx, body)
	}
}

func getServiceStatus(ctx context.CLIContext, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bech32Addr := mux.Vars(r)["providerOwner"]
		leaseID := mux.Vars(r)["leaseID"]
		serviceName := mux.Vars(r)["serviceName"]

		id, err := sdk.AccAddressFromBech32(bech32Addr)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusBadRequest, "Invalid address")
			return
		}
		provider, err := query.NewClient(ctx, ns).Provider(id)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Provider not found")
			return
		}

		body, status, fetchErr := fetchURL(fmt.Sprintf("%s/lease/%s/service/%s/status",
			provider.HostURI, leaseID, serviceName))
		if fetchErr != "" {
			rest.WriteErrorResponse(w, status, fetchErr)
			return
		}

		rest.PostProcessResponseBare(w, ctx, body)
	}
}

func getServiceLogs(ctx context.CLIContext, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bech32Addr := mux.Vars(r)["providerOwner"]
		leaseID := mux.Vars(r)["leaseID"]
		serviceName := mux.Vars(r)["serviceName"]

		id, err := sdk.AccAddressFromBech32(bech32Addr)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusBadRequest, "Invalid address")
			return
		}
		provider, err := query.NewClient(ctx, ns).Provider(id)
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Provider not found")
			return
		}

		body, status, fetchErr := fetchURL(fmt.Sprintf("%s/lease/%s/service/%s/logs",
			provider.HostURI, leaseID, serviceName))
		if fetchErr != "" {
			rest.WriteErrorResponse(w, status, fetchErr)
			return
		}

		rest.PostProcessResponseBare(w, ctx, body)
	}
}
