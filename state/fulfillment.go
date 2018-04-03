package state

import (
	"math"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
)

type FulfillmentAdapter interface {
	Save(*types.Fulfillment) error
	Get(daddr base.Bytes, group uint64, order uint64, paddr base.Bytes) (*types.Fulfillment, error)
	GetByKey(address base.Bytes) (*types.Fulfillment, error)
	ForDeployment(base.Bytes) ([]*types.Fulfillment, error)
	ForGroup(*types.DeploymentGroup) ([]*types.Fulfillment, error)
	ForOrder(*types.Order) ([]*types.Fulfillment, error)
	IDFor(*types.Fulfillment) []byte
	KeyFor(id []byte) []byte
}

func NewFulfillmentAdapter(db DB) FulfillmentAdapter {
	return &fulfillmentAdapter{db}
}

type fulfillmentAdapter struct {
	db DB
}

func (a *fulfillmentAdapter) Save(obj *types.Fulfillment) error {
	path := a.KeyFor(a.IDFor(obj))
	return saveObject(a.db, path, obj)
}

func (a *fulfillmentAdapter) Get(daddr base.Bytes, group uint64, order uint64, paddr base.Bytes) (*types.Fulfillment, error) {
	path := a.KeyFor(FulfillmentID(daddr, group, order, paddr))
	buf := a.db.Get(path)
	if buf == nil {
		return nil, nil
	}
	obj := new(types.Fulfillment)
	return obj, proto.Unmarshal(buf, obj)
}

func (a *fulfillmentAdapter) GetByKey(address base.Bytes) (*types.Fulfillment, error) {
	ful := types.Fulfillment{}
	key := a.KeyFor(address)
	buf := a.db.Get(key)
	if buf == nil {
		return nil, nil
	}
	return &ful, ful.Unmarshal(buf)
}

func (a *fulfillmentAdapter) ForDeployment(deployment base.Bytes) ([]*types.Fulfillment, error) {
	min := a.deploymentMinRange(deployment)
	max := a.deploymentMaxRange(deployment)
	return a.forRange(min, max)
}

func (a *fulfillmentAdapter) ForGroup(group *types.DeploymentGroup) ([]*types.Fulfillment, error) {
	min := a.groupMinRange(group)
	max := a.groupMaxRange(group)
	return a.forRange(min, max)
}

func (a *fulfillmentAdapter) ForOrder(order *types.Order) ([]*types.Fulfillment, error) {
	min := a.orderMinRange(order)
	max := a.orderMaxRange(order)
	return a.forRange(min, max)
}

// /fulfillment-orders/{deployment-address}{group-sequence}{order-sequence}{provider-address}
func (a *fulfillmentAdapter) KeyFor(id []byte) []byte {
	return append([]byte(FulfillmentPath), id...)
}

// {deployment-address}{group-sequence}{order-sequence}{provider-address}
func (a *fulfillmentAdapter) IDFor(obj *types.Fulfillment) []byte {
	return FulfillmentID(obj.Deployment, obj.GetGroup(), obj.GetOrder(), obj.Provider)
}

// /fulfillment-orders/{deployment-address}{group-sequence}{order-sequence}
func (a *fulfillmentAdapter) deploymentMinRange(deployment base.Bytes) []byte {
	return a.KeyFor(FulfillmentID(deployment, 0, 0, []byte{}))
}

// /fulfillment-orders/{deployment-address}{group-sequence}{max-order-sequence}{max-address}
func (a *fulfillmentAdapter) deploymentMaxRange(deployment base.Bytes) []byte {
	return a.KeyFor(FulfillmentID(deployment, math.MaxUint64, math.MaxUint64, MaxAddress()))
}

// /fulfillment-orders/{deployment-address}{group-sequence}{order-sequence}
func (a *fulfillmentAdapter) groupMinRange(group *types.DeploymentGroup) []byte {
	return a.KeyFor(FulfillmentID(group.Deployment, group.GetSeq(), 0, []byte{}))
}

// /fulfillment-orders/{deployment-address}{group-sequence}{max-order-sequence}{max-address}
func (a *fulfillmentAdapter) groupMaxRange(group *types.DeploymentGroup) []byte {
	return a.KeyFor(FulfillmentID(group.Deployment, group.GetSeq(), math.MaxUint64, MaxAddress()))
}

// /fulfillment-orders/{deployment-address}{group-sequence}{order-sequence}
func (a *fulfillmentAdapter) orderMinRange(order *types.Order) []byte {
	return a.KeyFor(FulfillmentID(order.Deployment, order.GetGroup(), order.GetSeq(), []byte{}))
}

// /fulfillment-orders/{deployment-address}{group-sequence}{order-sequence}{max-address}
func (a *fulfillmentAdapter) orderMaxRange(order *types.Order) []byte {
	return a.KeyFor(FulfillmentID(order.Deployment, order.GetGroup(), order.GetSeq(), MaxAddress()))
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
