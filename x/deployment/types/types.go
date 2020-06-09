package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/types"
)

//go:generate stringer -linecomment -output=autogen_stringer.go -type=DeploymentState,GroupState

// DeploymentState defines state of deployment
type DeploymentState uint8

const (
	// DeploymentActive is used when state of deployment is active
	DeploymentActive DeploymentState = iota + 1 // active
	// DeploymentClosed is used when state of deployment is closed
	DeploymentClosed // closed
)

// DeploymentStateMap is used to decode deployment state flag value
var DeploymentStateMap = map[string]DeploymentState{
	"active": DeploymentActive,
	"closed": DeploymentClosed,
}

// Deployment stores deploymentID, state and version details
type Deployment struct {
	DeploymentID `json:"id"`
	State        DeploymentState `json:"state"`
	Version      []byte          `json:"version"`
}

// ID method returns DeploymentID details of specific deployment
func (obj Deployment) ID() DeploymentID {
	return obj.DeploymentID
}

// GroupState defines state of group
type GroupState uint8

const (
	// GroupOpen is used when state of group is open
	GroupOpen GroupState = iota + 1 // open
	// GroupOrdered is used when state of group is ordered
	GroupOrdered // ordered
	// GroupMatched is used when state of group is matched
	GroupMatched // matched
	// GroupInsufficientFunds is used when group has insufficient funds
	GroupInsufficientFunds // insufficient-funds
	// GroupClosed is used when state of group is closed
	GroupClosed // closed
)

// GroupSpec stores group specifications
type GroupSpec struct {
	Name         string          `json:"name"`
	Requirements []sdk.Attribute `json:"requirements"`
	Resources    []Resource      `json:"resources"`
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

// Group stores groupID, state and other specifications
type Group struct {
	GroupID   `json:"id"`
	State     GroupState `json:"state"`
	GroupSpec `json:"spec"`
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

// Resource stores unit, count and price of each resource
type Resource struct {
	Unit  types.Unit `json:"unit"`
	Count uint32     `json:"count"`
	Price sdk.Coin   `json:"price"`
}

// GetUnit method returns unit of resource
func (r Resource) GetUnit() types.Unit {
	return r.Unit
}

// GetCount method returns count of resource
func (r Resource) GetCount() uint32 {
	return r.Count
}

// FullPrice method returns full price of resource
func (r Resource) FullPrice() sdk.Coin {
	return sdk.NewCoin(r.Price.Denom, r.Price.Amount.MulRaw(int64(r.Count)))
}
