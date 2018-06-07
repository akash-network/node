package state

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/tendermint/iavl"
)

type ProviderAdapter interface {
	Save(provider *types.Provider) error
	Get(base.Bytes) (*types.Provider, error)
	GetMaxRange() (*types.Providers, error)
	GetRangeWithProof(base.Bytes, base.Bytes, int) ([][]byte, *types.Providers, iavl.KeyRangeProof, error)
	KeyFor(base.Bytes) base.Bytes
}

type providerAdapter struct {
	db DB
}

func NewProviderAdapter(db DB) ProviderAdapter {
	return &providerAdapter{db}
}

func (d *providerAdapter) Save(provider *types.Provider) error {
	key := d.KeyFor(provider.Address)

	dbytes, err := proto.Marshal(provider)
	if err != nil {
		return err
	}

	d.db.Set(key, dbytes)
	return nil
}

func (d *providerAdapter) Get(address base.Bytes) (*types.Provider, error) {

	dc := types.Provider{}
	key := d.KeyFor(address)

	buf := d.db.Get(key)
	if buf == nil {
		return nil, nil
	}
	dc.Unmarshal(buf)

	return &dc, nil
}

func (d *providerAdapter) GetMaxRange() (*types.Providers, error) {
	_, dcs, _, err := d.GetRangeWithProof(MinAddress(), MaxAddress(), MaxRangeLimit)
	return dcs, err
}

func (d *providerAdapter) GetRangeWithProof(startKey base.Bytes, endKey base.Bytes, limit int) ([][]byte, *types.Providers, iavl.KeyRangeProof, error) {
	dcs := types.Providers{}
	proof := iavl.KeyRangeProof{}
	start := d.KeyFor(startKey)
	end := d.KeyFor(endKey)

	keys, dbytes, proof, err := d.db.GetRangeWithProof(start, end, limit)
	if err != nil {
		return nil, &dcs, proof, err
	}
	if keys == nil {
		return nil, &dcs, proof, nil
	}

	for _, d := range dbytes {
		dc := types.Provider{}
		dc.Unmarshal(d)
		dcs.Providers = append(dcs.Providers, dc)
	}

	return keys, &dcs, proof, nil
}

func (a *providerAdapter) KeyFor(address base.Bytes) base.Bytes {
	return append([]byte(ProviderPath), address...)
}
