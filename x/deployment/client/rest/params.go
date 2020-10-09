package rest

import (
	"net/http"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/x/deployment/query"
	"github.com/ovrclk/akash/x/deployment/types"
)

// DeploymentIDFromRequest returns DeploymentID from parsing request
func DeploymentIDFromRequest(r *http.Request) (types.DeploymentID, string) {
	ownerAddr := r.URL.Query().Get("owner")
	dseqNo := r.URL.Query().Get("dseq")
	var id types.DeploymentID

	if len(ownerAddr) != 0 {
		_, err := sdk.AccAddressFromBech32(ownerAddr)
		if err != nil {
			return types.DeploymentID{}, err.Error()
		}
		id.Owner = ownerAddr
	} else {
		return types.DeploymentID{}, "Missing owner query param"
	}

	if len(dseqNo) != 0 {
		dseq, err := strconv.ParseUint(dseqNo, 10, 64)
		if err != nil {
			return types.DeploymentID{}, err.Error()
		}
		id.DSeq = dseq
	} else {
		return types.DeploymentID{}, "Missing dseq query param"
	}
	return id, ""
}

// DepFiltersFromRequest  returns DeploymentFilters with given params in request
func DepFiltersFromRequest(r *http.Request) (query.DeploymentFilters, string) {
	ownerAddr := r.URL.Query().Get("owner")
	state := r.URL.Query().Get("state")
	var dfilters query.DeploymentFilters

	if len(ownerAddr) != 0 {
		owner, err := sdk.AccAddressFromBech32(ownerAddr)
		if err != nil {
			return query.DeploymentFilters{}, err.Error()
		}
		dfilters.Owner = owner
	}

	if len(state) != 0 {
		dfilters.StateFlagVal = state
	}
	return dfilters, ""
}

// GroupIDFromRequest returns GroupID from parsing request
func GroupIDFromRequest(r *http.Request) (types.GroupID, string) {
	dID, errMsg := DeploymentIDFromRequest(r)
	if len(errMsg) != 0 {
		return types.GroupID{}, errMsg
	}
	gseqNo := r.URL.Query().Get("gseq")

	var gseq uint32

	if len(gseqNo) != 0 {
		num, err := strconv.ParseUint(gseqNo, 10, 32)
		if err != nil {
			return types.GroupID{}, err.Error()
		}
		gseq = uint32(num)
	} else {
		return types.GroupID{}, "Missing oseq query param"
	}
	return types.MakeGroupID(dID, gseq), ""
}

// GroupFiltersFromRequest  returns GroupFilters with given params in request
func GroupFiltersFromRequest(r *http.Request) (query.GroupFilters, string) {
	dfilters, errMsg := DepFiltersFromRequest(r)
	if len(errMsg) != 0 {
		return query.GroupFilters{}, errMsg
	}

	gfilters := query.GroupFilters{
		Owner:        dfilters.Owner,
		StateFlagVal: dfilters.StateFlagVal,
	}
	return gfilters, ""
}
