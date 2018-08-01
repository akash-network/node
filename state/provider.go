package state

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
)

type ProviderAdapter interface {
	Save(provider *types.Provider) error
	Get(base.Bytes) (*types.Provider, error)
	GetMaxRange() (*types.Providers, error)
	GetRange(base.Bytes, base.Bytes, int) ([][]byte, *types.Providers, error)
	KeyFor(base.Bytes) base.Bytes
}

type providerAdapter struct {
	state State
}

func NewProviderAdapter(state State) ProviderAdapter {
	return &providerAdapter{state}
}

func (d *providerAdapter) Save(provider *types.Provider) error {
	key := d.KeyFor(provider.Address)

	dbytes, err := proto.Marshal(provider)
	if err != nil {
		return err
	}

	d.state.Set(key, dbytes)
	return nil
}

func (d *providerAdapter) Get(address base.Bytes) (*types.Provider, error) {

	dc := types.Provider{}
	key := d.KeyFor(address)

	buf := d.state.Get(key)
	if buf == nil {
		return nil, nil
	}
	dc.Unmarshal(buf)

	return &dc, nil
}

func (d *providerAdapter) GetMaxRange() (*types.Providers, error) {
	_, dcs, err := d.GetRange(MinAddress(), MaxAddress(), MaxRangeLimit)
	return dcs, err
}

func (d *providerAdapter) GetRange(startKey base.Bytes, endKey base.Bytes, limit int) ([][]byte, *types.Providers, error) {
	dcs := types.Providers{}
	start := d.KeyFor(startKey)
	end := d.KeyFor(endKey)

	keys, dbytes, err := d.state.GetRange(start, end, limit)
	if err != nil {
		return nil, &dcs, err
	}
	if keys == nil {
		return nil, &dcs, nil
	}

	for _, d := range dbytes {
		dc := &types.Provider{}
		dc.Unmarshal(d)
		dcs.Providers = append(dcs.Providers, dc)
	}

	return keys, &dcs, nil
}

func (a *providerAdapter) KeyFor(address base.Bytes) base.Bytes {
	return append([]byte(ProviderPath), address...)
}
