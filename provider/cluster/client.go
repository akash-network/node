package cluster

import (
	"bufio"
	"context"
	"errors"
	cosmostypes "github.com/cosmos/cosmos-sdk/types"
	"io"
	"math/rand"
	"sync"
	"time"

	eventsv1 "k8s.io/api/events/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ovrclk/akash/manifest"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	atypes "github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/unit"
	mquery "github.com/ovrclk/akash/x/market/query"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

var (
	_                         Client = (*nullClient)(nil)
	ErrClearHostnameNoMatches        = errors.New("clearing hostname, no matches")
)

type ReadClient interface {
	LeaseStatus(context.Context, mtypes.LeaseID) (*ctypes.LeaseStatus, error)
	LeaseEvents(context.Context, mtypes.LeaseID, string, bool) (ctypes.EventsWatcher, error)
	LeaseLogs(context.Context, mtypes.LeaseID, string, bool, *int64) ([]*ctypes.ServiceLog, error)
	ServiceStatus(context.Context, mtypes.LeaseID, string) (*ctypes.ServiceStatus, error)
}

// Client interface lease and deployment methods
type Client interface {
	ReadClient
	Deploy(ctx context.Context, lID mtypes.LeaseID, mgroup *manifest.Group, holdHostnames []string) error
	TeardownLease(context.Context, mtypes.LeaseID) error
	Deployments(context.Context) ([]ctypes.Deployment, error)
	Inventory(context.Context) ([]ctypes.Node, error)
	ClearHostname(ctx context.Context, ownerAddress cosmostypes.Address, dseq uint64, hostname string) error
}

type node struct {
	id                    string
	availableResources    atypes.ResourceUnits
	allocateableResources atypes.ResourceUnits
}

// NewNode returns new Node instance with provided details
func NewNode(id string, allocateable atypes.ResourceUnits, available atypes.ResourceUnits) ctypes.Node {
	return &node{id: id, allocateableResources: allocateable, availableResources: available}
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

func (n *node) Allocateable() atypes.ResourceUnits {
	return n.allocateableResources
}

const (
	// 5 CPUs, 5Gi memory for null client.
	nullClientCPU     = 5000
	nullClientMemory  = 32 * unit.Gi
	nullClientStorage = 512 * unit.Gi
)

type nullLease struct {
	ctx    context.Context
	cancel func()
	group  *manifest.Group
}

type nullClient struct {
	leases map[string]*nullLease
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
		leases: make(map[string]*nullLease),
		mtx:    sync.Mutex{},
	}
}

func (c *nullClient) Deploy(ctx context.Context, lid mtypes.LeaseID, mgroup *manifest.Group, holdHostnames []string) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	c.leases[mquery.LeasePath(lid)] = &nullLease{
		ctx:    ctx,
		cancel: cancel,
		group:  mgroup,
	}

	return nil
}

func (c *nullClient) LeaseStatus(_ context.Context, lid mtypes.LeaseID) (*ctypes.LeaseStatus, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	lease, ok := c.leases[mquery.LeasePath(lid)]
	if !ok {
		return nil, nil
	}

	resp := &ctypes.LeaseStatus{}
	resp.Services = make(map[string]*ctypes.ServiceStatus)
	for _, svc := range lease.group.Services {
		resp.Services[svc.Name] = &ctypes.ServiceStatus{
			Name:      svc.Name,
			Available: int32(svc.Count),
			Total:     int32(svc.Count),
		}
	}

	return resp, nil
}

func (c *nullClient) LeaseEvents(ctx context.Context, lid mtypes.LeaseID, _ string, follow bool) (ctypes.EventsWatcher, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	lease, ok := c.leases[mquery.LeasePath(lid)]
	if !ok {
		return nil, nil
	}

	if lease.ctx.Err() != nil {
		return nil, nil
	}

	feed := ctypes.NewEventsFeed(ctx)
	go func() {
		defer feed.Shutdown()

		tm := time.NewTicker(7 * time.Second)
		tm.Stop()

		genEvent := func() *eventsv1.Event {
			return &eventsv1.Event{
				EventTime:           v1.NewMicroTime(time.Now()),
				ReportingController: lease.group.Name,
			}
		}

		nfollowCh := make(chan *eventsv1.Event, 1)
		count := 0
		if !follow {
			count = rand.Intn(9) // nolint: gosec
			nfollowCh <- genEvent()
		} else {
			tm.Reset(time.Second)
		}

		for {
			select {
			case <-lease.ctx.Done():
				return
			case evt := <-nfollowCh:
				if !feed.SendEvent(evt) || count == 0 {
					return
				}
				count--
				nfollowCh <- genEvent()
				break
			case <-tm.C:
				tm.Stop()
				if !feed.SendEvent(genEvent()) {
					return
				}
				tm.Reset(time.Duration(rand.Intn(9)+1) * time.Second) // nolint: gosec
				break
			}
		}
	}()

	return feed, nil
}

func (c *nullClient) ServiceStatus(_ context.Context, _ mtypes.LeaseID, _ string) (*ctypes.ServiceStatus, error) {
	return nil, nil
}

func (c *nullClient) LeaseLogs(_ context.Context, _ mtypes.LeaseID, _ string, _ bool, _ *int64) ([]*ctypes.ServiceLog, error) {
	return nil, nil
}

func (c *nullClient) TeardownLease(_ context.Context, lid mtypes.LeaseID) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if lease, ok := c.leases[mquery.LeasePath(lid)]; ok {
		delete(c.leases, mquery.LeasePath(lid))
		lease.cancel()
	}

	return nil
}

func (c *nullClient) Deployments(context.Context) ([]ctypes.Deployment, error) {
	return nil, nil
}

func (c *nullClient) Inventory(context.Context) ([]ctypes.Node, error) {
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
		},
			atypes.ResourceUnits{
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

func (c *nullClient) ClearHostname(ctx context.Context, ownerAddress cosmostypes.Address, dseq uint64, hostname string) error {
	return nil
}
