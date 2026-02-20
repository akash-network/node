// Package v2_1_0
// nolint revive
package v2_1_0

import (
	utypes "pkg.akt.dev/node/v2/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(UpgradeName, initUpgrade)
}
