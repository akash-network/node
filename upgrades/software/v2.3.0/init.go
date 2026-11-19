package v2_3_0

import (
	"cosmossdk.io/log"
	sinktypes "pkg.akt.dev/go/node/sink/v1"

	apptypes "pkg.akt.dev/node/v2/app/types"
	utypes "pkg.akt.dev/node/v2/upgrades/types"
)

func init() {
	utypes.RegisterUpgrade(UpgradeName, func(logger log.Logger, app *apptypes.App) (utypes.IUpgrade, error) {
		return &upgrade{App: app, log: logger}, nil
	})

	utypes.RegisterMigration(sinktypes.ModuleName, 1, newSinkMigration)
}
