package cluster

import atypes "github.com/ovrclk/akash/types"

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

	ObservedGeneration int64 `json:"observed-generation"`
	Replicas           int32 `json:"replicas"`
	UpdatedReplicas    int32 `json:"updated-replicas"`
	ReadyReplicas      int32 `json:"ready-replicas"`
	AvailableReplicas  int32 `json:"available-replicas"`
}

// LeaseStatus includes list of services with their status
type LeaseStatus struct {
	Services []*ServiceStatus `json:"services"`
}
