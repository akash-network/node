// Package v2_2_0_sink_provisional
// nolint revive
package v2_2_0_sink_provisional

import (
	"context"
	"fmt"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	sinktypes "pkg.akt.dev/go/node/sink/v1"

	apptypes "pkg.akt.dev/node/v2/app/types"
	utypes "pkg.akt.dev/node/v2/upgrades/types"
)

const (
	// UpgradeName is a placeholder until the release that ships x/sink is chosen.
	UpgradeName = "v2.2.0-sink-provisional"
)

type upgrade struct {
	*apptypes.App
	log log.Logger
}

var _ utypes.IUpgrade = (*upgrade)(nil)

func initUpgrade(log log.Logger, app *apptypes.App) (utypes.IUpgrade, error) {
	up := &upgrade{
		App: app,
		log: log.With("module", fmt.Sprintf("upgrade/%s", UpgradeName)),
	}

	return up, nil
}

func (up *upgrade) StoreLoader() *storetypes.StoreUpgrades {
	return &storetypes.StoreUpgrades{
		Added:   []string{sinktypes.StoreKey},
		Deleted: []string{},
	}
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		sctx := sdk.UnwrapSDKContext(ctx)

		// plan.Info is standard cosmovisor info (binaries, configs) and is not
		// read here.
		if _, exists := fromVM[sinktypes.ModuleName]; exists {
			return nil, fmt.Errorf("sink is already present in the pre-upgrade version map")
		}

		cache, write := sctx.CacheContext()

		// The sink address is deterministic. Anyone can send it coins or create
		// an account there before this height, so neither may halt the chain.
		// Install the module account at that address and move any balance to the
		// community pool, so the module starts with a zero balance.
		// The upgrade proposal must state this disposition.
		swept, err := up.Keepers.Akash.Sink.PrepareInstall(cache, up.Keepers.Cosmos.Distr)
		if err != nil {
			return nil, fmt.Errorf("prepare sink address: %w", err)
		}

		if !swept.IsZero() {
			up.log.Info("moved preexisting sink address balance to the community pool", "amount", swept.String())
		}

		// RunMigrations runs sink InitGenesis with the default genesis state:
		// deposits and refunds disabled and no reporters.
		toVM, err := up.MM.RunMigrations(cache, up.Configurator, fromVM)
		if err != nil {
			return nil, err
		}

		if err = up.Keepers.Akash.Sink.CheckBalance(cache); err != nil {
			return nil, err
		}

		if up.Keepers.Akash.Sink.GetParams(cache).Enabled {
			return nil, fmt.Errorf("sink must remain disabled after installation")
		}

		write()

		return toVM, nil
	}
}
