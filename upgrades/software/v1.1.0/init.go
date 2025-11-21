// Package v1_1_0
// nolint revive
package v1_1_0

import (
	utypes "pkg.akt.dev/node/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(UpgradeName, initUpgrade)
}
