package cluster

import (
	"bufio"
	"io"

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
