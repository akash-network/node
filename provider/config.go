package provider

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/provider/bidengine"
	types "github.com/ovrclk/akash/types/v1beta2"
	"github.com/ovrclk/akash/validation/constants"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
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
	MaxGroupVolumes                 int
	BlockedHostnames                []string
	BidTimeout                      time.Duration
	ManifestTimeout                 time.Duration
	BalanceCheckerCfg               BalanceCheckerConfig
	Attributes                      types.Attributes
	DeploymentIngressStaticHosts    bool
	DeploymentIngressDomain         string
	ClusterSettings                 map[interface{}]interface{}
	RPCQueryTimeout                 time.Duration
	CachedResultMaxAge              time.Duration
}

func NewDefaultConfig() Config {
	return Config{
		ClusterWaitReadyDuration: time.Second * 10,
		BidDeposit:               mtypes.DefaultBidMinDeposit,
		BalanceCheckerCfg: BalanceCheckerConfig{
			// PollingPeriod:           5 * time.Minute,
			// MinimumBalanceThreshold: mparams.DefaultBidMinDeposit.Amount.Mul(sdk.NewIntFromUint64(2)).Uint64(),
			WithdrawalPeriod: 24 * time.Hour,
		},
		MaxGroupVolumes: constants.DefaultMaxGroupVolumes,
	}
}
