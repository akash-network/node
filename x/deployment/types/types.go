package types

import (
	"bytes"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/types"
	atypes "github.com/ovrclk/akash/x/audit/types"
)

type attributesMatching map[string]types.Attributes

// DefaultOrderBiddingDuration is the default time limit for an Order being active.
// After the duration, the Order is automatically closed.
// ( 24(hr) * 3600(seconds per hour) ) / 7s-Block
const DefaultOrderBiddingDuration = int64(12342)

// MaxBiddingDuration is roughly 30 days of block height
const MaxBiddingDuration = DefaultOrderBiddingDuration * int64(30)

// ID method returns DeploymentID details of specific deployment
func (obj Deployment) ID() DeploymentID {
	return obj.DeploymentID
}

// ValidateBasic asserts non-zero values
// TODO: This is causing an import cycle. I think there is some pattern here I'm missing tho..
func (g GroupSpec) ValidateBasic() error {
	return validateDeploymentGroup(g)
}

// GetResources method returns resources list in group
func (g GroupSpec) GetResources() []types.Resources {
	resources := make([]types.Resources, 0, len(g.Resources))
	for _, r := range g.Resources {
		resources = append(resources, types.Resources{
			Resources: r.Resources,
			Count:     r.Count,
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

// MatchRequirements method compares provided attributes with specific group attributes.
// Argument provider is a bit cumbersome. First element is attributes from x/provider store
// in case tenant does not need signed attributes at all
// rest of elements (if any) are attributes signed by various auditors
func (g GroupSpec) MatchRequirements(provider []atypes.Provider) bool {
	if (len(g.Requirements.SignedBy.AnyOf) != 0) || (len(g.Requirements.SignedBy.AllOf) != 0) {
		// we cannot match if there is no signed attributes
		if len(provider) < 2 {
			return false
		}

		existingRequirements := make(attributesMatching)

		for _, existing := range provider[1:] {
			existingRequirements[existing.Validator] = existing.Attributes
		}

		if len(g.Requirements.SignedBy.AllOf) != 0 {
			for _, validator := range g.Requirements.SignedBy.AllOf {
				if existingAttr, exists := existingRequirements[validator]; exists {
					if !types.AttributesSubsetOf(g.Requirements.Attributes, existingAttr) {
						return false
					}
				} else {
					return false
				}
			}
			return true
		}

		for _, validator := range g.Requirements.SignedBy.AnyOf {
			if existingAttr, exists := existingRequirements[validator]; exists {
				if types.AttributesSubsetOf(g.Requirements.Attributes, existingAttr) {
					return true
				}
			}
		}

		return false
	}

	return types.AttributesSubsetOf(g.Requirements.Attributes, provider[0].Attributes)
}

// MatchAttributes method compares provided attributes with specific group attributes
func (g GroupSpec) MatchAttributes(attr types.Attributes) bool {
	return types.AttributesSubsetOf(g.Requirements.Attributes, attr)
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
func (g Group) GetResources() []types.Resources {
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
