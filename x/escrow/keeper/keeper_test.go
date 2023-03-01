package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	types "github.com/akash-network/akash-api/go/node/escrow/v1beta3"

	"github.com/akash-network/node/testutil"
	"github.com/akash-network/node/testutil/state"
	"github.com/akash-network/node/x/escrow/keeper"
	"github.com/akash-network/node/x/escrow/keeper/mocks"
)

func Test_AccountCreate(t *testing.T) {
	ctx, keeper, bkeeper := setupKeeper(t)
	id := genAccountID(t)
	owner := testutil.AccAddress(t)
	amt := testutil.AkashCoinRandom(t)
	amt2 := testutil.AkashCoinRandom(t)

	// create account
	bkeeper.
		On("SendCoinsFromAccountToModule", ctx, owner, types.ModuleName, sdk.NewCoins(amt)).
		Return(nil)
	assert.NoError(t, keeper.AccountCreate(ctx, id, owner, owner, amt))

	// deposit more tokens
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 10)
	bkeeper.
		On("SendCoinsFromAccountToModule", ctx, owner, types.ModuleName, sdk.NewCoins(amt2)).
		Return(nil)
	assert.NoError(t, keeper.AccountDeposit(ctx, id, owner, amt2))

	// close account
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 10)
	bkeeper.
		On("SendCoinsFromModuleToAccount", ctx, types.ModuleName, owner, sdk.NewCoins(amt.Add(amt2))).
		Return(nil)
	assert.NoError(t, keeper.AccountClose(ctx, id))

	// no deposits after closed
	assert.Error(t, keeper.AccountDeposit(ctx, id, owner, amt))

	// no re-creating account
	assert.Error(t, keeper.AccountCreate(ctx, id, owner, owner, amt))
}

func Test_PaymentCreate(t *testing.T) {
	ctx, keeper, bkeeper := setupKeeper(t)
	aid := genAccountID(t)
	aowner := testutil.AccAddress(t)

	amt := testutil.AkashCoin(t, 1000)
	pid := testutil.Name(t, "payment")
	powner := testutil.AccAddress(t)
	rate := testutil.AkashCoin(t, 10)

	// create account
	bkeeper.
		On("SendCoinsFromAccountToModule", ctx, aowner, types.ModuleName, sdk.NewCoins(amt)).
		Return(nil)
	assert.NoError(t, keeper.AccountCreate(ctx, aid, aowner, aowner, amt))

	{
		acct, err := keeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.SettledAt)
	}

	// create payment
	assert.NoError(t, keeper.PaymentCreate(ctx, aid, pid, powner, sdk.NewDecCoinFromCoin(rate)))

	// withdraw some funds
	blkdelta := int64(10)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)
	bkeeper.
		On("SendCoinsFromModuleToAccount", ctx, types.ModuleName, powner, sdk.NewCoins(testutil.AkashCoin(t, rate.Amount.Int64()*blkdelta))).
		Return(nil)
	assert.NoError(t, keeper.PaymentWithdraw(ctx, aid, pid))

	{
		acct, err := keeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.SettledAt)

		require.Equal(t, types.AccountOpen, acct.State)
		require.Equal(t, testutil.AkashDecCoin(t, amt.Amount.Int64()-rate.Amount.Int64()*ctx.BlockHeight()), acct.Balance)
		require.Equal(t, testutil.AkashDecCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), acct.Transferred)

		payment, err := keeper.GetPayment(ctx, aid, pid)
		require.NoError(t, err)

		require.Equal(t, types.PaymentOpen, payment.State)
		require.Equal(t, testutil.AkashCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), payment.Withdrawn)
		require.Equal(t, testutil.AkashDecCoin(t, 0), payment.Balance)
	}

	// close payment
	blkdelta = 20
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)
	bkeeper.
		On("SendCoinsFromModuleToAccount", ctx, types.ModuleName, powner, sdk.NewCoins(testutil.AkashCoin(t, rate.Amount.Int64()*blkdelta))).
		Return(nil)
	assert.NoError(t, keeper.PaymentClose(ctx, aid, pid))

	{
		acct, err := keeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.SettledAt)

		require.Equal(t, types.AccountOpen, acct.State)
		require.Equal(t, testutil.AkashDecCoin(t, amt.Amount.Int64()-rate.Amount.Int64()*ctx.BlockHeight()), acct.Balance)
		require.Equal(t, testutil.AkashDecCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), acct.Transferred)

		payment, err := keeper.GetPayment(ctx, aid, pid)
		require.NoError(t, err)

		require.Equal(t, types.PaymentClosed, payment.State)
		require.Equal(t, testutil.AkashCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), payment.Withdrawn)
		require.Equal(t, testutil.AkashDecCoin(t, 0), payment.Balance)
	}

	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 30)

	// can't withdraw from a closed payment
	assert.Error(t, keeper.PaymentWithdraw(ctx, aid, pid))

	// can't re-created a closed payment
	assert.Error(t, keeper.PaymentCreate(ctx, aid, pid, powner, sdk.NewDecCoinFromCoin(rate)))

	// closing the account transfers all remaining funds
	bkeeper.
		On("SendCoinsFromModuleToAccount", ctx, types.ModuleName, aowner, sdk.NewCoins(testutil.AkashCoin(t, amt.Amount.Int64()-rate.Amount.Int64()*30))).
		Return(nil)
	assert.NoError(t, keeper.AccountClose(ctx, aid))
}

func Test_Payment_Overdraw(t *testing.T) {
	ctx, keeper, bkeeper := setupKeeper(t)
	aid := genAccountID(t)
	aowner := testutil.AccAddress(t)
	amt := testutil.AkashCoin(t, 1000)
	pid := testutil.Name(t, "payment")
	powner := testutil.AccAddress(t)
	rate := testutil.AkashCoin(t, 10)

	// create account
	bkeeper.
		On("SendCoinsFromAccountToModule", ctx, aowner, types.ModuleName, sdk.NewCoins(amt)).
		Return(nil)
	assert.NoError(t, keeper.AccountCreate(ctx, aid, aowner, aowner, amt))

	// create payment
	assert.NoError(t, keeper.PaymentCreate(ctx, aid, pid, powner, sdk.NewDecCoinFromCoin(rate)))

	// withdraw some funds
	blkdelta := int64(1000/10 + 1)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)
	bkeeper.
		On("SendCoinsFromModuleToAccount", ctx, types.ModuleName, powner, sdk.NewCoins(testutil.AkashCoin(t, 1000))).
		Return(nil)
	assert.NoError(t, keeper.PaymentWithdraw(ctx, aid, pid))

	{
		acct, err := keeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		assert.Equal(t, ctx.BlockHeight(), acct.SettledAt)

		assert.Equal(t, types.AccountOverdrawn, acct.State)
		assert.Equal(t, testutil.AkashDecCoin(t, 0), acct.Balance)
		assert.Equal(t, sdk.NewDecCoinFromCoin(amt), acct.Transferred)

		payment, err := keeper.GetPayment(ctx, aid, pid)
		assert.NoError(t, err)

		assert.Equal(t, types.PaymentOverdrawn, payment.State)
		assert.Equal(t, amt, payment.Withdrawn)
		assert.Equal(t, testutil.AkashDecCoin(t, 0), payment.Balance)
	}

}

func Test_PaymentCreate_later(t *testing.T) {
	ctx, keeper, bkeeper := setupKeeper(t)
	aid := genAccountID(t)
	aowner := testutil.AccAddress(t)

	amt := testutil.AkashCoin(t, 1000)
	pid := testutil.Name(t, "payment")
	powner := testutil.AccAddress(t)
	rate := testutil.AkashCoin(t, 10)

	// create account
	bkeeper.
		On("SendCoinsFromAccountToModule", ctx, aowner, types.ModuleName, sdk.NewCoins(amt)).
		Return(nil)
	assert.NoError(t, keeper.AccountCreate(ctx, aid, aowner, aowner, amt))

	blkdelta := int64(10)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)

	// create payment
	assert.NoError(t, keeper.PaymentCreate(ctx, aid, pid, powner, sdk.NewDecCoinFromCoin(rate)))

	{
		acct, err := keeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.SettledAt)
	}
}

func genAccountID(t testing.TB) types.AccountID {
	t.Helper()
	return types.AccountID{
		Scope: "test",
		XID:   testutil.Name(t, "acct"),
	}
}

func setupKeeper(t testing.TB) (sdk.Context, keeper.Keeper, *mocks.BankKeeper) {
	t.Helper()
	ssuite := state.SetupTestSuite(t)
	return ssuite.Context(), ssuite.EscrowKeeper(), ssuite.BankKeeper()
}
