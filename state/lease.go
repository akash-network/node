package state

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
)

type LeaseAdapter interface {
	Save(*types.Lease) error
	Get(daddr base.Bytes, group uint64, order uint64, paddr base.Bytes) (*types.Lease, error)
	GetByKey(address base.Bytes) (*types.Lease, error)
	IDFor(obj *types.Lease) []byte
	KeyFor(id []byte) []byte
}

func NewLeaseAdapter(db DB) LeaseAdapter {
	return &leaseAdapter{db}
}

type leaseAdapter struct {
	db DB
}

func (a *leaseAdapter) Save(obj *types.Lease) error {
	path := a.KeyFor(a.IDFor(obj))
	return saveObject(a.db, path, obj)
}

func (a *leaseAdapter) Get(daddr base.Bytes, group uint64, order uint64, paddr base.Bytes) (*types.Lease, error) {
	path := a.KeyFor(LeaseID(daddr, group, order, paddr))
	buf := a.db.Get(path)
	if buf == nil {
		return nil, nil
	}
	obj := new(types.Lease)
	return obj, proto.Unmarshal(buf, obj)
}

func (a *leaseAdapter) GetByKey(address base.Bytes) (*types.Lease, error) {
	ful := types.Lease{}
	key := a.KeyFor(address)
	buf := a.db.Get(key)
	if buf == nil {
		return nil, nil
	}
	return &ful, ful.Unmarshal(buf)
}

// /fulfillment-orders/{deployment-address}{group-sequence}{order-sequence}{provider-address}
func (a *leaseAdapter) KeyFor(id []byte) []byte {
	return append([]byte(LeasePath), id...)
}

// {deployment-address}{group-sequence}{order-sequence}{provider-address}
func (a *leaseAdapter) IDFor(obj *types.Lease) []byte {
	return LeaseID(obj.Deployment, obj.GetGroup(), obj.GetOrder(), obj.Provider)
}
