package cluster

import (
	"bufio"
	"context"
	"io"
	"time"

	eventsv1 "k8s.io/api/events/v1"

	"github.com/ovrclk/akash/manifest"
	atypes "github.com/ovrclk/akash/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

// Status stores current leases and inventory statuses
type Status struct {
	Leases    uint32          `json:"leases"`
	Inventory InventoryStatus `json:"inventory"`
}

// InventoryStatus stores active, pending and available units
type InventoryStatus struct {
	Active    []ResourceUnits `json:"active"`
	Pending   []ResourceUnits `json:"pending"`
	Available []ResourceUnits `json:"available"`
	Error     error           `json:"error"`
}

// ServiceStatus stores the current status of service
type ServiceStatus struct {
	Name      string   `json:"name"`
	Available int32    `json:"available"`
	Total     int32    `json:"total"`
	URIs      []string `json:"uris"`

	ObservedGeneration int64 `json:"observed_generation"`
	Replicas           int32 `json:"replicas"`
	UpdatedReplicas    int32 `json:"updated_replicas"`
	ReadyReplicas      int32 `json:"ready_replicas"`
	AvailableReplicas  int32 `json:"available_replicas"`
}

type ForwardedPortStatus struct {
	Host         string                   `json:"host,omitempty"`
	Port         uint16                   `json:"port"`
	ExternalPort uint16                   `json:"externalPort"`
	Proto        manifest.ServiceProtocol `json:"proto"`
	Available    int32                    `json:"available"`
	Name         string                   `json:"name"`
}

// LeaseStatus includes list of services with their status
type LeaseStatus struct {
	Services       map[string]*ServiceStatus        `json:"services"`
	ForwardedPorts map[string][]ForwardedPortStatus `json:"forwarded_ports"` // Container services that are externally accessible
}

type ResourceUnits struct {
	CPU       *atypes.CPU
	Memory    *atypes.Memory
	Storage   map[string]atypes.Storage
	Endpoints []atypes.Endpoint
}

// Node interface predefined with ID and Available methods
type Node interface {
	ID() string
	Available() ResourceUnits
	Allocateable() ResourceUnits
	Reserve(atypes.ResourceUnits) error
}

type InventoryRead interface {
	Nodes() []Node
}

type Inventory interface {
	InventoryRead
	AddStorageClass(class string)
	AddNode(string, ResourceUnits, ResourceUnits)
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

type LeaseEventObject struct {
	Kind      string `json:"kind" yaml:"kind"`
	Namespace string `json:"namespace" yaml:"namespace"`
	Name      string `json:"name" yaml:"name"`
}

type LeaseEvent struct {
	Type                string           `json:"type" yaml:"type"`
	ReportingController string           `json:"reportingController,omitempty" yaml:"reportingController"`
	ReportingInstance   string           `json:"reportingInstance,omitempty" yaml:"reportingInstance"`
	Time                time.Time        `json:"time" yaml:"time"`
	Reason              string           `json:"reason" yaml:"reason"`
	Note                string           `json:"note" yaml:"note"`
	Object              LeaseEventObject `json:"object" yaml:"object"`
}

type EventsWatcher interface {
	Shutdown()
	Done() <-chan struct{}
	ResultChan() <-chan *eventsv1.Event
	SendEvent(*eventsv1.Event) bool
}

type eventsFeed struct {
	ctx    context.Context
	cancel func()
	feed   chan *eventsv1.Event
}

var _ EventsWatcher = (*eventsFeed)(nil)

func NewEventsFeed(ctx context.Context) EventsWatcher {
	ctx, cancel := context.WithCancel(ctx)
	return &eventsFeed{
		ctx:    ctx,
		cancel: cancel,
		feed:   make(chan *eventsv1.Event),
	}
}

func (e *eventsFeed) Shutdown() {
	e.cancel()
}

func (e *eventsFeed) Done() <-chan struct{} {
	return e.ctx.Done()
}

func (e *eventsFeed) SendEvent(evt *eventsv1.Event) bool {
	select {
	case e.feed <- evt:
		return true
	case <-e.ctx.Done():
		return false
	}
}

func (e *eventsFeed) ResultChan() <-chan *eventsv1.Event {
	return e.feed
}

type ExecResult interface {
	ExitCode() int
}

func (m *ResourceUnits) Sub(rhs atypes.ResourceUnits) (ResourceUnits, error) {
	if (m.CPU == nil && rhs.CPU != nil) ||
		(m.Memory == nil && rhs.Memory != nil) ||
		(m.Storage == nil && rhs.Storage != nil) {
		return ResourceUnits{}, atypes.ErrCannotSub
	}

	// Make a deep copy
	res := ResourceUnits{
		CPU:       &atypes.CPU{},
		Memory:    &atypes.Memory{},
		Storage:   make(map[string]atypes.Storage),
		Endpoints: make([]atypes.Endpoint, len(m.Endpoints)),
	}

	*res.CPU = *m.CPU
	*res.Memory = *m.Memory
	copy(res.Endpoints, m.Endpoints)

	if res.CPU != nil {
		if err := res.CPU.Sub(rhs.CPU); err != nil {
			return ResourceUnits{}, err
		}
	}
	if res.Memory != nil {
		if err := res.Memory.Sub(rhs.Memory); err != nil {
			return ResourceUnits{}, err
		}
	}

	// for _, storage := range rhs.Storage {
	//
	// }

	// if res.Storage != nil {
	// 	if err := res.Storage.sub(rhs.Storage); err != nil {
	// 		return ResourceUnits{}, err
	// 	}
	// }

	return res, nil
}

func (m *ResourceUnits) GetCPU() *atypes.CPU {
	if m != nil {
		return m.CPU
	}
	return nil
}

func (m *ResourceUnits) GetMemory() *atypes.Memory {
	if m != nil {
		return m.Memory
	}
	return nil
}

func (m *ResourceUnits) GetStorage() atypes.Volumes {
	// if m != nil {
	// 	return m.Storage
	// }
	return nil
}

func (m *ResourceUnits) GetEndpoints() atypes.Endpoints {
	if m != nil {
		return m.Endpoints
	}
	return nil
}
