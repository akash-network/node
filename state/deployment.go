package state

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
)

type DeploymentAdapter interface {
	Save(deployment *types.Deployment) error
	Get(base.Bytes) (*types.Deployment, error)
	GetMaxRange() (*types.Deployments, error)
	KeyFor(base.Bytes) base.Bytes
	SequenceFor(base.Bytes) Sequence
}

type deploymentAdapter struct {
	state State
}

func NewDeploymentAdapter(state State) DeploymentAdapter {
	return &deploymentAdapter{state}
}

func (d *deploymentAdapter) Save(deployment *types.Deployment) error {
	key := d.KeyFor(deployment.Address)

	dbytes, err := proto.Marshal(deployment)
	if err != nil {
		return err
	}

	d.state.Set(key, dbytes)
	return nil
}

func (d *deploymentAdapter) Get(address base.Bytes) (*types.Deployment, error) {

	dep := types.Deployment{}

	key := d.KeyFor(address)

	buf := d.state.Get(key)
	if buf == nil {
		return nil, nil
	}

	if err := dep.Unmarshal(buf); err != nil {
		return nil, err
	}

	return &dep, nil
}

func (d *deploymentAdapter) GetMaxRange() (*types.Deployments, error) {
	_, deps, err := d.GetRange(MinAddress(), MaxAddress(), MaxRangeLimit)
	return deps, err
}

func (d *deploymentAdapter) GetRange(startKey base.Bytes, endKey base.Bytes, limit int) ([][]byte, *types.Deployments, error) {
	deps := types.Deployments{}

	start := d.KeyFor(startKey)
	end := d.KeyFor(endKey)

	keys, dbytes, err := d.state.GetRange(start, end, limit)
	if err != nil {
		return nil, &deps, err
	}
	if keys == nil {
		return nil, &deps, nil
	}

	for _, d := range dbytes {
		dep := types.Deployment{}
		if err := dep.Unmarshal(d); err != nil {
			return nil, nil, err
		}
		deps.Items = append(deps.Items, dep)
	}

	return keys, &deps, nil
}

func (a *deploymentAdapter) KeyFor(address base.Bytes) base.Bytes {
	return append([]byte(DeploymentPath), address...)
}

func (a *deploymentAdapter) SequenceFor(address base.Bytes) Sequence {
	path := append([]byte(DeploymentSequencePath), address...)
	return NewSequence(a.state, path)
}
