package query

import (
	"bytes"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/node/deployment/v1beta4"
)

// DeploymentFilters defines flags for deployment list filter
type DeploymentFilters struct {
	Owner sdk.AccAddress
	// State flag value given
	StateFlagVal string
	// Actual state value decoded from DeploymentStateMap
	State v1.Deployment_State
}

// Accept returns whether deployment filters valid or not
func (filters DeploymentFilters) Accept(obj v1.Deployment, isValidState bool) bool {
	if (filters.Owner.Empty() && !isValidState) ||
		(filters.Owner.Empty() && (obj.State == filters.State)) ||
		(!isValidState && (obj.ID.Owner == filters.Owner.String())) ||
		(obj.ID.Owner == filters.Owner.String() && obj.State == filters.State) {
		return true
	}

	return false
}

// Deployment stores deployment and groups details
type Deployment struct {
	v1.Deployment `json:"deployment"`
	Groups        v1beta4.Groups `json:"groups"`
}

func (d Deployment) String() string {
	return fmt.Sprintf(`Deployment
	Owner:   %s
	DSeq:    %d
	State:   %v
	Version: %s
	Num Groups: %d
	`, d.ID.Owner, d.ID.DSeq, d.State, d.Hash, len(d.Groups))
}

// Deployments represents slice of deployment struct
type Deployments []Deployment

func (ds Deployments) String() string {
	var buf bytes.Buffer

	const sep = "\n\n"

	for _, d := range ds {
		buf.WriteString(d.String())
		buf.WriteString(sep)
	}

	if len(ds) > 0 {
		buf.Truncate(buf.Len() - len(sep))
	}

	return buf.String()
}

// Group stores group ID, state and other specifications
type Group v1beta4.Group

// GroupFilters defines flags for group list filter
type GroupFilters struct {
	Owner sdk.AccAddress
	// State flag value given
	StateFlagVal string
	// Actual state value decoded from GroupStateMap
	State v1beta4.Group_State
}
