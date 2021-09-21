package cluster

import "time"

type Config struct {
	InventoryResourcePollPeriod     time.Duration
	InventoryResourceDebugFrequency uint
	InventoryExternalPortQuantity   uint
	CPUCommitLevel                  float64
	MemoryCommitLevel               float64
	StorageCommitLevel              float64
	BlockedHostnames                []string
	DeploymentIngressStaticHosts    bool
	DeploymentIngressDomain         string
	ClusterSettings                 map[interface{}]interface{}
}

func NewDefaultConfig() Config {
	return Config{
		InventoryResourcePollPeriod:     time.Second * 5,
		InventoryResourceDebugFrequency: 10,
	}
}
