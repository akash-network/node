// Package v0_18_0
package v0_18_0 //nolint:revive // this package is named this way becauase it is part of an upgrade

import (
	utypes "github.com/akash-network/node/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(upgradeName, initUpgrade)
}
