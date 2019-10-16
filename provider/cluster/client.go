package cluster

import (
	"bufio"
	"context"
	"errors"
	"io"
	"sync"

	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/unit"
)

var ErrNoDeployments = errors.New("no deployments")

type Client interface {
	Deploy(types.LeaseID, *types.ManifestGroup) error
	TeardownLease(types.LeaseID) error
	Deployments() ([]Deployment, error)
	LeaseStatus(types.LeaseID) (*types.LeaseStatusResponse, error)
	ServiceStatus(types.LeaseID, string) (*types.ServiceStatusResponse, error)
	ServiceLogs(context.Context, types.LeaseID, int64, bool) ([]*ServiceLog, error)
	Inventory() ([]Node, error)
}

type Node interface {
	ID() string
	Available() types.ResourceUnit
}

type node struct {
	id        string
	available types.ResourceUnit
}

func NewNode(id string, available types.ResourceUnit) Node {
	return &node{id: id, available: available}
}

func (n *node) ID() string {
	return n.id
}

func (n *node) Available() types.ResourceUnit {
	return n.available
}

type Deployment interface {
	LeaseID() types.LeaseID
	ManifestGroup() *types.ManifestGroup
}

type ServiceLog struct {
	Name    string
	Stream  io.ReadCloser
	Scanner *bufio.Scanner
}

const (
	// 5 CPUs, 5Gi memory for null client.
	nullClientCPU    = 5000
	nullClientMemory = 32 * unit.Gi
	nullClientDisk   = 512 * unit.Gi
)

type nullClient struct {
	leases map[string]*types.ManifestGroup
	mtx    sync.Mutex
}

func NewServiceLog(name string, stream io.ReadCloser) *ServiceLog {
	return &ServiceLog{
		Name:    name,
		Stream:  stream,
		Scanner: bufio.NewScanner(stream),
	}
}

func NullClient() Client {
	return &nullClient{
		leases: make(map[string]*types.ManifestGroup),
		mtx:    sync.Mutex{},
	}
}

func (c *nullClient) Deploy(lid types.LeaseID, mgroup *types.ManifestGroup) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.leases[lid.String()] = mgroup
	return nil
}

func (c *nullClient) LeaseStatus(lid types.LeaseID) (*types.LeaseStatusResponse, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	mgroup, ok := c.leases[lid.String()]
	if !ok {
		return nil, nil
	}

	resp := &types.LeaseStatusResponse{}
	for _, svc := range mgroup.Services {
		resp.Services = append(resp.Services, &types.ServiceStatus{
			Name:      svc.Name,
			Available: int32(svc.Count),
			Total:     int32(svc.Count),
		})
	}

	return resp, nil
}

func (c *nullClient) ServiceStatus(_ types.LeaseID, _ string) (*types.ServiceStatusResponse, error) {
	return nil, nil
}

func (c *nullClient) ServiceLogs(_ context.Context, _ types.LeaseID, _ int64, _ bool) ([]*ServiceLog, error) {
	return nil, nil
}

func (c *nullClient) TeardownLease(lid types.LeaseID) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	delete(c.leases, lid.String())
	return nil
}

func (c *nullClient) Deployments() ([]Deployment, error) {
	return nil, nil
}

func (c *nullClient) Inventory() ([]Node, error) {
	return []Node{
		NewNode("solo", types.ResourceUnit{
			CPU:    nullClientCPU,
			Memory: nullClientMemory,
			Disk:   nullClientDisk,
		}),
	}, nil
}
