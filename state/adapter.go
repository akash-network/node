package state

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/tendermint/iavl"
)

const (
	AccountPath = "/accounts/"

	DeploymentPath = "/deployments/"
)

type AccountAdapter interface {
	Save(account *types.Account) error
	Get(base.Bytes) (*types.Account, error)
	KeyFor(base.Bytes) base.Bytes
}

type accountAdapter struct {
	db DB
}

func NewAccountAdapter(db DB) AccountAdapter {
	return &accountAdapter{db}
}

func (a *accountAdapter) Save(account *types.Account) error {
	key := a.KeyFor(account.Address)

	abytes, err := proto.Marshal(account)
	if err != nil {
		return err
	}

	a.db.Set(key, abytes)
	return nil
}

func (a *accountAdapter) Get(address base.Bytes) (*types.Account, error) {

	acc := types.Account{}

	key := a.KeyFor(address)

	buf := a.db.Get(key)
	if buf == nil {
		return nil, nil
	}

	acc.Unmarshal(buf)

	return &acc, nil
}

func (a *accountAdapter) KeyFor(address base.Bytes) base.Bytes {
	return append([]byte(AccountPath), address...)
}

type DeploymentAdapter interface {
	Save(deployment *types.Deployment) error
	Get(base.Bytes) (*types.Deployment, error)
	GetRangeWithProof(base.Bytes, base.Bytes, int) ([][]byte, *types.Deployments, iavl.KeyRangeProof, error)
	KeyFor(base.Bytes) base.Bytes
	String() string
}

type deploymentAdapter struct {
	db DB
}

func NewDeploymentAdapter(db DB) DeploymentAdapter {
	return &deploymentAdapter{db}
}

func (d *deploymentAdapter) Save(deployment *types.Deployment) error {
	key := d.KeyFor(deployment.Address)

	dbytes, err := proto.Marshal(deployment)
	if err != nil {
		return err
	}

	d.db.Set(key, dbytes)
	return nil
}

func (d *deploymentAdapter) Get(address base.Bytes) (*types.Deployment, error) {

	dep := types.Deployment{}

	key := d.KeyFor(address)

	buf := d.db.Get(key)
	if buf == nil {
		return nil, nil
	}

	dep.Unmarshal(buf)

	return &dep, nil
}

func (d *deploymentAdapter) GetRangeWithProof(startKey base.Bytes, endKey base.Bytes, limit int) ([][]byte, *types.Deployments, iavl.KeyRangeProof, error) {
	deps := types.Deployments{}
	proof := iavl.KeyRangeProof{}
	dep := types.Deployment{}

	start := d.KeyFor(startKey)
	end := d.KeyFor(endKey)

	keys, dbytes, proof, err := d.db.GetRangeWithProof(start, end, limit)
	if err != nil {
		return nil, &deps, proof, err
	}
	if keys == nil {
		return nil, &deps, proof, nil
	}

	for _, d := range dbytes {
		dep.Unmarshal(d)
		deps.Deployments = append(deps.Deployments, dep)
	}

	return keys, &deps, proof, nil
}

func (a *deploymentAdapter) KeyFor(address base.Bytes) base.Bytes {
	return append([]byte(DeploymentPath), address...)
}

func (d *deploymentAdapter) String() string {
	return d.db.String()
}
