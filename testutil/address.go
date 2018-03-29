package testutil

import (
	"testing"

	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/util"
)

func Address(t *testing.T) []byte {
	return PublicKey(t).Address()
}

func HexAddress(t *testing.T) string {
	return util.X(Address(t))
}

func DeploymentAddress(t *testing.T) []byte {
	return state.DeploymentAddress(Address(t), 1)
}

func HexDeploymentAddress(t *testing.T) string {
	return util.X(DeploymentAddress(t))
}
