package types

import (
	"bytes"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/tendermint/tendermint/libs/common"
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
	Name         string          `json:"name"`
	Requirements []common.KVPair `json:"requirements"`
	Resources    []Resource      `json:"resources"`
}

func (g GroupSpec) GetResources() []Resource {
	return g.Resources
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

func (g GroupSpec) MatchAttributes(attrs []common.KVPair) bool {
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
	Unit  Unit     `json:"unit"`
	Count uint32   `json:"count"`
	Price sdk.Coin `json:"price"`
}

type Unit struct {
	CPU     uint32 `json:"cpu"`
	Memory  uint64 `json:"memory"`
	Storage uint64 `json:"storage"`
}
