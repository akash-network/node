package types

import (
	"bytes"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/types"
	tmkv "github.com/tendermint/tendermint/libs/kv"
)

// DeploymentState defines state of deployment
type DeploymentState uint8

const (
	// DeploymentActive is used when state of deployment is active
	DeploymentActive DeploymentState = iota
	// DeploymentClosed is used when state of deployment is closed
	DeploymentClosed DeploymentState = iota
)

// DeploymentFilters defines flags for deployment list filter
type DeploymentFilters struct {
	Owner sdk.AccAddress
	// State flag value given
	StateFlagVal string
	// Actual state value decoded from DeploymentStateMap
	State DeploymentState
}

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
	GroupOpen GroupState = iota
	// GroupOrdered is used when state of group is ordered
	GroupOrdered GroupState = iota
	// GroupMatched is used when state of group is matched
	GroupMatched GroupState = iota
	// GroupInsufficientFunds is used when group has insufficient funds
	GroupInsufficientFunds GroupState = iota
	// GroupClosed is used when state of group is closed
	GroupClosed GroupState = iota
)

// GroupFilters defines flags for group list filter
type GroupFilters struct {
	Owner sdk.AccAddress
	// State flag value given
	StateFlagVal string
	// Actual state value decoded from GroupStateMap
	State GroupState
}

// GroupStateMap is used to decode group state flag value
var GroupStateMap = map[string]GroupState{
	"open":         GroupOpen,
	"ordered":      GroupOrdered,
	"matched":      GroupMatched,
	"insufficient": GroupInsufficientFunds,
	"closed":       GroupClosed,
}

// GroupSpec stores group specifications
type GroupSpec struct {
	Name         string      `json:"name"`
	Requirements []tmkv.Pair `json:"requirements"`
	Resources    []Resource  `json:"resources"`
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
func (g GroupSpec) MatchAttributes(attrs []tmkv.Pair) bool {
loop:
	for _, req := range g.Requirements {
		for _, attr := range attrs {
			if bytes.Equal(req.Key, attr.Key) && bytes.Equal(req.Value, attr.Value) {
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
func (d Group) ID() GroupID {
	return d.GroupID
}

// ValidateOrderable method checks whether group opened or not
func (d Group) ValidateOrderable() error {
	switch d.State {
	case GroupOpen:
		return nil
	default:
		return fmt.Errorf("group not open")
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

func (r Resource) FullPrice() sdk.Coin {
	return sdk.NewCoin(r.Price.Denom, r.Price.Amount.MulRaw(int64(r.Count)))
}
