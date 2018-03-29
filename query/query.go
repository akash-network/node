package query

import (
	"github.com/ovrclk/akash/state"
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

func DeploymentGroupPath(daddr []byte, gseq uint64) string {
	return state.DeploymentGroupPath + X(state.DeploymentGroupID(daddr, gseq))
}

func OrderPath(daddr []byte, gseq uint64, oseq uint64) string {
	return state.OrderPath + X(state.OrderID(daddr, gseq, oseq))
}

func FulfillmentPath(daddr []byte, gseq uint64, oseq uint64, paddr []byte) string {
	return state.FulfillmentPath + X(state.FulfillmentID(daddr, gseq, oseq, paddr))
}

func LeasePath(daddr []byte, gseq uint64, oseq uint64, paddr []byte) string {
	return state.LeasePath + X(state.LeaseID(daddr, gseq, oseq, paddr))
}
