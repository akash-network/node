package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtestdata "github.com/CosmWasm/wasmd/x/wasm/keeper/testdata"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"

	epochstypes "pkg.akt.dev/go/node/epochs/v1beta1"
	escrowtypes "pkg.akt.dev/go/node/escrow/types/v1"
	markettypes "pkg.akt.dev/go/node/market/v1"
	sinktypes "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	sinkupgrade "pkg.akt.dev/node/v2/upgrades/software/v2.2.0"
	utypes "pkg.akt.dev/node/v2/upgrades/types"
	utilcli "pkg.akt.dev/node/v2/util/cli"
	escrowkeeper "pkg.akt.dev/node/v2/x/escrow/keeper"
	sinkhandler "pkg.akt.dev/node/v2/x/sink/handler"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

const sinkUpgradeContract = "akash1zwv6feuzhy6a9wekh96cd57lsarmqlwxdypdsplw6zhfncqw6ftqucuysk"

// preSinkLedger emulates the ledger just before the sink upgrade: no sink store
// contents and no account at the sink address.
func preSinkLedger(t *testing.T) (*sinkHarness, sdk.Context) {
	t.Helper()
	h := newSinkHarness(t, func(g *sinktypes.GenesisState) {
		g.Params.Enabled = false
		g.Params.Reporters = nil
		g.Params.ReportersThreshold = sdkmath.LegacyMustNewDecFromStr("0.75")
	})

	ctx, _ := h.ctx().CacheContext()
	ctx = ctx.WithChainID("akash-1").WithBlockHeight(100).WithBlockTime(h.now)
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
	installPreUpgradeContract(t, h, ctx)
	ctx = ctx.WithEventManager(sdk.NewEventManager())
	return h, ctx
}

func installPreUpgradeContract(t *testing.T, h *sinkHarness, ctx sdk.Context) {
	t.Helper()
	wasm := h.a.Keepers.Cosmos.Wasm
	codeID, _, err := wasmkeeper.NewGovPermissionKeeper(wasm).Create(ctx, h.sender, wasmtestdata.HackatomContractWasm(), &wasmtypes.AllowNobody)
	require.NoError(t, err)
	config := []byte(fmt.Sprintf(`{"admin":%q,"router_verifier":{"router_set_index":7,"routers":[[1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1]],"expected_emitter_chain":26,"expected_emitter_address":[2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2,2]}}`, h.sender.String()))
	created := &wasmtypes.AbsoluteTxPosition{BlockHeight: 1}
	_, err = wasmkeeper.InitGenesis(ctx, wasm, wasmtypes.GenesisState{
		Params: wasm.GetParams(ctx),
		Contracts: []wasmtypes.Contract{{
			ContractAddress: sinkUpgradeContract,
			ContractInfo: wasmtypes.ContractInfo{
				CodeID:  codeID,
				Creator: h.sender.String(),
				Admin:   wasm.GetAuthority(),
				Label:   "pre-upgrade Pyth state",
				Created: created,
			},
			ContractState: []wasmtypes.Model{{
				Key:   []byte("config"),
				Value: config,
			}},
			ContractCodeHistory: []wasmtypes.ContractCodeHistoryEntry{{
				Operation: wasmtypes.ContractCodeHistoryOperationTypeInit,
				CodeID:    codeID,
				Updated:   created,
				Msg:       []byte("{}"),
			}},
		}},
	})
	require.NoError(t, err)
	account := h.a.Keepers.Cosmos.Acct.NewAccountWithAddress(ctx, sdk.MustAccAddressFromBech32(sinkUpgradeContract))
	h.a.Keepers.Cosmos.Acct.SetAccount(ctx, account)
}

func seedPreUpgradeBalances(t *testing.T, h *sinkHarness, ctx sdk.Context, from, to sdk.AccAddress, coins sdk.Coins) {
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
	require.Equal(t, &storetypes.StoreUpgrades{
		Added:   []string{sinktypes.StoreKey},
		Deleted: []string{},
	}, up.StoreLoader())

	from := h.a.MM.GetVersionMap()
	delete(from, sinktypes.ModuleName)
	plan := upgradetypes.Plan{
		Name:   sinkupgrade.UpgradeName,
		Height: ctx.BlockHeight(),
		Info:   info,
	}
	versions, err := up.UpgradeHandler()(ctx, plan, from)
	if err == nil {
		require.Equal(t, h.a.MM.GetVersionMap(), versions)
		wasm := h.a.Keepers.Cosmos.Wasm
		contract := sdk.MustAccAddressFromBech32(sinkUpgradeContract)
		require.Equal(t, uint64(2), wasm.GetContractInfo(ctx, contract).CodeID)
		config, queryErr := wasm.QuerySmart(ctx, contract, []byte(`{"get_config":{}}`))
		require.NoError(t, queryErr)
		require.JSONEq(t, fmt.Sprintf(`{"admin":%q,"governance_target_chain":0,"router_verifier":{"router_set_index":7,"routers":[{"bytes":"AQEBAQEBAQEBAQEBAQEBAQEBAQE="}],"expected_emitter_chain":26,"expected_emitter_address":"AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI="}}`, h.sender.String()), string(config))
	}

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
	require.Empty(t, macc.GetPermissions())
	require.Equal(t, macc, h.a.Keepers.Cosmos.Acct.GetModuleAccount(ctx, sinktypes.ModuleName))

	// The module starts empty: balance = pending + minted + voided = 0.
	require.Equal(t, sinktypes.DefaultTotals(), sinkTotals(t, k, ctx))
	require.True(t, h.a.Keepers.Cosmos.Bank.GetAllBalances(ctx, sinkkeeper.Address()).IsZero())
	require.NoError(t, k.CheckBalance(ctx))
	require.False(t, sinkParams(t, k, ctx).Enabled)
	require.Equal(t, sinktypes.DefaultParams(), sinkParams(t, k, ctx))

	_, err := k.PrepareInstall(ctx, h.a.Keepers.Cosmos.Distr)
	require.ErrorContains(t, err, "already initialized")
	require.True(t, h.a.Keepers.Cosmos.Bank.BlockedAddr(sinkkeeper.Address()))
}

func TestSinkPrepareInstallBankFailureRollsBack(t *testing.T) {
	h, ctx := preSinkLedger(t)
	ak := h.a.Keepers.Cosmos.Acct
	base := ak.NewAccountWithAddress(ctx, sinkkeeper.Address())
	ak.SetAccount(ctx, base)
	dust := sdk.NewCoins(coin(100), sdk.NewInt64Coin("other", 5))
	seedPreUpgradeBalances(t, h, ctx, h.sender, sinkkeeper.Address(), dust)
	distr := h.a.Keepers.Cosmos.Distr
	require.NoError(t, distr.SetDelegatorWithdrawAddr(ctx, h.sender, sinkkeeper.Address()))
	fail := true
	h.a.Keepers.Cosmos.Bank.AppendSendRestriction(func(_ context.Context, from, to sdk.AccAddress, _ sdk.Coins) (sdk.AccAddress, error) {
		if fail && from.Equals(sinkkeeper.Address()) {
			return nil, sinktypes.ErrInternal.Wrap("injected sweep failure")
		}

		return to, nil
	})

	before := sinkUpgradeStores(t, h, ctx)
	events := ctx.EventManager().Events()
	pool := communityPool(t, h, ctx)
	_, err := h.a.Keepers.Akash.Sink.PrepareInstall(ctx, distr)
	require.ErrorContains(t, err, "injected sweep failure")
	require.Equal(t, before, sinkUpgradeStores(t, h, ctx))
	require.Equal(t, events, ctx.EventManager().Events())
	fail = false
	moved, err := h.a.Keepers.Akash.Sink.PrepareInstall(ctx, distr)
	require.NoError(t, err)
	require.Equal(t, dust, moved)
	require.True(t, h.a.Keepers.Cosmos.Bank.GetAllBalances(ctx, sinkkeeper.Address()).IsZero())
	require.Equal(t, pool.Add(sdk.NewDecCoinsFromCoins(dust...)...), communityPool(t, h, ctx))
	withdraw, err := distr.GetDelegatorWithdrawAddr(ctx, h.sender)
	require.NoError(t, err)
	require.Equal(t, h.sender, withdraw)
	require.Equal(t, base.GetAccountNumber(), ak.GetModuleAccount(ctx, sinktypes.ModuleName).GetAccountNumber())
}

func TestSinkUpgradeSweepsDustAndKeepsAccountNumber(t *testing.T) {
	h, ctx := preSinkLedger(t)
	ak := h.a.Keepers.Cosmos.Acct

	// A pre-upgrade MsgSend creates a BaseAccount at the deterministic address.
	dusted := ak.NewAccountWithAddress(ctx, sinkkeeper.Address())
	ak.SetAccount(ctx, dusted)
	dust := sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 1), sdk.NewInt64Coin("other", 5))
	seedPreUpgradeBalances(t, h, ctx, h.sender, sinkkeeper.Address(), dust)
	poolBefore := communityPool(t, h, ctx)
	supply := h.a.Keepers.Cosmos.Bank.GetSupply(ctx, sdkutil.DenomUakt)
	otherSupply := h.a.Keepers.Cosmos.Bank.GetSupply(ctx, "other")

	require.NoError(t, runSinkUpgrade(t, h, ctx))

	requireInstalled(t, h, ctx, dusted.GetAccountNumber())
	require.Equal(t, poolBefore.Add(sdk.NewDecCoinsFromCoins(dust...)...), communityPool(t, h, ctx))
	require.Equal(t, supply, h.a.Keepers.Cosmos.Bank.GetSupply(ctx, sdkutil.DenomUakt))
	require.Equal(t, otherSupply, h.a.Keepers.Cosmos.Bank.GetSupply(ctx, "other"))
	require.Equal(t, []proto.Message{&sinktypes.EventPreexistingBalanceMoved{
		Amount:    dust,
		Recipient: authtypes.NewModuleAddress(distrtypes.ModuleName).String(),
	}}, sinkEvents(t, ctx.EventManager().ABCIEvents()))
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
	locked := sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 1_000))
	vesting, err := vestingtypes.NewContinuousVestingAccount(base, locked, ctx.BlockTime().Unix(), ctx.BlockTime().Add(365*24*time.Hour).Unix())
	require.NoError(t, err)
	ak.SetAccount(ctx, vesting)
	seedPreUpgradeBalances(t, h, ctx, h.sender, sinkkeeper.Address(), locked)
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
	require.Empty(t, sinkEvents(t, ctx.EventManager().ABCIEvents()))

	// Governance enables deposits once reporters and a minimum are set. No
	// funding step comes first.
	k := h.a.Keepers.Akash.Sink
	p := sinkParams(t, k, ctx)
	p.Enabled = true
	p.MinAmountPerTransfer = coin(1)
	_, err := sinkhandler.NewServer(k).UpdateParams(ctx, &sinktypes.MsgUpdateParams{
		Authority: h.a.Keepers.Cosmos.Wasm.GetAuthority(),
		Params:    p,
	})
	require.ErrorContains(t, err, "deposits require reporters")

	for _, key := range h.reporters[:4] {
		p.Reporters = append(p.Reporters, sdk.AccAddress(key.PubKey().Address()).String())
	}

	p.ReportersThreshold = sdkmath.LegacyMustNewDecFromStr("0.75")
	_, err = sinkhandler.NewServer(k).UpdateParams(ctx, &sinktypes.MsgUpdateParams{
		Authority: h.a.Keepers.Cosmos.Wasm.GetAuthority(),
		Params:    p,
	})
	require.NoError(t, err)
	require.True(t, sinkParams(t, k, ctx).Enabled)
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
	require.NotNil(t, ctor)
	up, err := ctor(log.NewNopLogger(), h.a.App)
	require.NoError(t, err)
	plan := upgradetypes.Plan{
		Name:   sinkupgrade.UpgradeName,
		Height: ctx.BlockHeight(),
		Info:   cosmovisorInfo(t),
	}
	_, err = up.UpgradeHandler()(ctx, plan, h.a.MM.GetVersionMap())
	require.ErrorContains(t, err, "already present")
	require.False(t, ctx.KVStore(h.a.GetKey(sinktypes.StoreKey)).Has(sinkkeeper.ParamsKey.Bytes()))
}

func TestSinkUpgradePreservesNativeEpoch(t *testing.T) {
	for _, started := range []bool{false, true} {
		h, ctx := preSinkLedger(t)
		epochs := h.a.Keepers.Akash.Epochs
		require.NoError(t, epochs.RemoveEpoch(ctx, "day"))
		native := epochstypes.NewGenesisEpochInfo("day", 10*time.Minute)
		native.StartTime = ctx.BlockTime().Add(time.Hour)
		if started {
			native.StartTime = ctx.BlockTime().Add(-56 * 10 * time.Minute)
			native.CurrentEpoch = 57
			native.CurrentEpochStartTime = ctx.BlockTime()
			native.CurrentEpochStartHeight = 99
			native.EpochCountingStarted = true
		}

		require.NoError(t, epochs.AddEpoch(ctx, native))
		require.NoError(t, runSinkUpgrade(t, h, ctx))
		after, err := epochs.GetEpoch(ctx, "day")
		require.NoError(t, err)
		require.Equal(t, native, after)
		k := h.a.Keepers.Akash.Sink
		exported, err := k.ExportGenesis(ctx)
		require.NoError(t, err)
		require.Equal(t, sinktypes.DefaultGenesis(), exported)
		state, err := k.NewQuerier().State(ctx, nil)
		require.NoError(t, err)
		require.EqualValues(t, native.CurrentEpoch, state.Epoch)
		require.True(t, state.SendUsage.Amount.IsZero())
		require.False(t, state.Params.Enabled)
	}
}

func TestSinkUpgradeRejectsUnusableNativeEpoch(t *testing.T) {
	for _, name := range []string{"missing", "negative duration", "unstarted nonzero", "started zero"} {
		t.Run(name, func(t *testing.T) {
			h, ctx := preSinkLedger(t)
			epochs := h.a.Keepers.Akash.Epochs
			require.NoError(t, epochs.RemoveEpoch(ctx, "day"))
			native := epochstypes.NewGenesisEpochInfo("day", 10*time.Minute)
			switch name {
			case "negative duration":
				native.Duration = -time.Minute
			case "unstarted nonzero":
				native.CurrentEpoch = 57
			case "started zero":
				native.EpochCountingStarted = true
			}

			if name != "missing" {
				require.NoError(t, epochs.AddEpoch(ctx, native))
			}

			before := sinkUpgradeStores(t, h, ctx)
			events := ctx.EventManager().Events()
			var failure any
			func() {
				defer func() { failure = recover() }()
				_ = runSinkUpgrade(t, h, ctx)
			}()

			err, ok := failure.(error)
			require.True(t, ok, "expected genesis dependency failure, got %v", failure)

			if name == "missing" {
				require.ErrorIs(t, err, collections.ErrNotFound)
			} else {
				require.ErrorIs(t, err, sinktypes.ErrInvalidClock)
			}

			require.Equal(t, before, sinkUpgradeStores(t, h, ctx))
			require.Equal(t, events, ctx.EventManager().Events())
		})
	}
}

func sinkUpgradeOrphanPayment(t *testing.T, h *sinkHarness, ctx sdk.Context) escrowtypes.Payment {
	t.Helper()
	id := markettypes.LeaseID{
		Owner:    h.sender.String(),
		DSeq:     1,
		GSeq:     1,
		OSeq:     1,
		Provider: h.bystander.String(),
	}

	payment := escrowtypes.Payment{
		ID: id.ToEscrowPaymentID(),
		State: escrowtypes.PaymentState{
			Owner:     id.Provider,
			State:     escrowtypes.StateOpen,
			Rate:      sdk.NewDecCoinFromDec(sdkutil.DenomUact, sdkmath.LegacyMustNewDecFromStr("2.5")),
			Balance:   sdk.NewDecCoinFromDec(sdkutil.DenomUact, sdkmath.LegacyMustNewDecFromStr("13.75")),
			Unsettled: sdk.NewDecCoinFromDec(sdkutil.DenomUact, sdkmath.LegacyMustNewDecFromStr("4.25")),
			Withdrawn: sdk.NewInt64Coin(sdkutil.DenomUact, 17),
		},
	}

	require.NoError(t, h.a.Keepers.Akash.Escrow.SavePaymentRaw(ctx, payment))
	require.NoError(t, h.a.Keepers.Akash.Market.SaveLease(ctx, markettypes.Lease{
		ID:        id,
		State:     markettypes.LeaseClosed,
		Price:     payment.State.Rate,
		CreatedAt: 1,
		ClosedOn:  20,
	}))

	return payment
}

func TestUpgradeRegistrationAndVersions(t *testing.T) {
	h, ctx := preSinkLedger(t)
	payment := sinkUpgradeOrphanPayment(t, h, ctx)
	require.NoError(t, runSinkUpgrade(t, h, ctx))
	payment.State.State = escrowtypes.StateClosed
	got, err := h.a.Keepers.Akash.Escrow.GetPayment(ctx, payment.ID)
	require.NoError(t, err)
	require.Equal(t, payment, got)
	require.False(t, ctx.KVStore(h.a.Keepers.Akash.Escrow.StoreKey()).Has(escrowkeeper.BuildPaymentsKey(escrowtypes.StateOpen, &payment.ID)))
	require.Contains(t, ctx.EventManager().Events(), sdk.NewEvent("escrow_orphan_payments_repaired",
		sdk.NewAttribute("closed_payments", "1"), sdk.NewAttribute("missing_leases", "0")))
}

func sinkUpgradeStores(t *testing.T, h *sinkHarness, ctx sdk.Context) map[string]map[string]string {
	t.Helper()
	stores := make(map[string]map[string]string)
	for name, key := range h.a.GetKVStoreKey() {
		entries := make(map[string]string)
		iter := ctx.KVStore(key).Iterator(nil, nil)
		for ; iter.Valid(); iter.Next() {
			entries[string(iter.Key())] = string(iter.Value())
		}

		require.NoError(t, iter.Close())
		stores[name] = entries
	}

	return stores
}

func TestSinkUpgradeWasmFailureRollsBack(t *testing.T) {
	for _, account := range []string{"absent", "base", "vesting"} {
		t.Run(account, func(t *testing.T) {
			h, ctx := preSinkLedger(t)
			ak := h.a.Keepers.Cosmos.Acct
			var accountNumber uint64
			if account != "absent" {
				base := ak.NewAccountWithAddress(ctx, sinkkeeper.Address()).(*authtypes.BaseAccount)
				require.NoError(t, base.SetSequence(9))
				accountNumber = base.GetAccountNumber()
				dust := sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 1_000), sdk.NewInt64Coin("other", 5))
				if account == "vesting" {
					vesting, err := vestingtypes.NewContinuousVestingAccount(base, dust, ctx.BlockTime().Unix(), ctx.BlockTime().Add(365*24*time.Hour).Unix())
					require.NoError(t, err)
					ak.SetAccount(ctx, vesting)
				} else {
					ak.SetAccount(ctx, base)
				}

				seedPreUpgradeBalances(t, h, ctx, h.sender, sinkkeeper.Address(), dust)
			}

			distr := h.a.Keepers.Cosmos.Distr
			require.NoError(t, distr.SetDelegatorWithdrawAddr(ctx, h.sender, sinkkeeper.Address()))
			payment := sinkUpgradeOrphanPayment(t, h, ctx)
			contract := sdk.MustAccAddressFromBech32(sinkUpgradeContract)
			store := ctx.KVStore(h.a.GetKey(wasmtypes.StoreKey))
			configKey := append(wasmtypes.GetContractStorePrefix(contract), []byte("config")...)
			legacyConfig := bytes.Clone(store.Get(configKey))
			store.Delete(configKey)
			ctx.EventManager().EmitEvent(sdk.NewEvent("before_upgrade"))
			before := sinkUpgradeStores(t, h, ctx)
			events := append(sdk.Events(nil), ctx.EventManager().Events()...)

			err := runSinkUpgrade(t, h, ctx)
			require.ErrorContains(t, err, "LegacyConfig")
			require.Equal(t, before, sinkUpgradeStores(t, h, ctx), "failed upgrade must preserve every module's caller-visible state")
			require.Equal(t, events, ctx.EventManager().Events())

			store.Set(configKey, legacyConfig)
			require.NoError(t, runSinkUpgrade(t, h, ctx))
			acc := ak.GetAccount(ctx, sinkkeeper.Address())
			if account == "absent" {
				accountNumber = acc.GetAccountNumber()
			} else {
				require.Equal(t, uint64(9), acc.GetSequence())
			}

			requireInstalled(t, h, ctx, accountNumber)
			withdraw, err := distr.GetDelegatorWithdrawAddr(ctx, h.sender)
			require.NoError(t, err)
			require.Equal(t, h.sender, withdraw)
			payment.State.State = escrowtypes.StateClosed
			got, err := h.a.Keepers.Akash.Escrow.GetPayment(ctx, payment.ID)
			require.NoError(t, err)
			require.Equal(t, payment, got)
		})
	}
}
