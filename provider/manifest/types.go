package manifest

import (
	"github.com/ovrclk/akash/manifest"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

// Status is the data structure
type Status struct {
	Deployments uint32
}

type SubmitRequest struct {
	Deployment dtypes.DeploymentID
	Manifest   manifest.Manifest `json:"manifest"`
}
