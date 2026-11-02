package v2_2_0

import (
	"context"
	"fmt"
	"strconv"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	"cosmossdk.io/x/upgrade/plan"
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

		if planInfo, err := plan.ParseInfo(p.Info); err != nil {
			return fromVM, err
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

		toVM, err := up.MM.RunMigrations(ctx, up.Configurator, fromVM)
		if err != nil {
			return toVM, err
		}

		result, err := RepairEscrowPayments(sctx, up.Keepers.Akash.Escrow, up.Keepers.Akash.Market)
		if err != nil {
			return nil, err
		}
		sctx.EventManager().EmitEvent(sdk.NewEvent("escrow_orphan_payments_repaired",
			sdk.NewAttribute("closed_payments", strconv.FormatUint(result.ClosedPayments, 10)),
			sdk.NewAttribute("missing_leases", strconv.FormatUint(result.MissingLeases, 10)),
		))

		msgServer := wasmkeeper.NewMsgServerImpl(up.Keepers.Cosmos.Wasm)
		govAddr := up.Keepers.Cosmos.Wasm.GetAuthority()

		contractAddr := "akash1zwv6feuzhy6a9wekh96cd57lsarmqlwxdypdsplw6zhfncqw6ftqucuysk"

		_, err = msgServer.StoreAndMigrateContract(ctx, &wasmtypes.MsgStoreAndMigrateContract{
			Authority:             govAddr,
			WASMByteCode:          pythVaaContract,
			Contract:              contractAddr,
			InstantiatePermission: &wasmtypes.AllowNobody,
			Msg:                   []byte("{}"),
		})
		if err != nil {
			return toVM, err
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

		// This upgrade changes records only; module schemas and versions are unchanged.
		return toVM, nil
	}
}
