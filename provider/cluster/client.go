package cluster

import (
	"bufio"
	"context"
	"errors"
	"io"
	"sync"

	"github.com/ovrclk/akash/manifest"
	atypes "github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/unit"
	mquery "github.com/ovrclk/akash/x/market/query"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

var _ Client = (*nullClient)(nil)

// ErrNoDeployments indicates no deployments exist
var ErrNoDeployments = errors.New("no deployments")

type ReadClient interface {
	LeaseStatus(context.Context, mtypes.LeaseID) (*LeaseStatus, error)
	ServiceStatus(context.Context, mtypes.LeaseID, string) (*ServiceStatus, error)
	ServiceLogs(context.Context, mtypes.LeaseID, string, bool, *int64) ([]*ServiceLog, error)
}

// Client interface lease and deployment methods
type Client interface {
	ReadClient
	Deploy(context.Context, mtypes.LeaseID, *manifest.Group) error
	TeardownLease(context.Context, mtypes.LeaseID) error
	Deployments(context.Context) ([]Deployment, error)
	Inventory(context.Context) ([]Node, error)
}

// Node interface predefined with ID and Available methods
type Node interface {
	ID() string
	Available() atypes.ResourceUnits
	Reserve(atypes.ResourceUnits) error
}

type node struct {
	id                 string
	availableResources atypes.ResourceUnits
}

// NewNode returns new Node instance with provided details
func NewNode(id string, available atypes.ResourceUnits) Node {
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

// Deployment interface defined with LeaseID and ManifestGroup methods
type Deployment interface {
	LeaseID() mtypes.LeaseID
	ManifestGroup() manifest.Group
}

// ServiceLog stores name, stream and scanner
type ServiceLog struct {
	Name    string
	Stream  io.ReadCloser
	Scanner *bufio.Scanner
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
func NewServiceLog(name string, stream io.ReadCloser) *ServiceLog {
	return &ServiceLog{
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

func (c *nullClient) LeaseStatus(ctx context.Context, lid mtypes.LeaseID) (*LeaseStatus, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	mgroup, ok := c.leases[mquery.LeasePath(lid)]
	if !ok {
		return nil, nil
	}

	resp := &LeaseStatus{}
	for _, svc := range mgroup.Services {
		resp.Services = append(resp.Services, &ServiceStatus{
			Name:      svc.Name,
			Available: int32(svc.Count),
			Total:     int32(svc.Count),
		})
	}

	return resp, nil
}

func (c *nullClient) ServiceStatus(ctx context.Context, _ mtypes.LeaseID, _ string) (*ServiceStatus, error) {
	return nil, nil
}

func (c *nullClient) ServiceLogs(_ context.Context, _ mtypes.LeaseID, _ string, _ bool, _ *int64) ([]*ServiceLog, error) {
	return nil, nil
}

func (c *nullClient) TeardownLease(ctx context.Context, lid mtypes.LeaseID) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	delete(c.leases, mquery.LeasePath(lid))
	return nil
}

func (c *nullClient) Deployments(ctx context.Context) ([]Deployment, error) {
	return nil, nil
}

func (c *nullClient) Inventory(ctx context.Context) ([]Node, error) {
	return []Node{
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
