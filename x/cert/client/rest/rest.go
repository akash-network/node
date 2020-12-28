package rest

import (
	"fmt"
	"math/big"
	"net/http"

	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/rest"
	"github.com/gorilla/mux"

	"github.com/ovrclk/akash/x/cert/query"
	"github.com/ovrclk/akash/x/cert/types"
)

func RegisterRoutes(ctx client.Context, r *mux.Router, ns string) {
	prefix := fmt.Sprintf("/%s/certificates", ns)

	r.HandleFunc(fmt.Sprintf("/%s/list", prefix), listAll(ctx, ns)).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s/state/{state}/list", prefix), listAllState(ctx, ns)).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s/owner/{owner}/list", prefix), listOwner(ctx, ns)).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s/owner/{owner}/state/{state}/list", prefix), listOwnerState(ctx, ns)).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s/owner/{owner}/{serial}", prefix), getOwnerSerial(ctx, ns)).Methods("GET")
}

func listAll(ctx client.Context, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := query.NewRawClient(ctx, ns).Certificates()
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
			return
		}
		rest.PostProcessResponse(w, ctx, res)
	}
}

func listAllState(ctx client.Context, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := query.NewRawClient(ctx, ns).CertificatesState(mux.Vars(r)["state"])
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
			return
		}
		rest.PostProcessResponse(w, ctx, res)
	}
}

func listOwner(ctx client.Context, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, err := sdk.AccAddressFromBech32(mux.Vars(r)["owner"])
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusBadRequest, "Invalid address")
			return
		}

		var res []byte
		if res, err = query.NewRawClient(ctx, ns).Owner(owner); err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
			return
		}

		rest.PostProcessResponse(w, ctx, res)
	}
}

func listOwnerState(ctx client.Context, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, err := sdk.AccAddressFromBech32(mux.Vars(r)["owner"])
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusBadRequest, "Invalid address")
			return
		}

		var res []byte
		if res, err = query.NewRawClient(ctx, ns).OwnerState(owner, mux.Vars(r)["state"]); err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
			return
		}

		rest.PostProcessResponse(w, ctx, res)
	}
}

func getOwnerSerial(ctx client.Context, ns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, err := sdk.AccAddressFromBech32(mux.Vars(r)["owner"])
		if err != nil {
			rest.WriteErrorResponse(w, http.StatusBadRequest, "Invalid address")
			return
		}

		serial, valid := new(big.Int).SetString(mux.Vars(r)["serial"], 10)
		if !valid {
			rest.WriteErrorResponse(w, http.StatusBadRequest, "Invalid serial number")
			return
		}

		var res []byte

		res, err = query.NewRawClient(ctx, ns).Certificate(types.CertID{
			Owner:  owner,
			Serial: *serial,
		})

		if err != nil {
			rest.WriteErrorResponse(w, http.StatusNotFound, "Not Found")
			return
		}

		rest.PostProcessResponse(w, ctx, res)
	}
}
