package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/go/node/escrow/v1"
	"pkg.akt.dev/go/testutil"

	cmocks "pkg.akt.dev/node/testutil/cosmos/mocks"
	"pkg.akt.dev/node/testutil/state"
	"pkg.akt.dev/node/x/escrow/keeper"
)

func Test_AccountCreate(t *testing.T) {
	ctx, keeper, bkeeper := setupKeeper(t)
	id := genAccountID(t)
	owner := testutil.AccAddress(t)
	amt := testutil.AkashCoinRandom(t)
	amt2 := testutil.AkashCoinRandom(t)

	// create account
	bkeeper.
		On("SendCoinsFromAccountToModule", ctx, owner, v1.ModuleName, sdk.NewCoins(amt)).
		Return(nil)
	assert.NoError(t, keeper.AccountCreate(ctx, id, owner, []v1.Deposit{{
		Depositor: owner.String(),
		Height:    ctx.BlockHeight(),
		Amount:    sdk.NewCoin(amt.Denom, amt.Amount),
		Balance:   sdk.NewDecCoinFromCoin(amt),
	}}))

	// deposit more tokens
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 10)
	bkeeper.
		On("SendCoinsFromAccountToModule", ctx, owner, v1.ModuleName, sdk.NewCoins(amt2)).
		Return(nil)
	assert.NoError(t, keeper.AccountDeposit(ctx, id, []v1.Deposit{{
		Depositor: owner.String(),
		Height:    ctx.BlockHeight(),
		Amount:    sdk.NewCoin(amt2.Denom, amt2.Amount),
		Balance:   sdk.NewDecCoinFromCoin(amt2),
	}}))

	// close account
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 10)
	bkeeper.
		On("SendCoinsFromModuleToAccount", ctx, v1.ModuleName, owner, sdk.NewCoins(amt.Add(amt2))).
		Return(nil)
	assert.NoError(t, keeper.AccountClose(ctx, id))

	// no deposits after closed
	assert.Error(t, keeper.AccountDeposit(ctx, id, []v1.Deposit{{
		Depositor: owner.String(),
		Height:    ctx.BlockHeight(),
		Amount:    sdk.NewCoin(amt.Denom, amt.Amount),
		Balance:   sdk.NewDecCoinFromCoin(amt),
	}}))

	// no re-creating account
	assert.Error(t, keeper.AccountCreate(ctx, id, owner, []v1.Deposit{{
		Depositor: owner.String(),
		Height:    ctx.BlockHeight(),
		Amount:    sdk.NewCoin(amt.Denom, amt.Amount),
		Balance:   sdk.NewDecCoinFromCoin(amt),
	}}))
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
		On("SendCoinsFromAccountToModule", ctx, aowner, v1.ModuleName, sdk.NewCoins(amt)).
		Return(nil)
	assert.NoError(t, keeper.AccountCreate(ctx, aid, aowner, []v1.Deposit{{
		Depositor: aowner.String(),
		Height:    ctx.BlockHeight(),
		Amount:    sdk.NewCoin(amt.Denom, amt.Amount),
		Balance:   sdk.NewDecCoinFromCoin(amt),
	}}))

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
		On("SendCoinsFromModuleToAccount", ctx, v1.ModuleName, powner, sdk.NewCoins(testutil.AkashCoin(t, rate.Amount.Int64()*blkdelta))).
		Return(nil)
	assert.NoError(t, keeper.PaymentWithdraw(ctx, aid, pid))

	{
		acct, err := keeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.SettledAt)

		require.Equal(t, v1.StateOpen, acct.State)
		require.Equal(t, testutil.AkashDecCoin(t, amt.Amount.Int64()-rate.Amount.Int64()*ctx.BlockHeight()), acct.Funds[0].Balance)
		require.Equal(t, testutil.AkashDecCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), acct.Transferred[0])

		payment, err := keeper.GetPayment(ctx, aid, pid)
		require.NoError(t, err)

		require.Equal(t, v1.StateOpen, payment.State)
		require.Equal(t, testutil.AkashCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), payment.Withdrawn)
		require.Equal(t, testutil.AkashDecCoin(t, 0), payment.Balance)
	}

	// close payment
	blkdelta = 20
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)
	bkeeper.
		On("SendCoinsFromModuleToAccount", ctx, v1.ModuleName, powner, sdk.NewCoins(testutil.AkashCoin(t, rate.Amount.Int64()*blkdelta))).
		Return(nil)
	assert.NoError(t, keeper.PaymentClose(ctx, aid, pid))

	{
		acct, err := keeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.SettledAt)

		require.Equal(t, v1.StateOpen, acct.State)
		require.Equal(t, testutil.AkashDecCoin(t, amt.Amount.Int64()-rate.Amount.Int64()*ctx.BlockHeight()), acct.Funds[0].Balance)
		require.Equal(t, testutil.AkashDecCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), acct.Transferred[0])

		payment, err := keeper.GetPayment(ctx, aid, pid)
		require.NoError(t, err)

		require.Equal(t, v1.StateClosed, payment.State)
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
		On("SendCoinsFromModuleToAccount", ctx, v1.ModuleName, aowner, sdk.NewCoins(testutil.AkashCoin(t, amt.Amount.Int64()-rate.Amount.Int64()*30))).
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
		On("SendCoinsFromAccountToModule", ctx, aowner, v1.ModuleName, sdk.NewCoins(amt)).
		Return(nil)
	err := keeper.AccountCreate(ctx, aid, aowner, []v1.Deposit{{
		Depositor: aowner.String(),
		Height:    ctx.BlockHeight(),
		Amount:    sdk.NewCoin(amt.Denom, amt.Amount),
		Balance:   sdk.NewDecCoinFromCoin(amt),
	}})

	require.NoError(t, err)

	// create payment
	err = keeper.PaymentCreate(ctx, aid, pid, powner, sdk.NewDecCoinFromCoin(rate))
	require.NoError(t, err)

	// withdraw some funds
	blkdelta := int64(1000/10 + 5)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)
	bkeeper.
		On("SendCoinsFromModuleToAccount", ctx, v1.ModuleName, powner, sdk.NewCoins(testutil.AkashCoin(t, 1000))).
		Return(nil)

	err = keeper.PaymentWithdraw(ctx, aid, pid)
	require.NoError(t, err)

	{
		acct, err := keeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.SettledAt)

		require.Equal(t, v1.StateOverdrawn, acct.State)
		require.Equal(t, testutil.AkashDecCoin(t, 0), acct.Funds[0].Balance)
		require.Equal(t, sdk.NewDecCoins(sdk.NewDecCoinFromCoin(amt)), acct.Transferred)

		payment, err := keeper.GetPayment(ctx, aid, pid)
		require.NoError(t, err)

		require.Equal(t, v1.StateOverdrawn, payment.State)
		require.Equal(t, amt, payment.Withdrawn)
		require.Equal(t, testutil.AkashDecCoin(t, 0), payment.Balance)
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
		On("SendCoinsFromAccountToModule", ctx, aowner, v1.ModuleName, sdk.NewCoins(amt)).
		Return(nil)
	assert.NoError(t, keeper.AccountCreate(ctx, aid, aowner, []v1.Deposit{{
		Depositor: aowner.String(),
		Height:    ctx.BlockHeight(),
		Amount:    sdk.NewCoin(amt.Denom, amt.Amount),
		Balance:   sdk.NewDecCoinFromCoin(amt),
	}}))

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

func genAccountID(t testing.TB) v1.AccountID {
	t.Helper()
	return v1.AccountID{
		Scope: "test",
		XID:   testutil.Name(t, "acct"),
	}
}

func setupKeeper(t testing.TB) (sdk.Context, keeper.Keeper, *cmocks.BankKeeper) {
	t.Helper()
	ssuite := state.SetupTestSuite(t)
	return ssuite.Context(), ssuite.EscrowKeeper(), ssuite.BankKeeper()
}
