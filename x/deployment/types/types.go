package types

import (
	"bytes"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/types"
)

// ID method returns DeploymentID details of specific deployment
func (obj Deployment) ID() DeploymentID {
	return obj.DeploymentID
}

// GetResources method returns resources list in group
func (g GroupSpec) GetResources() []types.Resource {
	resources := make([]types.Resource, 0, len(g.Resources))
	for _, r := range g.Resources {
		resources = append(resources, types.Resource{
			Unit:  r.Unit,
			Count: r.Count,
		})
	}
	return resources
}

// GetName method returns group name
func (g GroupSpec) GetName() string {
	return g.Name
}

// Price method returns price of group
func (g GroupSpec) Price() sdk.Coin {
	var price sdk.Coin
	for idx, resource := range g.Resources {
		if idx == 0 {
			price = resource.FullPrice()
			continue
		}
		price = price.Add(resource.FullPrice())
	}
	return price
}

// MatchAttributes method compares provided attributes with specific group attributes
func (g GroupSpec) MatchAttributes(attrs []sdk.Attribute) bool {
loop:
	for _, req := range g.Requirements {
		for _, attr := range attrs {
			if req.Key == attr.Key && req.Value == attr.Value {
				continue loop
			}
		}
		return false
	}
	return true
}

// ID method returns GroupID details of specific group
func (g Group) ID() GroupID {
	return g.GroupID
}

// ValidateOrderable method checks whether group status is Open or not
func (g Group) ValidateOrderable() error {
	switch g.State {
	case GroupOpen:
		return nil
	default:
		return ErrGroupNotOpen
	}
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

// GetName method returns group name
func (g Group) GetName() string {
	return g.GroupSpec.Name
}

// GetResources method returns resources list in group
func (g Group) GetResources() []types.Resource {
	return g.GroupSpec.GetResources()
}

// FullPrice method returns full price of resource
func (r Resource) FullPrice() sdk.Coin {
	return sdk.NewCoin(r.Price.Denom, r.Price.Amount.MulRaw(int64(r.Count)))
}

func (d DeploymentResponse) String() string {
	return fmt.Sprintf(`Deployment
	Owner:   %s
	DSeq:    %d
	State:   %v
	Version: %s
	Num Groups: %d
	`, d.Deployment.DeploymentID.Owner, d.Deployment.DeploymentID.DSeq,
		d.Deployment.State, d.Deployment.Version, len(d.Groups))
}

// DeploymentResponses is a collection of DeploymentResponse
type DeploymentResponses []DeploymentResponse

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
func (filters DeploymentFilters) Accept(obj Deployment) bool {
	// Checking owner filter
	if !filters.Owner.Empty() && !filters.Owner.Equals(obj.DeploymentID.Owner) {
		return false
	}

	// Checking dseq filter
	if filters.DSeq != 0 && filters.DSeq != obj.DeploymentID.DSeq {
		return false
	}

	// Checking state filter
	if filters.State != 0 && filters.State != obj.State {
		return false
	}

	return true
}
