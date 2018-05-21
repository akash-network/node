package query

import (
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
)

func AccountPath(address []byte) string {
	return state.AccountPath + X(address)
}

func ProviderPath(address []byte) string {
	return state.ProviderPath + X(address)
}

func DeploymentPath(address []byte) string {
	return state.DeploymentPath + X(address)
}

func DeploymentGroupPath(id types.DeploymentGroupID) string {
	return state.DeploymentGroupPath + keys.DeploymentGroupID(id).Path()
}

func OrderPath(id types.OrderID) string {
	return state.OrderPath + keys.OrderID(id).Path()
}

func FulfillmentPath(id types.FulfillmentID) string {
	return state.FulfillmentPath + keys.FulfillmentID(id).Path()
}

func LeasePath(id types.LeaseID) string {
	return state.LeasePath + keys.LeaseID(id).Path()
}
