package v1beta2

import (
	"bytes"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "github.com/akash-network/node/types/v1beta2"
)

type attributesMatching map[string]types.Attributes

const (
	// ManifestVersionLength is the length of manifest version
	ManifestVersionLength = 32

	// DefaultOrderBiddingDuration is the default time limit for an Order being active.
	// After the duration, the Order is automatically closed.
	// ( 24(hr) * 3600(seconds per hour) ) / 7s-Block
	DefaultOrderBiddingDuration = int64(12342)

	// MaxBiddingDuration is roughly 30 days of block height
	MaxBiddingDuration = DefaultOrderBiddingDuration * int64(30)
)

// ID method returns DeploymentID details of specific deployment
func (obj Deployment) ID() DeploymentID {
	return obj.DeploymentID
}

// MatchAttributes method compares provided attributes with specific group attributes
func (g GroupSpec) MatchAttributes(attr types.Attributes) bool {
	return types.AttributesSubsetOf(g.Requirements.Attributes, attr)
}

// ID method returns GroupID details of specific group
func (g Group) ID() GroupID {
	return g.GroupID
}

// ValidateClosable provides error response if group is already closed,
// and thus should not be closed again, else nil.
func (g Group) ValidateClosable() error {
	switch g.State {
	case GroupClosed:
		return ErrGroupClosed
	default:
		return nil
	}
}

// ValidatePausable provides error response if group is not pausable
func (g Group) ValidatePausable() error {
	switch g.State {
	case GroupClosed:
		return ErrGroupClosed
	case GroupPaused:
		return ErrGroupPaused
	default:
		return nil
	}
}

// ValidatePausable provides error response if group is not pausable
func (g Group) ValidateStartable() error {
	switch g.State {
	case GroupClosed:
		return ErrGroupClosed
	case GroupOpen:
		return ErrGroupOpen
	default:
		return nil
	}
}

// GetName method returns group name
func (g Group) GetName() string {
	return g.GroupSpec.Name
}

// GetResources method returns resources list in group
func (g Group) GetResources() []types.Resources {
	return g.GroupSpec.GetResources()
}

// FullPrice method returns full price of resource
func (r Resource) FullPrice() sdk.DecCoin {
	return sdk.NewDecCoinFromDec(r.Price.Denom, r.Price.Amount.MulInt64(int64(r.Count)))
}

// DeploymentResponses is a collection of DeploymentResponse
type DeploymentResponses []QueryDeploymentResponse

func (ds DeploymentResponses) String() string {
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

// Accept returns whether deployment filters valid or not
func (filters DeploymentFilters) Accept(obj Deployment, stateVal Deployment_State) bool {
	// Checking owner filter
	if filters.Owner != "" && filters.Owner != obj.DeploymentID.Owner {
		return false
	}

	// Checking dseq filter
	if filters.DSeq != 0 && filters.DSeq != obj.DeploymentID.DSeq {
		return false
	}

	// Checking state filter
	if stateVal != 0 && stateVal != obj.State {
		return false
	}

	return true
}
