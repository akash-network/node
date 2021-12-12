package v1beta1

import (
	"bufio"
	"context"
	"io"

	eventsv1 "k8s.io/api/events/v1"

	manifest "github.com/ovrclk/akash/manifest/v1"
	atypes "github.com/ovrclk/akash/types/v1beta1"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta1"
)

// Status stores current leases and inventory statuses
type Status struct {
	Leases    uint32          `json:"leases"`
	Inventory InventoryStatus `json:"inventory"`
}

// InventoryStatus stores active, pending and available units
type InventoryStatus struct {
	Active    []atypes.ResourceUnits `json:"active"`
	Pending   []atypes.ResourceUnits `json:"pending"`
	Available []atypes.ResourceUnits `json:"available"`
	Error     error                  `json:"error"`
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

// Node interface predefined with ID and Available methods
type Node interface {
	ID() string
	Available() atypes.ResourceUnits
	Allocateable() atypes.ResourceUnits
	Reserve(atypes.ResourceUnits) error
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
	Reason              string           `json:"reason" yaml:"reason"`
	Note                string           `json:"note" yaml:"note"`
	Object              LeaseEventObject `json:"object" yaml:"object"`
}

// EventsWatcher
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
