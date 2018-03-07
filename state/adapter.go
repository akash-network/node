package state

import (
	"math"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/tendermint/iavl"
)

const (
	AccountPath         = "/accounts/"
	DeploymentPath      = "/deployments/"
	DatacenterPath      = "/datacenters/"
	DeploymentOrderPath = "/deploymentorders/"

	MaxRangeLimit = math.MaxInt64
)

func GetMinStartRange() base.Bytes {
	minStartRange := new(base.Bytes)
	minStartRange.DecodeString("")
	return *minStartRange
}

func GetMaxEndRange64() base.Bytes {
	maxEndRange64 := new(base.Bytes)
	maxEndRange64.DecodeString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
	return *maxEndRange64
}

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
	GetMaxRange() (*types.Deployments, error)
	GetRangeWithProof(base.Bytes, base.Bytes, int) ([][]byte, *types.Deployments, iavl.KeyRangeProof, error)
	KeyFor(base.Bytes) base.Bytes
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

func (d *deploymentAdapter) GetMaxRange() (*types.Deployments, error) {
	_, deps, _, err := d.GetRangeWithProof(GetMinStartRange(), GetMaxEndRange64(), MaxRangeLimit)
	return deps, err
}

func (d *deploymentAdapter) GetRangeWithProof(startKey base.Bytes, endKey base.Bytes, limit int) ([][]byte, *types.Deployments, iavl.KeyRangeProof, error) {
	deps := types.Deployments{}
	proof := iavl.KeyRangeProof{}

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
		dep := types.Deployment{}
		dep.Unmarshal(d)
		deps.Deployments = append(deps.Deployments, dep)
	}

	return keys, &deps, proof, nil
}

func (a *deploymentAdapter) KeyFor(address base.Bytes) base.Bytes {
	return append([]byte(DeploymentPath), address...)
}

type DatacenterAdapter interface {
	Save(datacenter *types.Datacenter) error
	Get(base.Bytes) (*types.Datacenter, error)
	GetMaxRange() (*types.Datacenters, error)
	GetRangeWithProof(base.Bytes, base.Bytes, int) ([][]byte, *types.Datacenters, iavl.KeyRangeProof, error)
	KeyFor(base.Bytes) base.Bytes
}

type datacenterAdapter struct {
	db DB
}

func NewDatacenterAdapter(db DB) DatacenterAdapter {
	return &datacenterAdapter{db}
}

func (d *datacenterAdapter) Save(datacenter *types.Datacenter) error {
	key := d.KeyFor(datacenter.Address)

	dbytes, err := proto.Marshal(datacenter)
	if err != nil {
		return err
	}

	d.db.Set(key, dbytes)
	return nil
}

func (d *datacenterAdapter) Get(address base.Bytes) (*types.Datacenter, error) {

	dc := types.Datacenter{}
	key := d.KeyFor(address)

	buf := d.db.Get(key)
	if buf == nil {
		return nil, nil
	}
	dc.Unmarshal(buf)

	return &dc, nil
}

func (d *datacenterAdapter) GetMaxRange() (*types.Datacenters, error) {
	_, dcs, _, err := d.GetRangeWithProof(GetMinStartRange(), GetMaxEndRange64(), MaxRangeLimit)
	return dcs, err
}

func (d *datacenterAdapter) GetRangeWithProof(startKey base.Bytes, endKey base.Bytes, limit int) ([][]byte, *types.Datacenters, iavl.KeyRangeProof, error) {
	dcs := types.Datacenters{}
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
		dc := types.Datacenter{}
		dc.Unmarshal(d)
		dcs.Datacenters = append(dcs.Datacenters, dc)
	}

	return keys, &dcs, proof, nil
}

func (a *datacenterAdapter) KeyFor(address base.Bytes) base.Bytes {
	return append([]byte(DatacenterPath), address...)
}

type DeploymentOrderAdapter interface {
	Save(deploymentOrder *types.DeploymentOrder) error
	Get(base.Bytes) (*types.DeploymentOrder, error)
	GetMaxRange() (*types.DeploymentOrders, error)
	GetRangeWithProof(base.Bytes, base.Bytes, int) ([][]byte, *types.DeploymentOrders, iavl.KeyRangeProof, error)
	KeyFor(base.Bytes) base.Bytes
}

type deploymentOrderAdapter struct {
	db DB
}

func NewDeploymentOrderAdapter(db DB) DeploymentOrderAdapter {
	return &deploymentOrderAdapter{db}
}

func (d *deploymentOrderAdapter) Save(deploymentOrder *types.DeploymentOrder) error {
	key := d.KeyFor(deploymentOrder.Address)
	dbytes, err := proto.Marshal(deploymentOrder)
	if err != nil {
		return err
	}

	d.db.Set(key, dbytes)
	return nil
}

func (d *deploymentOrderAdapter) Get(address base.Bytes) (*types.DeploymentOrder, error) {

	depo := types.DeploymentOrder{}

	key := d.KeyFor(address)

	buf := d.db.Get(key)
	if buf == nil {
		return nil, nil
	}

	depo.Unmarshal(buf)

	return &depo, nil
}

func (d *deploymentOrderAdapter) GetMaxRange() (*types.DeploymentOrders, error) {
	_, depos, _, err := d.GetRangeWithProof(GetMinStartRange(), GetMaxEndRange64(), MaxRangeLimit)
	return depos, err
}

func (d *deploymentOrderAdapter) GetRangeWithProof(startKey base.Bytes, endKey base.Bytes, limit int) ([][]byte, *types.DeploymentOrders, iavl.KeyRangeProof, error) {
	depos := types.DeploymentOrders{}
	proof := iavl.KeyRangeProof{}

	start := d.KeyFor(startKey)
	end := d.KeyFor(endKey)

	keys, dbytes, proof, err := d.db.GetRangeWithProof(start, end, limit)
	if err != nil {
		return nil, &depos, proof, err
	}
	if keys == nil {
		return nil, &depos, proof, nil
	}

	for _, d := range dbytes {
		depo := types.DeploymentOrder{}
		depo.Unmarshal(d)
		depos.DeploymentOrders = append(depos.DeploymentOrders, depo)
	}

	return keys, &depos, proof, nil
}

func (a *deploymentOrderAdapter) KeyFor(address base.Bytes) base.Bytes {
	return append([]byte(DeploymentOrderPath), address...)
}
