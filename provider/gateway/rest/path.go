package rest

import (
	"fmt"

	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
)

const (
	deploymentPathPrefix = "/deployment/{dseq}"
	leasePathPrefix      = "/lease/{dseq}/{gseq}/{oseq}"
	hostnamePrefix       = "/hostname"
	endpointPrefix       = "/endpoint"
	migratePathPrefix    = "/migrate"
)

func versionPath() string {
	return "version"
}

func statusPath() string {
	return "status"
}

func validatePath() string {
	return "validate"
}

func leasePath(id mtypes.LeaseID) string {
	return fmt.Sprintf("lease/%d/%d/%d", id.DSeq, id.GSeq, id.OSeq)
}

func submitManifestPath(dseq uint64) string {
	return fmt.Sprintf("deployment/%d/manifest", dseq)
}

func leaseStatusPath(id mtypes.LeaseID) string {
	return fmt.Sprintf("%s/status", leasePath(id))
}

func leaseShellPath(lID mtypes.LeaseID) string {
	return fmt.Sprintf("%s/shell", leasePath(lID))
}

func leaseEventsPath(id mtypes.LeaseID) string {
	return fmt.Sprintf("%s/kubeevents", leasePath(id))
}

func serviceStatusPath(id mtypes.LeaseID, service string) string {
	return fmt.Sprintf("%s/service/%s/status", leasePath(id), service)
}

func serviceLogsPath(id mtypes.LeaseID) string {
	return fmt.Sprintf("%s/logs", leasePath(id))
}
