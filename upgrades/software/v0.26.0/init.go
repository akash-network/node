// Package v0_26_0
// nolint revive
package v0_26_0

import (
	utypes "github.com/akash-network/node/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(UpgradeName, initUpgrade)
}
