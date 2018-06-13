package query

import (
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/util"
)

func AccountPath(address []byte) string {
	return state.AccountPath + util.X(address)
}

func ProvidersPath() string {
	return state.ProviderPath
}

func ProviderPath(address []byte) string {
	return state.ProviderPath + util.X(address)
}

func DeploymentsPath() string {
	return state.DeploymentPath
}

func DeploymentPath(address []byte) string {
	return state.DeploymentPath + util.X(address)
}

func DeploymentLeasesPath(address []byte) string {
	return state.LeasePath + util.X(address)
}

func DeploymentGroupsPath() string {
	return state.DeploymentGroupPath
}

func DeploymentGroupPath(id types.DeploymentGroupID) string {
	return state.DeploymentGroupPath + keys.DeploymentGroupID(id).Path()
}

func OrdersPath() string {
	return state.OrderPath
}

func OrderPath(id types.OrderID) string {
	return state.OrderPath + keys.OrderID(id).Path()
}

func FulfillmentsPath() string {
	return state.FulfillmentPath
}

func FulfillmentPath(id types.FulfillmentID) string {
	return state.FulfillmentPath + keys.FulfillmentID(id).Path()
}

func LeasesPath() string {
	return state.LeasePath
}

func LeasePath(id types.LeaseID) string {
	return state.LeasePath + keys.LeaseID(id).Path()
}
