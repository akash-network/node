package v1beta2

import (
	etypes "github.com/akash-network/node/x/escrow/types/v1beta2"
)

const (
	EscrowScope = "deployment"
)

func EscrowAccountForDeployment(id DeploymentID) etypes.AccountID {
	return etypes.AccountID{
		Scope: EscrowScope,
		XID:   id.String(),
	}
}

func DeploymentIDFromEscrowAccount(id etypes.AccountID) (DeploymentID, bool) {
	if id.Scope != EscrowScope {
		return DeploymentID{}, false
	}

	did, err := ParseDeploymentID(id.XID)
	return did, err == nil
}
