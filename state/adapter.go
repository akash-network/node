package state

import (
	"bytes"
	"math"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/tendermint/iavl"
)

const (
	AccountPath = "/accounts/"

	DeploymentPath         = "/deployments/"
	DeploymentSequencePath = "/deployments-seq/"

	DeploymentGroupPath  = "/deployment-groups/"
	DatacenterPath       = "/datacenters/"
	DeploymentOrderPath  = "/deploymentorders/"
	FulfillmentOrderPath = "/fulfillment-orders/"
	LeasePath            = "/lease/"

	MaxRangeLimit = math.MaxInt64

	addressSize = 32 // XXX: check
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

func MaxAddress() []byte {
	return bytes.Repeat([]byte{0xff}, addressSize)
}

func MinAddress() []byte {
	return make([]byte, addressSize)
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
	return saveObject(a.db, key, account)
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
	SequenceFor(base.Bytes) Sequence
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
		deps.Items = append(deps.Items, dep)
	}

	return keys, &deps, proof, nil
}

func (a *deploymentAdapter) KeyFor(address base.Bytes) base.Bytes {
	return append([]byte(DeploymentPath), address...)
}

func (a *deploymentAdapter) SequenceFor(address base.Bytes) Sequence {
	path := append([]byte(DeploymentSequencePath), address...)
	return NewSequence(a.db, path)
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
