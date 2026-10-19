package app_test

import (
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

func (h *sinkHarness) depositFrom(t *testing.T, key *secp256k1.PrivKey, amount int64) *abci.ExecTxResult {
	t.Helper()
	msg := &types.MsgSink{DestType: 1, Dest: base58.Encode(h.recipient[:]), Amount: sdk.NewInt64Coin("uakt", amount),
		Sender: sdk.AccAddress(key.PubKey().Address()).String()}
	return h.deliverBytes(t, h.txFrom(t, key, "", msg))
}

func TestSinkDailyAmountPerAccount(t *testing.T) {
	h := newSinkHarness(t, func(g *types.GenesisState) {
		g.Params.MinTransfer = 10
		g.Params.MaxDailyAmountPerAccount = 100
		g.Params.MaxPendingPerAccount = 20
	})
	k := h.a.Keepers.Akash.Sink
	usage := func() types.AccountUsage { return k.GetAccountUsage(h.ctx(), h.sender) }
	res := h.deliver(t, "", h.deposit(60))
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, uint64(60), usage().DailyAmount)

	before := h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender)
	totals := k.GetTotals(h.ctx())
	res = h.deliver(t, "", h.deposit(41))
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "sink amount 41 uakt exceeds the account's remaining daily amount of 40 uakt")
	require.Equal(t, totals, k.GetTotals(h.ctx()))
	require.Equal(t, uint64(60), usage().DailyAmount)
	require.Equal(t, uint32(1), usage().DailyCount)
	require.Equal(t, before.AmountOf("uakt").SubRaw(10), h.a.Keepers.Cosmos.Bank.GetAllBalances(h.ctx(), h.sender).AmountOf("uakt"))
	require.NoError(t, k.CheckBalance(h.ctx()))

	res = h.deliver(t, "", h.deposit(40)) // exactly the remainder
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, uint64(100), usage().DailyAmount)
	res = h.deliver(t, "", h.deposit(10))
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "remaining daily amount of 0 uakt")

	// The cap is per account.
	other := h.reporters[4]
	res = h.depositFrom(t, other, 100)
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, uint64(100), k.GetAccountUsage(h.ctx(), sdk.AccAddress(other.PubKey().Address())).DailyAmount)

	// The UTC day boundary resets the amount with the count.
	day := usage().Day
	h.now = time.Unix(int64(day+1)*86400-2, 0) //nolint:gosec // next block: one second before midnight
	res = h.deliver(t, "", h.deposit(10))
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "remaining daily amount of 0 uakt")
	res = h.deliver(t, "", h.deposit(100)) // exactly midnight
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, types.AccountUsage{Sender: h.sender, Day: day + 1, DailyCount: 1, PendingCount: 3, DailyAmount: 100}, usage())

	// Lowering the cap below today's usage blocks further deposits. Turning
	// it off still accumulates, so a later cap counts the whole day.
	gov := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	server := sinkkeeper.MsgServer{Keeper: k}
	p := k.GetParams(h.ctx())
	p.MaxDailyAmountPerAccount = 50
	_, err := server.UpdateParams(h.ctx(), &types.MsgUpdateParams{Authority: gov, Params: p})
	require.NoError(t, err)
	res = h.deliver(t, "", h.deposit(10))
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "remaining daily amount of 0 uakt")
	p.MaxDailyAmountPerAccount = 0
	_, err = server.UpdateParams(h.ctx(), &types.MsgUpdateParams{Authority: gov, Params: p})
	require.NoError(t, err)
	res = h.deliver(t, "", h.deposit(100))
	require.Zero(t, res.Code, res.Log)
	require.Equal(t, uint64(200), usage().DailyAmount)
	p.MaxDailyAmountPerAccount = 210
	_, err = server.UpdateParams(h.ctx(), &types.MsgUpdateParams{Authority: gov, Params: p})
	require.NoError(t, err)
	res = h.deliver(t, "", h.deposit(11))
	require.NotZero(t, res.Code)
	require.Contains(t, res.Log, "remaining daily amount of 10 uakt")

	// Governance cannot set a nonzero cap below the minimum.
	p.MaxDailyAmountPerAccount = 9
	_, err = server.UpdateParams(h.ctx(), &types.MsgUpdateParams{Authority: gov, Params: p})
	require.ErrorContains(t, err, "max daily amount per account must be zero or at least min transfer")
	require.Equal(t, uint64(210), k.GetParams(h.ctx()).MaxDailyAmountPerAccount)

	// Export and import keep the cap and each account's daily amount.
	exported := k.ExportGenesis(h.ctx())
	require.Equal(t, uint64(210), exported.Params.MaxDailyAmountPerAccount)
	amounts := map[string]uint64{}
	for _, a := range exported.Accounts {
		amounts[sdk.AccAddress(a.Sender).String()] = a.DailyAmount
	}
	require.Equal(t, map[string]uint64{h.sender.String(): 200, sdk.AccAddress(other.PubKey().Address()).String(): 100}, amounts)
	h.checkSettlementExport(t)
}
