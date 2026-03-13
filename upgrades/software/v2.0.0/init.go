// Package v2_0_0
// nolint revive
package v2_0_0

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	dv1 "pkg.akt.dev/go/node/deployment/v1"

	utypes "pkg.akt.dev/node/v2/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(UpgradeName, initUpgrade)

	utypes.RegisterMigration(dv1.ModuleName, 6, newDeploymentMigration)

	const pythChecksum = "91dc2aada6e94f102013cb7bf799892b137b033561430941475a3e355e7eef4d"
	const wormholeChecksum = "4f8366678c6947bd6765e2bdd3d5650245835eff4f7b9defb032f844ab9c770f"

	pythActual := sha256.Sum256(pythContract)
	wormholeActual := sha256.Sum256(wormholeContract)

	pythActualStr := hex.EncodeToString(pythActual[:])
	wormholeActualStr := hex.EncodeToString(wormholeActual[:])

	if pythChecksum != pythActualStr {
		panic(fmt.Sprintf("pyth checksum does not match expected != actual (%s != %s)", pythChecksum, pythActualStr))
	}

	if wormholeChecksum != wormholeActualStr {
		panic(fmt.Sprintf("wormhole checksum does not match expected != actual (%s != %s)", wormholeChecksum, wormholeActualStr))
	}
}
