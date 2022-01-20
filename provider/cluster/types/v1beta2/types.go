package v1beta2

import (
	"bufio"
	"context"
	"io"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	eventsv1 "k8s.io/api/events/v1"

	manifest "github.com/ovrclk/akash/manifest/v2beta1"
	types "github.com/ovrclk/akash/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"

	"github.com/ovrclk/akash/sdl"
)

var (
	// ErrInsufficientCapacity is the new error when capacity is insufficient
	ErrInsufficientCapacity = errors.New("insufficient capacity")
)

// Status stores current leases and inventory statuses
type Status struct {
	Leases    uint32          `json:"leases"`
	Inventory InventoryStatus `json:"inventory"`
}

type InventoryMetricTotal struct {
	CPU              uint64           `json:"cpu"`
	Memory           uint64           `json:"memory"`
	StorageEphemeral uint64           `json:"storage_ephemeral"`
	Storage          map[string]int64 `json:"storage,omitempty"`
}

type InventoryStorageStatus struct {
	Class string `json:"class"`
	Size  int64  `json:"size"`
}

// InventoryStatus stores active, pending and available units
type InventoryStatus struct {
	Active    []InventoryMetricTotal `json:"active,omitempty"`
	Pending   []InventoryMetricTotal `json:"pending,omitempty"`
	Available struct {
		Nodes   []InventoryNodeMetric    `json:"nodes,omitempty"`
		Storage []InventoryStorageStatus `json:"storage,omitempty"`
	} `json:"available,omitempty"`
	Error error `json:"error,omitempty"`
}

type InventoryNodeMetric struct {
	CPU              uint64 `json:"cpu"`
	Memory           uint64 `json:"memory"`
	StorageEphemeral uint64 `json:"storage_ephemeral"`
}

func (inv *InventoryMetricTotal) AddResources(res types.Resources) {
	cpu := sdk.NewIntFromUint64(inv.CPU)
	mem := sdk.NewIntFromUint64(inv.Memory)
	ephemeralStorage := sdk.NewIntFromUint64(inv.StorageEphemeral)

	if res.Resources.CPU != nil {
		cpu = cpu.Add(res.Resources.CPU.Units.Val.MulRaw(int64(res.Count)))
	}

	if res.Resources.Memory != nil {
		mem = mem.Add(res.Resources.Memory.Quantity.Val.MulRaw(int64(res.Count)))
	}

	for _, storage := range res.Resources.Storage {
		if storageClass, found := storage.Attributes.Find(sdl.StorageAttributeClass).AsString(); !found {
			ephemeralStorage = ephemeralStorage.Add(storage.Quantity.Val.MulRaw(int64(res.Count)))
		} else {
			val := sdk.NewIntFromUint64(uint64(inv.Storage[storageClass]))
			val = val.Add(storage.Quantity.Val.MulRaw(int64(res.Count)))
			inv.Storage[storageClass] = val.Int64()
		}
	}

	inv.CPU = cpu.Uint64()
	inv.Memory = mem.Uint64()
	inv.StorageEphemeral = ephemeralStorage.Uint64()
}

type InventoryNode struct {
	Name        string              `json:"name"`
	Allocatable InventoryNodeMetric `json:"allocatable"`
	Available   InventoryNodeMetric `json:"available"`
}

type InventoryMetrics struct {
	Nodes            []InventoryNode      `json:"nodes"`
	TotalAllocatable InventoryMetricTotal `json:"total_allocatable"`
	TotalAvailable   InventoryMetricTotal `json:"total_available"`
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
	Name         string                   `json:"name"`
}

// LeaseStatus includes list of services with their status
type LeaseStatus struct {
	Services       map[string]*ServiceStatus        `json:"services"`
	ForwardedPorts map[string][]ForwardedPortStatus `json:"forwarded_ports"` // Container services that are externally accessible
}

type Inventory interface {
	Adjust(Reservation) error
	Metrics() InventoryMetrics
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
