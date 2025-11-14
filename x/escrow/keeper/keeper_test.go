package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pkg.akt.dev/go/node/escrow/module"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	"pkg.akt.dev/go/testutil"

	cmocks "pkg.akt.dev/node/v2/testutil/cosmos/mocks"
	"pkg.akt.dev/node/v2/testutil/state"
	"pkg.akt.dev/node/v2/x/escrow/keeper"
)

func Test_AccountCreate(t *testing.T) {
	ctx, keeper, bkeeper := setupKeeper(t)
	id := testutil.DeploymentID(t).ToEscrowAccountID()

	owner := testutil.AccAddress(t)
	amt := testutil.AkashCoinRandom(t)
	amt2 := testutil.AkashCoinRandom(t)

	// create account
	bkeeper.
		On("SendCoinsFromAccountToModule", ctx, owner, module.ModuleName, sdk.NewCoins(amt)).
		Return(nil)
	assert.NoError(t, keeper.AccountCreate(ctx, id, owner, []etypes.Depositor{{
		Owner:   owner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}}))

	// deposit more tokens
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 10)
	bkeeper.
		On("SendCoinsFromAccountToModule", ctx, owner, module.ModuleName, sdk.NewCoins(amt2)).
		Return(nil)
	assert.NoError(t, keeper.AccountDeposit(ctx, id, []etypes.Depositor{{
		Owner:   owner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt2),
	}}))

	// close account
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 10)
	bkeeper.
		On("SendCoinsFromModuleToAccount", ctx, module.ModuleName, owner, sdk.NewCoins(amt.Add(amt2))).
		Return(nil)
	assert.NoError(t, keeper.AccountClose(ctx, id))

	// no deposits after closed
	assert.Error(t, keeper.AccountDeposit(ctx, id, []etypes.Depositor{{
		Owner:   owner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}}))

	// no re-creating account
	assert.Error(t, keeper.AccountCreate(ctx, id, owner, []etypes.Depositor{{
		Owner:   owner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}}))
}

func Test_PaymentCreate(t *testing.T) {
	ctx, keeper, bkeeper := setupKeeper(t)
	lid := testutil.LeaseID(t)
	did := lid.DeploymentID()

	aid := did.ToEscrowAccountID()
	pid := lid.ToEscrowPaymentID()

	aowner := testutil.AccAddress(t)

	amt := testutil.AkashCoin(t, 1000)
	powner := testutil.AccAddress(t)
	rate := testutil.AkashCoin(t, 10)

	// create account
	bkeeper.
		On("SendCoinsFromAccountToModule", ctx, aowner, module.ModuleName, sdk.NewCoins(amt)).
		Return(nil)
	assert.NoError(t, keeper.AccountCreate(ctx, aid, aowner, []etypes.Depositor{{
		Owner:   aowner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}}))

	{
		acct, err := keeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.State.SettledAt)
	}

	// create payment
	err := keeper.PaymentCreate(ctx, pid, powner, sdk.NewDecCoinFromCoin(rate))
	assert.NoError(t, err)

	// withdraw some funds
	blkdelta := int64(10)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)
	bkeeper.
		On("SendCoinsFromModuleToAccount", ctx, module.ModuleName, powner, sdk.NewCoins(testutil.AkashCoin(t, rate.Amount.Int64()*blkdelta))).
		Return(nil)
	err = keeper.PaymentWithdraw(ctx, pid)
	assert.NoError(t, err)

	{
		acct, err := keeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.State.SettledAt)

		require.Equal(t, etypes.StateOpen, acct.State.State)
		require.Equal(t, testutil.AkashDecCoin(t, amt.Amount.Int64()-rate.Amount.Int64()*ctx.BlockHeight()), sdk.NewDecCoinFromDec(acct.State.Funds[0].Denom, acct.State.Funds[0].Amount))
		require.Equal(t, testutil.AkashDecCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), acct.State.Transferred[0])

		payment, err := keeper.GetPayment(ctx, pid)
		require.NoError(t, err)

		require.Equal(t, etypes.StateOpen, payment.State.State)
		require.Equal(t, testutil.AkashCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), payment.State.Withdrawn)
		require.Equal(t, testutil.AkashDecCoin(t, 0), payment.State.Balance)
	}

	// close payment
	blkdelta = 20
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)
	bkeeper.
		On("SendCoinsFromModuleToAccount", ctx, module.ModuleName, powner, sdk.NewCoins(testutil.AkashCoin(t, rate.Amount.Int64()*blkdelta))).
		Return(nil)
	assert.NoError(t, keeper.PaymentClose(ctx, pid))

	{
		acct, err := keeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.State.SettledAt)

		require.Equal(t, etypes.StateOpen, acct.State.State)
		require.Equal(t, testutil.AkashDecCoin(t, amt.Amount.Int64()-rate.Amount.Int64()*ctx.BlockHeight()), sdk.NewDecCoinFromDec(acct.State.Funds[0].Denom, acct.State.Funds[0].Amount))
		require.Equal(t, testutil.AkashDecCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), acct.State.Transferred[0])

		payment, err := keeper.GetPayment(ctx, pid)
		require.NoError(t, err)

		require.Equal(t, etypes.StateClosed, payment.State.State)
		require.Equal(t, testutil.AkashCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), payment.State.Withdrawn)
		require.Equal(t, testutil.AkashDecCoin(t, 0), payment.State.Balance)
	}

	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 30)

	// can't withdraw from a closed payment
	assert.Error(t, keeper.PaymentWithdraw(ctx, pid))

	// can't re-created a closed payment
	assert.Error(t, keeper.PaymentCreate(ctx, pid, powner, sdk.NewDecCoinFromCoin(rate)))

	// closing the account transfers all remaining funds
	bkeeper.
		On("SendCoinsFromModuleToAccount", ctx, module.ModuleName, aowner, sdk.NewCoins(testutil.AkashCoin(t, amt.Amount.Int64()-rate.Amount.Int64()*30))).
		Return(nil)
	assert.NoError(t, keeper.AccountClose(ctx, aid))
}

func Test_Payment_Overdraw(t *testing.T) {
	ctx, keeper, bkeeper := setupKeeper(t)
	lid := testutil.LeaseID(t)
	did := lid.DeploymentID()

	aid := did.ToEscrowAccountID()
	pid := lid.ToEscrowPaymentID()

	aowner := testutil.AccAddress(t)
	amt := testutil.AkashCoin(t, 1000)
	powner := testutil.AccAddress(t)
	rate := testutil.AkashCoin(t, 10)

	// create account
	bkeeper.
		On("SendCoinsFromAccountToModule", ctx, aowner, module.ModuleName, sdk.NewCoins(amt)).
		Return(nil)
	err := keeper.AccountCreate(ctx, aid, aowner, []etypes.Depositor{{
		Owner:   aowner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}})

	require.NoError(t, err)

	// create payment
	err = keeper.PaymentCreate(ctx, pid, powner, sdk.NewDecCoinFromCoin(rate))
	require.NoError(t, err)

	// withdraw some funds
	blkdelta := int64(1000/10 + 5)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)
	bkeeper.
		On("SendCoinsFromModuleToAccount", ctx, module.ModuleName, powner, sdk.NewCoins(testutil.AkashCoin(t, 1000))).
		Return(nil)

	err = keeper.PaymentWithdraw(ctx, pid)
	require.NoError(t, err)

	{
		acct, err := keeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.State.SettledAt)

		require.Equal(t, etypes.StateOverdrawn, acct.State.State)
		require.True(t, acct.State.Funds[0].Amount.IsNegative())
		require.Equal(t, sdk.NewDecCoins(sdk.NewDecCoinFromCoin(amt)), acct.State.Transferred)

		payment, err := keeper.GetPayment(ctx, pid)
		require.NoError(t, err)

		require.Equal(t, etypes.StateOverdrawn, payment.State.State)
		require.Equal(t, amt, payment.State.Withdrawn)
		require.Equal(t, testutil.AkashDecCoin(t, 0), payment.State.Balance)
	}
}

func Test_PaymentCreate_later(t *testing.T) {
	ctx, keeper, bkeeper := setupKeeper(t)
	lid := testutil.LeaseID(t)
	did := lid.DeploymentID()

	aid := did.ToEscrowAccountID()
	pid := lid.ToEscrowPaymentID()

	aowner := testutil.AccAddress(t)

	amt := testutil.AkashCoin(t, 1000)
	powner := testutil.AccAddress(t)
	rate := testutil.AkashCoin(t, 10)

	// create account
	bkeeper.
		On("SendCoinsFromAccountToModule", ctx, aowner, module.ModuleName, sdk.NewCoins(amt)).
		Return(nil)
	assert.NoError(t, keeper.AccountCreate(ctx, aid, aowner, []etypes.Depositor{{
		Owner:   aowner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}}))

	blkdelta := int64(10)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)

	// create payment
	assert.NoError(t, keeper.PaymentCreate(ctx, pid, powner, sdk.NewDecCoinFromCoin(rate)))

	{
		acct, err := keeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.State.SettledAt)
	}
}

func setupKeeper(t testing.TB) (sdk.Context, keeper.Keeper, *cmocks.BankKeeper) {
	t.Helper()
	ssuite := state.SetupTestSuite(t)
	return ssuite.Context(), ssuite.EscrowKeeper(), ssuite.BankKeeper()
}
