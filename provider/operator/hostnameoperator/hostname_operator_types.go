package hostnameoperator

import (
	ctypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"time"
)

type managedHostname struct {
	lastEvent    ctypes.HostnameResourceEvent
	presentLease mtypes.LeaseID

	presentServiceName  string
	presentExternalPort uint32
	lastChangeAt        time.Time
}

type hostnameOperatorConfig struct {
	pruneInterval      time.Duration
	webRefreshInterval time.Duration
	retryDelay         time.Duration
}
