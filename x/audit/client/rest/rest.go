package rest

import (
	"fmt"
	"net/http"

	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/rest"
	"github.com/gorilla/mux"

	types "github.com/akash-network/akash-api/go/node/audit/v1beta3"

	"github.com/akash-network/node/x/audit/query"
)

// RegisterRoutes registers all query routes
func RegisterRoutes(ctx client.Context, r *mux.Router, ns string) {
	prefix := fmt.Sprintf("/%s/attributes", ns)

	// Get all signed
	r.HandleFunc(fmt.Sprintf("/%s/list", prefix), listAllSignedHandler(ctx, ns)).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s/owner/{providerOwner}/list", prefix), listProviderAttributes(ctx, ns)).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s/auditor/{auditor}/{providerOwner}", prefix), listAuditorProviderAttributes(ctx, ns)).Methods("GET")
}

func listAllSignedHandler(ctx client.Context, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := query.NewRawClient(ctx, ns).AllProviders()
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
			return
		}
		rest.PostProcessResponse(w, ctx, res)
	}
}

func listProviderAttributes(ctx client.Context, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := sdk.AccAddressFromBech32(mux.Vars(r)["providerOwner"])
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

func listAuditorProviderAttributes(ctx client.Context, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auditor, err := sdk.AccAddressFromBech32(mux.Vars(r)["auditor"])
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusBadRequest, "Invalid address")
			return
		}

		var res []byte

		if addr := mux.Vars(r)["providerOwner"]; addr == "list" {
			res, err = query.NewRawClient(ctx, ns).Auditor(auditor)
		} else {
			var owner sdk.AccAddress
			if owner, err = sdk.AccAddressFromBech32(addr); err != nil {
				rest.WriteErrorResponse(w, http.StatusBadRequest, "Invalid address")
				return
			}

			res, err = query.NewRawClient(ctx, ns).ProviderID(types.ProviderID{
				Owner:   owner,
				Auditor: auditor,
			})
		}

		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
			return
		}

		rest.PostProcessResponse(w, ctx, res)
	}
}
