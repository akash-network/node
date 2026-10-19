// Package v2_2_0_sink_provisional
// nolint revive
package v2_2_0_sink_provisional

import (
	utypes "pkg.akt.dev/node/v2/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(UpgradeName, initUpgrade)
}
