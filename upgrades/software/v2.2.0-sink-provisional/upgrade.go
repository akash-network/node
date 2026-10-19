// Package v2_2_0_sink_provisional
// nolint revive
package v2_2_0_sink_provisional

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	sinktypes "pkg.akt.dev/go/node/sink/v1"

	apptypes "pkg.akt.dev/node/v2/app/types"
	utypes "pkg.akt.dev/node/v2/upgrades/types"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

const (
	// UpgradeName is a placeholder until the release that ships x/sink is chosen.
	UpgradeName = "v2.2.0-sink-provisional"
)

// PlanInfo is the JSON carried in the upgrade plan's info field. Binaries is
// the standard cosmovisor download map. ReserveSource is the account that
// funds the sink launch reserve.
type PlanInfo struct {
	Binaries      map[string]string `json:"binaries,omitempty"`
	ReserveSource string            `json:"reserve_source"`
}

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

// ParsePlanInfo returns the reserve source named in the plan info. The info
// must be a single JSON object with no fields other than binaries and
// reserve_source, so a misspelled key cannot pass silently.
func ParsePlanInfo(info string) (sdk.AccAddress, error) {
	decoder := json.NewDecoder(strings.NewReader(info))
	decoder.DisallowUnknownFields()

	var plan PlanInfo
	if err := decoder.Decode(&plan); err != nil {
		return nil, fmt.Errorf("sink upgrade info: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("sink upgrade info must be one JSON object")
	}

	address, err := sdk.AccAddressFromBech32(plan.ReserveSource)
	if err != nil || len(address) != 20 || address.Equals(sinkkeeper.Address()) {
		return nil, fmt.Errorf("sink upgrade requires a valid ordinary reserve_source address")
	}

	return address, nil
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		sctx := sdk.UnwrapSDKContext(ctx)

		source, err := ParsePlanInfo(plan.Info)
		if err != nil {
			return nil, err
		}

		if _, exists := fromVM[sinktypes.ModuleName]; exists {
			return nil, fmt.Errorf("sink is already present in the pre-upgrade version map")
		}

		cache, write := sctx.CacheContext()

		// The sink address is deterministic. Anyone can send it coins or create
		// an account there before this height, so neither may halt the chain.
		// Install the module account at that address and move any balance to the
		// community pool; the reserve then backs the exact bank invariant alone.
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

		if err = up.Keepers.Akash.Sink.FundLaunchReserve(cache, source); err != nil {
			return nil, fmt.Errorf("sink launch reserve must be fully funded at upgrade: %w", err)
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
