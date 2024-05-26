package rest

import (
	"net/http"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/node/deployment/v1beta4"
)

// DeploymentIDFromRequest returns DeploymentID from parsing request
func DeploymentIDFromRequest(r *http.Request) (v1.DeploymentID, string) {
	ownerAddr := r.URL.Query().Get("owner")
	dseqNo := r.URL.Query().Get("dseq")
	var id v1.DeploymentID

	if len(ownerAddr) != 0 {
		_, err := sdk.AccAddressFromBech32(ownerAddr)
		if err != nil {
			return v1.DeploymentID{}, err.Error()
		}
		id.Owner = ownerAddr
	} else {
		return v1.DeploymentID{}, "Missing owner query param"
	}

	if len(dseqNo) != 0 {
		dseq, err := strconv.ParseUint(dseqNo, 10, 64)
		if err != nil {
			return v1.DeploymentID{}, err.Error()
		}
		id.DSeq = dseq
	} else {
		return v1.DeploymentID{}, "Missing dseq query param"
	}
	return id, ""
}

// DepFiltersFromRequest  returns DeploymentFilters with given params in request
func DepFiltersFromRequest(r *http.Request) (v1beta4.DeploymentFilters, string) {
	ownerAddr := r.URL.Query().Get("owner")
	state := r.URL.Query().Get("state")
	var dfilters v1beta4.DeploymentFilters

	if len(ownerAddr) != 0 {
		owner, err := sdk.AccAddressFromBech32(ownerAddr)
		if err != nil {
			return v1beta4.DeploymentFilters{}, err.Error()
		}
		dfilters.Owner = owner.String()
	}

	if len(state) != 0 {
		dfilters.State = state
	}
	return dfilters, ""
}

// GroupIDFromRequest returns GroupID from parsing request
func GroupIDFromRequest(r *http.Request) (v1.GroupID, string) {
	dID, errMsg := DeploymentIDFromRequest(r)
	if len(errMsg) != 0 {
		return v1.GroupID{}, errMsg
	}
	gseqNo := r.URL.Query().Get("gseq")

	var gseq uint32

	if len(gseqNo) != 0 {
		num, err := strconv.ParseUint(gseqNo, 10, 32)
		if err != nil {
			return v1.GroupID{}, err.Error()
		}
		gseq = uint32(num)
	} else {
		return v1.GroupID{}, "Missing oseq query param"
	}
	return v1.MakeGroupID(dID, gseq), ""
}

// GroupFiltersFromRequest  returns GroupFilters with given params in request
func GroupFiltersFromRequest(r *http.Request) (v1beta4.GroupFilters, string) {
	dfilters, errMsg := DepFiltersFromRequest(r)
	if len(errMsg) != 0 {
		return v1beta4.GroupFilters{}, errMsg
	}

	gfilters := v1beta4.GroupFilters{
		Owner: dfilters.Owner,
		State: dfilters.State,
	}
	return gfilters, ""
}
