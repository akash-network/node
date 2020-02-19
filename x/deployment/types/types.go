package types

import (
	"bytes"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/types"
	tmkv "github.com/tendermint/tendermint/libs/kv"
)

type DeploymentState uint8

const (
	DeploymentActive DeploymentState = iota
	DeploymentClosed DeploymentState = iota
)

type Deployment struct {
	DeploymentID `json:"id"`
	State        DeploymentState `json:"state"`
	Version      []byte          `json:"version"`
}

func (obj Deployment) ID() DeploymentID {
	return obj.DeploymentID
}

type GroupState uint8

const (
	GroupOpen              GroupState = iota
	GroupOrdered           GroupState = iota
	GroupMatched           GroupState = iota
	GroupInsufficientFunds GroupState = iota
	GroupClosed            GroupState = iota
)

type GroupSpec struct {
	Name         string      `json:"name"`
	Requirements []tmkv.Pair `json:"requirements"`
	Resources    []Resource  `json:"resources"`
}

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

func (g GroupSpec) GetName() string {
	return g.Name
}

func (g GroupSpec) Price() sdk.Coin {
	var price sdk.Coin
	for idx, resource := range g.Resources {
		if idx == 0 {
			price = resource.Price
			continue
		}
		price = price.Add(resource.Price)
	}
	return price
}

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

type Group struct {
	GroupID   `json:"id"`
	State     GroupState `json:"state"`
	GroupSpec `json:"spec"`
}

func (obj Group) ID() GroupID {
	return obj.GroupID
}

func (d Group) ValidateOrderable() error {
	switch d.State {
	case GroupOpen:
		return nil
	default:
		return fmt.Errorf("group not open")
	}
}

type Resource struct {
	Unit  types.Unit `json:"unit"`
	Count uint32     `json:"count"`
	Price sdk.Coin   `json:"price"`
}

func (r Resource) GetUnit() types.Unit {
	return r.Unit
}
func (r Resource) GetCount() uint32 {
	return r.Count
}
