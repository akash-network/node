package cmd

import (
	ctypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"sync/atomic"
	"time"
)

type managedHostname struct {
	lastEvent    ctypes.HostnameResourceEvent
	presentLease mtypes.LeaseID

	presentServiceName  string
	presentExternalPort uint32
	lastChangeAt        time.Time
}

type preparedResultData struct {
	preparedAt time.Time
	data       []byte
}

type preparedResult struct {
	needsPrepare bool
	data         *atomic.Value
}

func newPreparedResult() preparedResult {
	result := preparedResult{
		data:         new(atomic.Value),
		needsPrepare: true,
	}
	result.set([]byte{})
	return result
}

func (pr *preparedResult) flag() {
	pr.needsPrepare = true
}

func (pr *preparedResult) set(data []byte) {
	pr.needsPrepare = false
	pr.data.Store(preparedResultData{
		preparedAt: time.Now(),
		data:       data,
	})
}

func (pr *preparedResult) get() preparedResultData {
	return (pr.data.Load()).(preparedResultData)
}

type ignoreListEntry struct {
	failureCount uint
	failedAt     time.Time
	lastError    error
	hostnames    map[string]struct{}
}

type hostnameOperatorConfig struct {
	listenAddress        string
	pruneInterval        time.Duration
	ignoreListEntryLimit uint
	ignoreListAgeLimit   time.Duration
	webRefreshInterval   time.Duration
	retryDelay           time.Duration
	eventFailureLimit    uint
}
