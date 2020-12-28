package cluster

import (
	"bufio"
	"context"
	"io"
	"sync"

	"github.com/ovrclk/akash/manifest"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	atypes "github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/unit"
	mquery "github.com/ovrclk/akash/x/market/query"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

var _ Client = (*nullClient)(nil)

type ReadClient interface {
	LeaseStatus(context.Context, mtypes.LeaseID) (*ctypes.LeaseStatus, error)
	ServiceStatus(context.Context, mtypes.LeaseID, string) (*ctypes.ServiceStatus, error)
	ServiceLogs(context.Context, mtypes.LeaseID, string, bool, *int64) ([]*ctypes.ServiceLog, error)
}

// Client interface lease and deployment methods
type Client interface {
	ReadClient
	Deploy(context.Context, mtypes.LeaseID, *manifest.Group) error
	TeardownLease(context.Context, mtypes.LeaseID) error
	Deployments(context.Context) ([]ctypes.Deployment, error)
	Inventory(context.Context) ([]ctypes.Node, error)
}

type node struct {
	id                 string
	availableResources atypes.ResourceUnits
}

// NewNode returns new Node instance with provided details
func NewNode(id string, available atypes.ResourceUnits) ctypes.Node {
	return &node{id: id, availableResources: available}
}

// ID returns id of node
func (n *node) ID() string {
	return n.id
}

func (n *node) Reserve(atypes.ResourceUnits) error {
	return nil
}

// Available returns available units of node
func (n *node) Available() atypes.ResourceUnits {
	return n.availableResources
}

const (
	// 5 CPUs, 5Gi memory for null client.
	nullClientCPU     = 5000
	nullClientMemory  = 32 * unit.Gi
	nullClientStorage = 512 * unit.Gi
)

type nullClient struct {
	leases map[string]*manifest.Group
	mtx    sync.Mutex
}

// NewServiceLog creates and returns a service log with provided details
func NewServiceLog(name string, stream io.ReadCloser) *ctypes.ServiceLog {
	return &ctypes.ServiceLog{
		Name:    name,
		Stream:  stream,
		Scanner: bufio.NewScanner(stream),
	}
}

// NullClient returns nullClient instance
func NullClient() Client {
	return &nullClient{
		leases: make(map[string]*manifest.Group),
		mtx:    sync.Mutex{},
	}
}

func (c *nullClient) Deploy(ctx context.Context, lid mtypes.LeaseID, mgroup *manifest.Group) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.leases[mquery.LeasePath(lid)] = mgroup
	return nil
}

func (c *nullClient) LeaseStatus(ctx context.Context, lid mtypes.LeaseID) (*ctypes.LeaseStatus, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	mgroup, ok := c.leases[mquery.LeasePath(lid)]
	if !ok {
		return nil, nil
	}

	resp := &ctypes.LeaseStatus{}
	resp.Services = make(map[string]*ctypes.ServiceStatus)
	for _, svc := range mgroup.Services {
		resp.Services[svc.Name] = &ctypes.ServiceStatus{
			Name:      svc.Name,
			Available: int32(svc.Count),
			Total:     int32(svc.Count),
		}
	}

	return resp, nil
}

func (c *nullClient) ServiceStatus(ctx context.Context, _ mtypes.LeaseID, _ string) (*ctypes.ServiceStatus, error) {
	return nil, nil
}

func (c *nullClient) ServiceLogs(_ context.Context, _ mtypes.LeaseID, _ string, _ bool, _ *int64) ([]*ctypes.ServiceLog, error) {
	return nil, nil
}

func (c *nullClient) TeardownLease(ctx context.Context, lid mtypes.LeaseID) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	delete(c.leases, mquery.LeasePath(lid))
	return nil
}

func (c *nullClient) Deployments(ctx context.Context) ([]ctypes.Deployment, error) {
	return nil, nil
}

func (c *nullClient) Inventory(ctx context.Context) ([]ctypes.Node, error) {
	return []ctypes.Node{
		NewNode("solo", atypes.ResourceUnits{
			CPU: &atypes.CPU{
				Units: atypes.NewResourceValue(nullClientCPU),
			},
			Memory: &atypes.Memory{
				Quantity: atypes.NewResourceValue(nullClientMemory),
			},
			Storage: &atypes.Storage{
				Quantity: atypes.NewResourceValue(nullClientStorage),
			},
		}),
	}, nil
}
