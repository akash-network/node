package v2_2_0

import (
	"context"
	"fmt"
	"strconv"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	sinktypes "pkg.akt.dev/go/node/sink/v1"

	apptypes "pkg.akt.dev/node/v2/app/types"
	utypes "pkg.akt.dev/node/v2/upgrades/types"
)

const UpgradeName = "v2.2.0"

func init() {
	utypes.RegisterUpgrade(UpgradeName, func(logger log.Logger, app *apptypes.App) (utypes.IUpgrade, error) {
		return &upgrade{App: app, log: logger}, nil
	})
}

type upgrade struct {
	*apptypes.App
	log log.Logger
}

func (*upgrade) StoreLoader() *storetypes.StoreUpgrades {
	return &storetypes.StoreUpgrades{
		Added:   []string{sinktypes.StoreKey},
		Deleted: []string{},
	}
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx context.Context, p upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		sctx := sdk.UnwrapSDKContext(ctx)
		if _, exists := fromVM[sinktypes.ModuleName]; exists {
			return nil, fmt.Errorf("sink is already present in the pre-upgrade version map")
		}

		cache, write := sctx.CacheContext()

		swept, err := up.Keepers.Akash.Sink.PrepareInstall(cache, up.Keepers.Cosmos.Distr)
		if err != nil {
			return nil, fmt.Errorf("prepare sink address: %w", err)
		}

		if !swept.IsZero() {
			up.log.Info("moved preexisting sink address balance to the community pool", "amount", swept.String())
		}

		toVM, err := up.MM.RunMigrations(cache, up.Configurator, fromVM)
		if err != nil {
			return toVM, err
		}

		result, err := RepairEscrowPayments(cache, up.Keepers.Akash.Escrow, up.Keepers.Akash.Market)
		if err != nil {
			return nil, err
		}
		cache.EventManager().EmitEvent(sdk.NewEvent("escrow_orphan_payments_repaired",
			sdk.NewAttribute("closed_payments", strconv.FormatUint(result.ClosedPayments, 10)),
			sdk.NewAttribute("missing_leases", strconv.FormatUint(result.MissingLeases, 10)),
		))

		msgServer := wasmkeeper.NewMsgServerImpl(up.Keepers.Cosmos.Wasm)
		govAddr := up.Keepers.Cosmos.Wasm.GetAuthority()

		contractAddr := "akash1zwv6feuzhy6a9wekh96cd57lsarmqlwxdypdsplw6zhfncqw6ftqucuysk"

		_, err = msgServer.StoreAndMigrateContract(cache, &wasmtypes.MsgStoreAndMigrateContract{
			Authority:             govAddr,
			WASMByteCode:          pythVaaContract,
			Contract:              contractAddr,
			InstantiatePermission: &wasmtypes.AllowNobody,
			Msg:                   []byte("{}"),
		})
		if err != nil {
			return toVM, err
		}

		if err = up.Keepers.Akash.Sink.CheckBalance(cache); err != nil {
			return nil, err
		}

		sinkParams, err := up.Keepers.Akash.Sink.GetParams(cache)
		if err != nil {
			return nil, err
		}
		if sinkParams.DepositsEnabled {
			return nil, fmt.Errorf("sink must remain disabled after installation")
		}

		write()

		return toVM, nil
	}
}
