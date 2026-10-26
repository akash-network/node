package app_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	"github.com/stretchr/testify/require"

	sinktypes "pkg.akt.dev/go/node/sink/v1"
	sinkupgrade "pkg.akt.dev/node/v2/upgrades/software/v2.2.0-sink-provisional"
	utypes "pkg.akt.dev/node/v2/upgrades/types"
	utilcli "pkg.akt.dev/node/v2/util/cli"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

// preSinkLedger emulates the ledger just before the sink upgrade: no sink store
// contents and no account at the sink address.
func preSinkLedger(t *testing.T) (*sinkHarness, sdk.Context) {
	t.Helper()
	h := newSinkHarness(t, func(g *sinktypes.GenesisState) {
		g.Params.Enabled = false
		g.Params.Reporters = nil
		g.Params.ReporterThreshold = 0
	})
	ctx, _ := h.ctx().CacheContext()
	ctx = ctx.WithChainID("akash-1").WithBlockTime(h.now).WithEventManager(sdk.NewEventManager())
	s := ctx.KVStore(h.a.GetKey(sinktypes.StoreKey))
	it := storetypes.KVStorePrefixIterator(s, nil)
	var keys [][]byte
	for ; it.Valid(); it.Next() {
		keys = append(keys, bytes.Clone(it.Key()))
	}
	require.NoError(t, it.Close())
	for _, key := range keys {
		s.Delete(key)
	}
	ak := h.a.Keepers.Cosmos.Acct
	ak.RemoveAccount(ctx, ak.GetAccount(ctx, sinkkeeper.Address()))
	require.True(t, h.a.Keepers.Cosmos.Bank.GetAllBalances(ctx, sinkkeeper.Address()).IsZero())
	return h, ctx
}

// sendBeforeUpgrade moves coins the way a pre-upgrade MsgSend would. The old
// binary has no sink restriction, so write balances directly.
func sendBeforeUpgrade(t *testing.T, h *sinkHarness, ctx sdk.Context, from, to sdk.AccAddress, coins sdk.Coins) {
	t.Helper()
	bank, ok := h.a.Keepers.Cosmos.Bank.(bankkeeper.BaseKeeper)
	require.True(t, ok)
	balances := bank.Balances
	for _, coin := range coins {
		key := collections.Join(from, coin.Denom)
		have, err := balances.Get(ctx, key)
		require.NoError(t, err)
		require.NoError(t, balances.Set(ctx, key, have.Sub(coin.Amount)))
		require.NoError(t, balances.Set(ctx, collections.Join(to, coin.Denom), h.a.Keepers.Cosmos.Bank.GetBalance(ctx, to, coin.Denom).Amount.Add(coin.Amount)))
	}
}

func runSinkUpgradeWithInfo(t *testing.T, h *sinkHarness, ctx sdk.Context, info string) error {
	t.Helper()
	ctor := utypes.GetUpgradesList()[sinkupgrade.UpgradeName]
	require.NotNil(t, ctor)
	up, err := ctor(log.NewNopLogger(), h.a.App)
	require.NoError(t, err)
	require.Equal(t, []string{sinktypes.StoreKey}, up.StoreLoader().Added)
	from := h.a.MM.GetVersionMap()
	delete(from, sinktypes.ModuleName)
	plan := upgradetypes.Plan{Name: sinkupgrade.UpgradeName, Height: ctx.BlockHeight(), Info: info}
	_, err = up.UpgradeHandler()(ctx, plan, from)
	return err
}

// cosmovisorInfo is plan info as the release tooling generates it.
func cosmovisorInfo(t *testing.T) string {
	t.Helper()
	bz, err := json.Marshal(utilcli.UpgradeInfo{
		Binaries: map[string]string{"linux/amd64": "https://example.com/akash.zip?checksum=sha256:00"},
		Configs:  map[string]string{"app.toml": "https://example.com/app.toml"},
	})
	require.NoError(t, err)
	return string(bz)
}

func runSinkUpgrade(t *testing.T, h *sinkHarness, ctx sdk.Context) error {
	t.Helper()
	return runSinkUpgradeWithInfo(t, h, ctx, cosmovisorInfo(t))
}

func communityPool(t *testing.T, h *sinkHarness, ctx sdk.Context) sdk.DecCoins {
	t.Helper()
	pool, err := h.a.Keepers.Cosmos.Distr.FeePool.Get(ctx)
	require.NoError(t, err)
	return pool.CommunityPool
}

func requireInstalled(t *testing.T, h *sinkHarness, ctx sdk.Context, accountNumber uint64) {
	t.Helper()
	k := h.a.Keepers.Akash.Sink
	acc := h.a.Keepers.Cosmos.Acct.GetAccount(ctx, sinkkeeper.Address())
	macc, ok := acc.(sdk.ModuleAccountI)
	require.True(t, ok, "%T", acc)
	require.Equal(t, sinktypes.ModuleName, macc.GetName())
	require.Equal(t, accountNumber, macc.GetAccountNumber())
	require.Nil(t, macc.GetPubKey())
	require.Equal(t, macc, h.a.Keepers.Cosmos.Acct.GetModuleAccount(ctx, sinktypes.ModuleName))
	// The module starts empty: balance = pending + minted + voided = 0.
	require.Equal(t, sinktypes.Totals{ReporterEpoch: 1}, k.GetTotals(ctx))
	require.True(t, h.a.Keepers.Cosmos.Bank.GetAllBalances(ctx, sinkkeeper.Address()).IsZero())
	require.NoError(t, k.CheckBalance(ctx))
	require.False(t, k.GetParams(ctx).Enabled)
	require.Equal(t, sinktypes.DefaultParams(), k.GetParams(ctx))
	// The sweep authorization is gone once the store is initialized.
	_, err := k.PrepareInstall(ctx, h.a.Keepers.Cosmos.Distr)
	require.ErrorContains(t, err, "already initialized")
	require.Error(t, h.a.Keepers.Cosmos.Bank.SendCoinsFromModuleToAccount(ctx, sinktypes.ModuleName, h.sender, sdk.NewCoins(sdk.NewInt64Coin("uakt", 1))))
}

func TestSinkUpgradeSweepsDustAndKeepsAccountNumber(t *testing.T) {
	h, ctx := preSinkLedger(t)
	ak := h.a.Keepers.Cosmos.Acct
	// A pre-upgrade MsgSend creates a BaseAccount at the deterministic address.
	dusted := ak.NewAccountWithAddress(ctx, sinkkeeper.Address())
	ak.SetAccount(ctx, dusted)
	dust := sdk.NewCoins(sdk.NewInt64Coin("uakt", 1), sdk.NewInt64Coin("other", 5))
	sendBeforeUpgrade(t, h, ctx, h.sender, sinkkeeper.Address(), dust)
	poolBefore := communityPool(t, h, ctx)
	supply := h.a.Keepers.Cosmos.Bank.GetSupply(ctx, "uakt")

	require.NoError(t, runSinkUpgrade(t, h, ctx))

	requireInstalled(t, h, ctx, dusted.GetAccountNumber())
	require.Equal(t, poolBefore.Add(sdk.NewDecCoinsFromCoins(dust...)...), communityPool(t, h, ctx))
	require.Equal(t, supply, h.a.Keepers.Cosmos.Bank.GetSupply(ctx, "uakt"))
	var swept []string
	for _, event := range ctx.EventManager().Events() {
		if event.Type == "sink_preexisting_balance" {
			swept = append(swept, event.Attributes[0].Value)
		}
	}
	require.Equal(t, []string{dust.String()}, swept)
}

func TestSinkUpgradeConvertsEmptyBaseAccount(t *testing.T) {
	// Feegrant and similar paths create an account without sending coins.
	h, ctx := preSinkLedger(t)
	ak := h.a.Keepers.Cosmos.Acct
	existing := ak.NewAccountWithAddress(ctx, sinkkeeper.Address())
	ak.SetAccount(ctx, existing)
	poolBefore := communityPool(t, h, ctx)
	require.NoError(t, runSinkUpgrade(t, h, ctx))
	requireInstalled(t, h, ctx, existing.GetAccountNumber())
	require.Equal(t, poolBefore, communityPool(t, h, ctx))
}

func TestSinkUpgradeSweepsLockedVestingBalance(t *testing.T) {
	// MsgCreateVestingAccount could place locked coins at the address.
	h, ctx := preSinkLedger(t)
	ak := h.a.Keepers.Cosmos.Acct
	base := ak.NewAccountWithAddress(ctx, sinkkeeper.Address()).(*authtypes.BaseAccount)
	locked := sdk.NewCoins(sdk.NewInt64Coin("uakt", 1_000))
	vesting, err := vestingtypes.NewContinuousVestingAccount(base, locked, ctx.BlockTime().Unix(), ctx.BlockTime().Add(365*24*time.Hour).Unix())
	require.NoError(t, err)
	ak.SetAccount(ctx, vesting)
	sendBeforeUpgrade(t, h, ctx, h.sender, sinkkeeper.Address(), locked)
	poolBefore := communityPool(t, h, ctx)
	require.NoError(t, runSinkUpgrade(t, h, ctx))
	requireInstalled(t, h, ctx, base.GetAccountNumber())
	require.Equal(t, poolBefore.Add(sdk.NewDecCoinsFromCoins(locked...)...), communityPool(t, h, ctx))
}

func TestSinkUpgradeCleanAddressAndRestrictions(t *testing.T) {
	h, ctx := preSinkLedger(t)
	poolBefore := communityPool(t, h, ctx)
	require.NoError(t, runSinkUpgrade(t, h, ctx))
	acc := h.a.Keepers.Cosmos.Acct.GetAccount(ctx, sinkkeeper.Address())
	require.NotNil(t, acc)
	requireInstalled(t, h, ctx, acc.GetAccountNumber())
	require.Equal(t, poolBefore, communityPool(t, h, ctx))
	for _, event := range ctx.EventManager().Events() {
		require.NotEqual(t, "sink_preexisting_balance", event.Type)
	}

	// Governance enables deposits once reporters and a minimum are set. No
	// funding step comes first.
	k := h.a.Keepers.Akash.Sink
	p := k.GetParams(ctx)
	p.Enabled = true
	p.MinTransfer = 1
	require.ErrorContains(t, k.SetParams(ctx, p), "reporters must be configured")
	for _, key := range h.reporters[:4] {
		p.Reporters = append(p.Reporters, sdk.AccAddress(key.PubKey().Address()).String())
	}
	p.ReporterThreshold = 3
	require.NoError(t, k.SetParams(ctx, p))
	require.True(t, k.GetParams(ctx).Enabled)
	require.NoError(t, k.CheckBalance(ctx))
}

func TestSinkUpgradePlanInfo(t *testing.T) {
	// The handler does not read plan info. Every form cosmovisor accepts
	// installs the module, and balances outside the sink address do not move.
	for name, info := range map[string]string{
		"generated": cosmovisorInfo(t),
		"empty":     "",
		"binaries":  `{"binaries":{"linux/amd64":"https://example.com/akash.zip?checksum=sha256:00"}}`,
		"url":       "https://example.com/upgrade-info.json",
	} {
		t.Run(name, func(t *testing.T) {
			h, ctx := preSinkLedger(t)
			before := h.a.Keepers.Cosmos.Bank.GetAllBalances(ctx, h.bystander)
			require.NoError(t, runSinkUpgradeWithInfo(t, h, ctx, info))
			acc := h.a.Keepers.Cosmos.Acct.GetAccount(ctx, sinkkeeper.Address())
			require.NotNil(t, acc)
			requireInstalled(t, h, ctx, acc.GetAccountNumber())
			require.Equal(t, before, h.a.Keepers.Cosmos.Bank.GetAllBalances(ctx, h.bystander))
		})
	}
}

func TestSinkUpgradeRejectsPresentModule(t *testing.T) {
	h, ctx := preSinkLedger(t)
	ctor := utypes.GetUpgradesList()[sinkupgrade.UpgradeName]
	up, err := ctor(log.NewNopLogger(), h.a.App)
	require.NoError(t, err)
	plan := upgradetypes.Plan{Name: sinkupgrade.UpgradeName, Height: ctx.BlockHeight(), Info: cosmovisorInfo(t)}
	_, err = up.UpgradeHandler()(ctx, plan, h.a.MM.GetVersionMap())
	require.ErrorContains(t, err, "already present")
	require.False(t, ctx.KVStore(h.a.GetKey(sinktypes.StoreKey)).Has([]byte("params")))
}
