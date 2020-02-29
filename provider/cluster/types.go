package cluster

import atypes "github.com/ovrclk/akash/types"

// Status stores current leases and inventory statuses
type Status struct {
	Leases    uint32
	Inventory InventoryStatus
}

// InventoryStatus stores active, pending and available units
type InventoryStatus struct {
	Active    []atypes.Unit
	Pending   []atypes.Unit
	Available []atypes.Unit
}

// ServiceStatus stores the current status of service
type ServiceStatus struct {
	Name      string
	Available int32
	Total     int32
	URIs      []string

	ObservedGeneration int64
	Replicas           int32
	UpdatedReplicas    int32
	ReadyReplicas      int32
	AvailableReplicas  int32
}

// LeaseStatus includes list of services with their status
type LeaseStatus struct {
	Services []*ServiceStatus
}
