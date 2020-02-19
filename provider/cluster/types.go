package cluster

import atypes "github.com/ovrclk/akash/types"

type Status struct {
	Leases    uint32
	Inventory InventoryStatus
}

type InventoryStatus struct {
	Active    []atypes.Unit
	Pending   []atypes.Unit
	Available []atypes.Unit
}

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

type LeaseStatus struct {
	Services []*ServiceStatus
}
