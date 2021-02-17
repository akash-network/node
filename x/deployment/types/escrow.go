package types

import (
	etypes "github.com/ovrclk/akash/x/escrow/types"
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
