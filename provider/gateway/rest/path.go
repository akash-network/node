package rest

import (
	"fmt"

	mtypes "github.com/ovrclk/akash/x/market/types"
)

const (
	deploymentPathPrefix = "/deployment/{dseq}"
	leasePathPrefix      = "/lease/{dseq}/{gseq}/{oseq}"
)

func statusPath() string {
	return "status"
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

func leaseEventsPath(id mtypes.LeaseID) string {
	return fmt.Sprintf("%s/kubeevents", leasePath(id))
}

func serviceStatusPath(id mtypes.LeaseID, service string) string {
	return fmt.Sprintf("%s/service/%s/status", leasePath(id), service)
}

func serviceLogsPath(id mtypes.LeaseID) string {
	return fmt.Sprintf("%s/logs", leasePath(id))
}
