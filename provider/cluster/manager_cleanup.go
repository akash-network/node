package cluster

import (
	"context"
	"github.com/ovrclk/akash/manifest/v2beta1"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	logger "github.com/tendermint/tendermint/libs/log"
)

type purgeIPEntry struct {
	serviceName string
	port        uint32
	proto       v2beta1.ServiceProtocol
}

type deployCleanupHelper struct {
	client Client
	log    logger.Logger
	lease  mtypes.LeaseID

	hostnamesToPurge []string
	ipsToPurge       []purgeIPEntry
}

func newDeployCleanupHelper(lease mtypes.LeaseID, client Client, log logger.Logger) *deployCleanupHelper {
	return &deployCleanupHelper{
		client: client,
		log:    log,
		lease:  lease,
	}
}

func (dch *deployCleanupHelper) addHostname(hostname string) {
	dch.hostnamesToPurge = append(dch.hostnamesToPurge, hostname)
}

func (dch *deployCleanupHelper) addIP(serviceName string, port uint32, proto v2beta1.ServiceProtocol) {
	dch.ipsToPurge = append(
		dch.ipsToPurge,
		purgeIPEntry{
			serviceName: serviceName,
			port:        port,
			proto:       proto,
		})
}

func (dch *deployCleanupHelper) purgeAll(ctx context.Context) {
	for _, hostname := range dch.hostnamesToPurge {
		err := dch.client.PurgeDeclaredHostname(ctx, dch.lease, hostname)
		if err != nil {
			dch.log.Error("could not purge hostname",
				"lease", dch.lease, "hsotname", hostname, "error", err)
		}
	}

	for _, ipEntry := range dch.ipsToPurge {
		err := dch.client.PurgeDeclaredIP(ctx, dch.lease, ipEntry.serviceName, ipEntry.port, ipEntry.proto)
		if err != nil {
			dch.log.Error("could not purge IP",
				"lease", dch.lease,
				"serviceName", ipEntry.serviceName,
				"port", ipEntry.port,
				"proto", ipEntry.proto,
				"error", err)
		}
	}
}
