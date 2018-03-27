package state

import (
	"math"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/tendermint/iavl"
)

type LeaseAdapter interface {
	Save(*types.Lease) error
	Get(daddr base.Bytes, group uint64, order uint64, paddr base.Bytes) (*types.Lease, error)
	GetByKey(address base.Bytes) (*types.Lease, error)
	GetMaxRange() ([]*types.Lease, error)
	GetRangeWithProof(base.Bytes, base.Bytes, int) ([][]byte, []*types.Lease, iavl.KeyRangeProof, error)
	IDFor(obj *types.Lease) []byte
	KeyFor(id []byte) []byte
	All() ([]*types.Lease, error)
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

// /lease/{deployment-address}{group-sequence}{order-sequence}{provider-address}
func (a *leaseAdapter) KeyFor(id []byte) []byte {
	return append([]byte(LeasePath), id...)
}

func (a *leaseAdapter) All() ([]*types.Lease, error) {
	min := a.allMinRange()
	max := a.allMaxRange()
	return a.forRange(min, max)
}

// {deployment-address}{group-sequence}{order-sequence}{provider-address}
func (a *leaseAdapter) IDFor(obj *types.Lease) []byte {
	return LeaseID(obj.Deployment, obj.GetGroup(), obj.GetOrder(), obj.Provider)
}

func (a *leaseAdapter) allMinRange() []byte {
	return a.KeyFor(LeaseID(MinAddress(), 0, 0, MinAddress()))
}

func (a *leaseAdapter) allMaxRange() []byte {
	return a.KeyFor(LeaseID(MaxAddress(), math.MaxUint64, math.MaxUint64, MaxAddress()))
}

func (a *leaseAdapter) forRange(min, max []byte) ([]*types.Lease, error) {
	_, bufs, _, err := a.db.GetRangeWithProof(min, max, MaxRangeLimit)
	if err != nil {
		return nil, err
	}

	var items []*types.Lease

	for _, buf := range bufs {
		item := &types.Lease{}
		if err := item.Unmarshal(buf); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
