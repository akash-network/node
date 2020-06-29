package gateway

import (
	dquery "github.com/ovrclk/akash/x/deployment/query"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mquery "github.com/ovrclk/akash/x/market/query"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

const (
	deploymentPathPrefix = "/deployment/{owner}/{dseq}"
	leasePathPrefix      = "/lease/{owner}/{dseq}/{gseq}/{oseq}/{provider}"
)

func statusPath() string {
	return "status"
}

func submitManifestPath(id dtypes.DeploymentID) string {
	return dquery.DeploymentPath(id) + "/manifest"
}

func leaseStatusPath(id mtypes.LeaseID) string {
	return mquery.LeasePath(id) + "/status"
}

func serviceStatusPath(id mtypes.LeaseID, service string) string {
	return mquery.LeasePath(id) + "/service/" + service + "/status"
}

func serviceLogsPath(id mtypes.LeaseID, service string) string {
	return mquery.LeasePath(id) + "/service/" + service + "/logs"
}
