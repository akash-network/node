package state

import (
	"math"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
)

type LeaseAdapter interface {
	Save(*types.Lease) error
	Get(types.LeaseID) (*types.Lease, error)
	ForDeployment(deployment base.Bytes) ([]*types.Lease, error)
	All() ([]*types.Lease, error)
}

func NewLeaseAdapter(db DB) LeaseAdapter {
	return &leaseAdapter{db}
}

type leaseAdapter struct {
	db DB
}

func (a *leaseAdapter) Save(obj *types.Lease) error {
	path := a.keyFor(obj.LeaseID)
	return saveObject(a.db, path, obj)
}

func (a *leaseAdapter) Get(id types.LeaseID) (*types.Lease, error) {
	path := a.keyFor(id)
	buf := a.db.Get(path)
	if buf == nil {
		return nil, nil
	}
	obj := new(types.Lease)
	return obj, proto.Unmarshal(buf, obj)
}

func (a *leaseAdapter) keyFor(id types.LeaseID) []byte {
	key := keys.LeaseID(id).Bytes()
	return append([]byte(LeasePath), key...)
}

func (a *leaseAdapter) All() ([]*types.Lease, error) {
	min := a.allMinRange()
	max := a.allMaxRange()
	return a.forRange(min, max)
}

func (a *leaseAdapter) ForDeployment(deployment base.Bytes) ([]*types.Lease, error) {

	min := a.keyFor(types.LeaseID{
		Deployment: deployment,
		Provider:   MinAddress(),
	})

	max := a.keyFor(types.LeaseID{
		Deployment: deployment,
		Group:      math.MaxUint64,
		Order:      math.MaxUint64,
		Provider:   MinAddress(),
	})

	return a.forRange(min, max)
}

func (a *leaseAdapter) allMinRange() []byte {
	return a.keyFor(types.LeaseID{
		Deployment: MinAddress(),
		Provider:   MinAddress(),
	})
}

func (a *leaseAdapter) allMaxRange() []byte {
	return a.keyFor(types.LeaseID{
		Deployment: MaxAddress(),
		Group:      math.MaxUint64,
		Order:      math.MaxUint64,
		Provider:   MaxAddress(),
	})
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
