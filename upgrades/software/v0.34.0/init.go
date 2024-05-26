// Package v0_34_0
// nolint revive
package v0_34_0

import (
	utypes "pkg.akt.dev/akashd/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(UpgradeName, initUpgrade)
}
