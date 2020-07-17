package query

import (
	"errors"
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/x/deployment/types"
)

const (
	deploymentsPath = "deployments"
	deploymentPath  = "deployment"
	groupPath       = "group"
)

var (
	ErrInvalidPath  = errors.New("query: invalid path")
	ErrOwnerValue   = errors.New("query: invalid owner value")
	ErrStateValue   = errors.New("query: invalid state value")
	ErrInvalidParam = errors.New("query: invalid param")
)

// getDeploymentsPath returns deployments path for queries
func getDeploymentsPath(dfilters DeploymentFilters) string {
	res := fmt.Sprintf("%s/%s/%v", deploymentsPath, dfilters.Owner, dfilters.StateFlagVal)
	if dfilters.After {
		res += "/after"
	}

	return res
}

// DeploymentPath return deployment path of given deployment id for queries
func DeploymentPath(id types.DeploymentID) string {
	return fmt.Sprintf("%s/%s", deploymentPath, deploymentParts(id))
}

// getGroupPath return group path of given group id for queries
func getGroupPath(id types.GroupID) string {
	return fmt.Sprintf("%s/%s/%v/%v", groupPath, id.Owner, id.DSeq, id.GSeq)
}

// parseDeploymentPath returns DeploymentID details with provided queries, and return
// error if occurred due to wrong query
func ParseDeploymentPath(parts []string) (types.DeploymentID, error) {
	if len(parts) < 2 {
		return types.DeploymentID{}, ErrInvalidPath
	}

	owner, err := sdk.AccAddressFromBech32(parts[0])
	if err != nil {
		return types.DeploymentID{}, err
	}

	dseq, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return types.DeploymentID{}, err
	}

	return types.DeploymentID{
		Owner: owner,
		DSeq:  dseq,
	}, nil
}

// parseDepFiltersPath returns DeploymentFilters details with provided queries, and return
// error if occurred due to wrong query
func parseDepFiltersPath(parts []string) (DeploymentFilters, bool, error) {
	if len(parts) < 2 {
		return DeploymentFilters{}, false, ErrInvalidPath
	}

	owner, err := sdk.AccAddressFromBech32(parts[0])
	if err != nil {
		return DeploymentFilters{}, false, err
	}

	if !owner.Empty() && sdk.VerifyAddressFormat(owner) != nil {
		return DeploymentFilters{}, false, ErrOwnerValue
	}

	state, ok := types.DeploymentStateMap[parts[1]]

	if !ok && (parts[1] != "") {
		return DeploymentFilters{}, false, ErrStateValue
	}

	after := false

	if len(parts) > 2 {
		if parts[2] != "after" {
			return DeploymentFilters{}, false, ErrInvalidParam
		}
		after = true
	}

	return DeploymentFilters{
		Owner:        owner,
		StateFlagVal: parts[1],
		State:        state,
		After:        after,
	}, ok, nil
}

// ParseGroupPath returns GroupID details with provided queries, and return
// error if occurred due to wrong query
func ParseGroupPath(parts []string) (types.GroupID, error) {
	if len(parts) < 3 {
		return types.GroupID{}, ErrInvalidPath
	}

	did, err := ParseDeploymentPath(parts[0:2])
	if err != nil {
		return types.GroupID{}, err
	}

	gseq, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		return types.GroupID{}, err
	}

	return types.MakeGroupID(did, uint32(gseq)), nil
}

func deploymentParts(id types.DeploymentID) string {
	return fmt.Sprintf("%s/%v", id.Owner, id.DSeq)
}
