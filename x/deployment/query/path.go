package query

import (
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

func DeploymentsPath() string {
	return deploymentsPath
}

func DeploymentPath(id types.DeploymentID) string {
	return fmt.Sprintf("%s/%s/%v", deploymentPath, id.Owner, id.DSeq)
}

func GroupPath(id types.GroupID) string {
	return fmt.Sprintf("%s/%s/%v/%v", groupPath, id.Owner, id.DSeq, id.GSeq)
}

func ParseDeploymentPath(parts []string) (types.DeploymentID, error) {
	if len(parts) < 2 {
		return types.DeploymentID{}, fmt.Errorf("invalid path")
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

func ParseGroupPath(parts []string) (types.GroupID, error) {
	if len(parts) < 3 {
		return types.GroupID{}, fmt.Errorf("invalid path")
	}

	did, err := ParseDeploymentPath(parts[0:2])
	if err != nil {
		return types.GroupID{}, err
	}

	gseq, err := strconv.ParseUint(parts[2], 10, 32)

	return types.MakeGroupID(did, uint32(gseq)), nil
}
