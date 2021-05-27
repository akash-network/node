package provider

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/provider/bidengine"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

type Config struct {
	ClusterWaitReadyDuration        time.Duration
	ClusterPublicHostname           string
	ClusterExternalPortQuantity     uint
	InventoryResourcePollPeriod     time.Duration
	InventoryResourceDebugFrequency uint
	BidPricingStrategy              bidengine.BidPricingStrategy
	BidDeposit                      sdk.Coin
	CPUCommitLevel                  float64
	MemoryCommitLevel               float64
	StorageCommitLevel              float64
	BlockedHostnames                []string
	DeploymentIngressStaticHosts    bool
	BidTimeout                      time.Duration
	ManifestTimeout                 time.Duration
}

func NewDefaultConfig() Config {
	return Config{
		ClusterWaitReadyDuration: time.Second * 5,
		BidDeposit:               mtypes.DefaultBidMinDeposit,
	}
}
