package state

import (
	"math"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
)

type FulfillmentAdapter interface {
	Save(*types.Fulfillment) error
	Get(id types.FulfillmentID) (*types.Fulfillment, error)
	ForDeployment(base.Bytes) ([]*types.Fulfillment, error)
	ForGroup(types.DeploymentGroupID) ([]*types.Fulfillment, error)
	ForOrder(types.OrderID) ([]*types.Fulfillment, error)
}

func NewFulfillmentAdapter(db DB) FulfillmentAdapter {
	return &fulfillmentAdapter{db}
}

type fulfillmentAdapter struct {
	db DB
}

func (a *fulfillmentAdapter) Save(obj *types.Fulfillment) error {
	path := a.keyFor(obj.FulfillmentID)
	return saveObject(a.db, path, obj)
}

func (a *fulfillmentAdapter) Get(id types.FulfillmentID) (*types.Fulfillment, error) {
	path := a.keyFor(id)
	buf := a.db.Get(path)
	if buf == nil {
		return nil, nil
	}
	obj := new(types.Fulfillment)
	return obj, proto.Unmarshal(buf, obj)
}

func (a *fulfillmentAdapter) ForDeployment(deployment base.Bytes) ([]*types.Fulfillment, error) {
	min := a.deploymentMinRange(deployment)
	max := a.deploymentMaxRange(deployment)
	return a.forRange(min, max)
}

func (a *fulfillmentAdapter) ForGroup(id types.DeploymentGroupID) ([]*types.Fulfillment, error) {
	min := a.groupMinRange(id)
	max := a.groupMaxRange(id)
	return a.forRange(min, max)
}

func (a *fulfillmentAdapter) ForOrder(order types.OrderID) ([]*types.Fulfillment, error) {
	min := a.orderMinRange(order)
	max := a.orderMaxRange(order)
	return a.forRange(min, max)
}

func (a *fulfillmentAdapter) keyFor(id types.FulfillmentID) []byte {
	path := keys.FulfillmentID(id).Bytes()
	return append([]byte(FulfillmentPath), path...)
}

func (a *fulfillmentAdapter) deploymentMinRange(deployment base.Bytes) []byte {
	return a.keyFor(types.FulfillmentID{
		Deployment: deployment,
	})
}

func (a *fulfillmentAdapter) deploymentMaxRange(deployment base.Bytes) []byte {
	return a.keyFor(types.FulfillmentID{
		Deployment: deployment,
		Group:      math.MaxUint64,
		Order:      math.MaxUint64,
		Provider:   MaxAddress(),
	})
}

func (a *fulfillmentAdapter) groupMinRange(id types.DeploymentGroupID) []byte {
	return a.keyFor(types.FulfillmentID{
		Deployment: id.Deployment,
		Group:      id.Seq,
	})
}

func (a *fulfillmentAdapter) groupMaxRange(id types.DeploymentGroupID) []byte {
	return a.keyFor(types.FulfillmentID{
		Deployment: id.Deployment,
		Group:      id.Seq,
		Order:      math.MaxUint64,
		Provider:   MaxAddress(),
	})
}

func (a *fulfillmentAdapter) orderMinRange(id types.OrderID) []byte {
	return a.keyFor(types.FulfillmentID{
		Deployment: id.Deployment,
		Group:      id.Group,
		Order:      id.Seq,
	})
}

func (a *fulfillmentAdapter) orderMaxRange(id types.OrderID) []byte {
	return a.keyFor(types.FulfillmentID{
		Deployment: id.Deployment,
		Group:      id.Group,
		Order:      id.Seq,
		Provider:   MaxAddress(),
	})
}

func (a *fulfillmentAdapter) forRange(min, max []byte) ([]*types.Fulfillment, error) {
	_, bufs, _, err := a.db.GetRangeWithProof(min, max, MaxRangeLimit)
	if err != nil {
		return nil, err
	}

	var items []*types.Fulfillment

	for _, buf := range bufs {
		item := &types.Fulfillment{}
		if err := item.Unmarshal(buf); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
