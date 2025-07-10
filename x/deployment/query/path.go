package query

import (
	"errors"
	"fmt"
	"strconv"

	types "pkg.akt.dev/go/node/deployment/v1"
)

const (
	deploymentsPath = "deployments"
	deploymentPath  = "deployment"
	groupPath       = "group"
)

var (
	ErrInvalidPath = errors.New("query: invalid path")
)

// getDeploymentsPath returns deployments path for queries
func getDeploymentsPath(dfilters DeploymentFilters) string {
	return fmt.Sprintf("%s/%s/%v", deploymentsPath, dfilters.Owner, dfilters.StateFlagVal)
}

// DeploymentPath return deployment path of given deployment id for queries
func DeploymentPath(id types.DeploymentID) string {
	return fmt.Sprintf("%s/%s", deploymentPath, deploymentParts(id))
}

// getGroupPath return group path of given group id for queries
func getGroupPath(id types.GroupID) string {
	return fmt.Sprintf("%s/%s/%v/%v", groupPath, id.Owner, id.DSeq, id.GSeq)
}

// ParseGroupPath returns GroupID details with provided queries, and return
// error if occurred due to wrong query
func ParseGroupPath(parts []string) (types.GroupID, error) {
	if len(parts) < 3 {
		return types.GroupID{}, ErrInvalidPath
	}

	did, err := types.ParseDeploymentPath(parts[0:2])
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
