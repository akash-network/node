package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/go/node/escrow/module"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/testutil/state"
)

type kTestSuite struct {
	*state.TestSuite
}

func Test_AccountSettlement(t *testing.T) {
	ssuite := state.SetupTestSuite(t)
	ctx := ssuite.Context()

	bkeeper := ssuite.BankKeeper()
	ekeeper := ssuite.EscrowKeeper()

	lid := testutil.LeaseID(t)
	did := lid.DeploymentID()

	aid := did.ToEscrowAccountID()
	pid := lid.ToEscrowPaymentID()

	aowner := testutil.AccAddress(t)

	amt := testutil.AkashCoin(t, 1000)
	powner := testutil.AccAddress(t)
	rate := testutil.AkashCoin(t, 10)

	// create an account
	bkeeper.
		On("SendCoinsFromAccountToModule", ctx, aowner, module.ModuleName, sdk.NewCoins(amt)).
		Return(nil).Once()
	assert.NoError(t, ekeeper.AccountCreate(ctx, aid, aowner, []etypes.Depositor{{
		Owner:   aowner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}}))

	{
		acct, err := ekeeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.State.SettledAt)
	}

	// create payment
	err := ekeeper.PaymentCreate(ctx, pid, powner, sdk.NewDecCoinFromCoin(rate))
	assert.NoError(t, err)

	blkdelta := int64(10)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)
	// trigger settlement by closing the account,
	// 2% is take rate, which in this test equals 2
	// 98 uakt is payment amount
	// 900 uakt must be returned to the aowner

	bkeeper.
		On("SendCoinsFromModuleToModule", ctx, module.ModuleName, distrtypes.ModuleName, sdk.NewCoins(testutil.AkashCoin(t, 2))).
		Return(nil).Once().
		On("SendCoinsFromModuleToAccount", mock.Anything, module.ModuleName, powner, sdk.NewCoins(testutil.AkashCoin(t, (rate.Amount.Int64()*10)-2))).
		Return(nil).Once().
		On("SendCoinsFromModuleToAccount", mock.Anything, module.ModuleName, aowner, sdk.NewCoins(testutil.AkashCoin(t, amt.Amount.Int64()-(rate.Amount.Int64()*10)))).
		Return(nil).Once()
	err = ekeeper.AccountClose(ctx, aid)
	assert.NoError(t, err)

	acct, err := ekeeper.GetAccount(ctx, aid)
	require.NoError(t, err)
	require.Equal(t, ctx.BlockHeight(), acct.State.SettledAt)
	require.Equal(t, etypes.StateClosed, acct.State.State)
	require.Equal(t, testutil.AkashDecCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), acct.State.Transferred[0])
}

func Test_AccountCreate(t *testing.T) {
	ssuite := state.SetupTestSuite(t)
	ctx := ssuite.Context()

	bkeeper := ssuite.BankKeeper()
	ekeeper := ssuite.EscrowKeeper()

	id := testutil.DeploymentID(t).ToEscrowAccountID()

	owner := testutil.AccAddress(t)
	amt := testutil.AkashCoinRandom(t)
	amt2 := testutil.AkashCoinRandom(t)

	// create account
	bkeeper.
		On("SendCoinsFromAccountToModule", mock.Anything, owner, module.ModuleName, sdk.NewCoins(amt)).
		Return(nil).Once()
	assert.NoError(t, ekeeper.AccountCreate(ctx, id, owner, []etypes.Depositor{{
		Owner:   owner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}}))

	// deposit more tokens
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 10)
	bkeeper.
		On("SendCoinsFromAccountToModule", mock.Anything, owner, module.ModuleName, sdk.NewCoins(amt2)).
		Return(nil).Once()

	assert.NoError(t, ekeeper.AccountDeposit(ctx, id, []etypes.Depositor{{
		Owner:   owner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt2),
	}}))

	// close account
	// each deposit is it's own send
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 10)
	bkeeper.
		On("SendCoinsFromModuleToAccount", mock.Anything, module.ModuleName, owner, sdk.NewCoins(amt)).
		Return(nil).Once().
		On("SendCoinsFromModuleToAccount", mock.Anything, module.ModuleName, owner, sdk.NewCoins(amt2)).
		Return(nil).Once()

	assert.NoError(t, ekeeper.AccountClose(ctx, id))

	// no deposits after closed
	assert.Error(t, ekeeper.AccountDeposit(ctx, id, []etypes.Depositor{{
		Owner:   owner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}}))

	// no re-creating account
	assert.Error(t, ekeeper.AccountCreate(ctx, id, owner, []etypes.Depositor{{
		Owner:   owner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}}))
}

func Test_PaymentCreate(t *testing.T) {
	ssuite := state.SetupTestSuite(t)
	ctx := ssuite.Context()

	bkeeper := ssuite.BankKeeper()
	ekeeper := ssuite.EscrowKeeper()

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
		Return(nil).Once()
	assert.NoError(t, ekeeper.AccountCreate(ctx, aid, aowner, []etypes.Depositor{{
		Owner:   aowner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}}))

	{
		acct, err := ekeeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.State.SettledAt)
	}

	// create payment
	err := ekeeper.PaymentCreate(ctx, pid, powner, sdk.NewDecCoinFromCoin(rate))
	assert.NoError(t, err)

	// withdraw some funds
	blkdelta := int64(10)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)
	bkeeper.
		On("SendCoinsFromModuleToModule", mock.Anything, module.ModuleName, distrtypes.ModuleName, sdk.NewCoins(testutil.AkashCoin(t, 2))).
		Return(nil).Once().
		On("SendCoinsFromModuleToAccount", mock.Anything, module.ModuleName, powner, sdk.NewCoins(testutil.AkashCoin(t, (rate.Amount.Int64()*blkdelta)-2))).
		Return(nil).Once()
	err = ekeeper.PaymentWithdraw(ctx, pid)
	assert.NoError(t, err)

	{
		acct, err := ekeeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.State.SettledAt)

		require.Equal(t, etypes.StateOpen, acct.State.State)
		require.Equal(t, testutil.AkashDecCoin(t, amt.Amount.Int64()-rate.Amount.Int64()*ctx.BlockHeight()), sdk.NewDecCoinFromDec(acct.State.Funds[0].Denom, acct.State.Funds[0].Amount))
		require.Equal(t, testutil.AkashDecCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), acct.State.Transferred[0])

		payment, err := ekeeper.GetPayment(ctx, pid)
		require.NoError(t, err)

		require.Equal(t, etypes.StateOpen, payment.State.State)
		require.Equal(t, testutil.AkashCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), payment.State.Withdrawn)
		require.Equal(t, testutil.AkashDecCoin(t, 0), payment.State.Balance)
	}

	// close payment
	blkdelta = 20
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)
	bkeeper.
		On("SendCoinsFromModuleToModule", mock.Anything, module.ModuleName, distrtypes.ModuleName, sdk.NewCoins(testutil.AkashCoin(t, 4))).
		Return(nil).Once().
		On("SendCoinsFromModuleToAccount", ctx, module.ModuleName, powner, sdk.NewCoins(testutil.AkashCoin(t, (rate.Amount.Int64()*blkdelta)-4))).
		Return(nil).Once()
	assert.NoError(t, ekeeper.PaymentClose(ctx, pid))

	{
		acct, err := ekeeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.State.SettledAt)

		require.Equal(t, etypes.StateOpen, acct.State.State)
		require.Equal(t, testutil.AkashDecCoin(t, amt.Amount.Int64()-rate.Amount.Int64()*ctx.BlockHeight()), sdk.NewDecCoinFromDec(acct.State.Funds[0].Denom, acct.State.Funds[0].Amount))
		require.Equal(t, testutil.AkashDecCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), acct.State.Transferred[0])

		payment, err := ekeeper.GetPayment(ctx, pid)
		require.NoError(t, err)

		require.Equal(t, etypes.StateClosed, payment.State.State)
		require.Equal(t, testutil.AkashCoin(t, rate.Amount.Int64()*ctx.BlockHeight()), payment.State.Withdrawn)
		require.Equal(t, testutil.AkashDecCoin(t, 0), payment.State.Balance)
	}

	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 30)

	// can't withdraw from a closed payment
	assert.Error(t, ekeeper.PaymentWithdraw(ctx, pid))

	// can't re-created a closed payment
	assert.Error(t, ekeeper.PaymentCreate(ctx, pid, powner, sdk.NewDecCoinFromCoin(rate)))

	// closing the account transfers all remaining funds
	bkeeper.
		On("SendCoinsFromModuleToAccount", ctx, module.ModuleName, aowner, sdk.NewCoins(testutil.AkashCoin(t, amt.Amount.Int64()-rate.Amount.Int64()*30))).
		Return(nil).Once()
	err = ekeeper.AccountClose(ctx, aid)
	assert.NoError(t, err)
}

func Test_Overdraft(t *testing.T) {
	ssuite := state.SetupTestSuite(t)
	ctx := ssuite.Context()

	bkeeper := ssuite.BankKeeper()
	ekeeper := ssuite.EscrowKeeper()

	lid := testutil.LeaseID(t)
	did := lid.DeploymentID()

	aid := did.ToEscrowAccountID()
	pid := lid.ToEscrowPaymentID()

	aowner := testutil.AccAddress(t)
	amt := testutil.AkashCoin(t, 1000)
	powner := testutil.AccAddress(t)
	rate := testutil.AkashCoin(t, 10)

	// create the account
	bkeeper.
		On("SendCoinsFromAccountToModule", ctx, aowner, module.ModuleName, sdk.NewCoins(amt)).
		Return(nil).Once()
	err := ekeeper.AccountCreate(ctx, aid, aowner, []etypes.Depositor{{
		Owner:   aowner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}})

	require.NoError(t, err)

	// create payment
	err = ekeeper.PaymentCreate(ctx, pid, powner, sdk.NewDecCoinFromCoin(rate))
	require.NoError(t, err)

	// withdraw after 105 blocks
	// account is expected to be overdrafted for 50uakt, i.e. balance must show -50
	blkdelta := int64(1000/10 + 5)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)
	bkeeper.
		On("SendCoinsFromModuleToModule", mock.Anything, module.ModuleName, distrtypes.ModuleName, sdk.NewCoins(testutil.AkashCoin(t, 20))).
		Return(nil).Once().
		On("SendCoinsFromModuleToAccount", mock.Anything, module.ModuleName, powner, sdk.NewCoins(testutil.AkashCoin(t, 980))).
		Return(nil).Once()

	err = ekeeper.PaymentWithdraw(ctx, pid)
	require.NoError(t, err)

	acct, err := ekeeper.GetAccount(ctx, aid)
	require.NoError(t, err)
	require.Equal(t, ctx.BlockHeight(), acct.State.SettledAt)

	expectedOverdraft := sdkmath.LegacyNewDec(50)

	require.Equal(t, etypes.StateOverdrawn, acct.State.State)
	require.True(t, acct.State.Funds[0].Amount.IsNegative())
	require.Equal(t, sdk.NewDecCoins(sdk.NewDecCoinFromCoin(amt)), acct.State.Transferred)
	require.Equal(t, expectedOverdraft, acct.State.Funds[0].Amount.Abs())

	payment, err := ekeeper.GetPayment(ctx, pid)
	require.NoError(t, err)

	require.Equal(t, etypes.StateOverdrawn, payment.State.State)
	require.Equal(t, amt, payment.State.Withdrawn)
	require.Equal(t, testutil.AkashDecCoin(t, 0), payment.State.Balance)
	require.Equal(t, expectedOverdraft, payment.State.Unsettled.Amount)

	// account close will should not return an error when trying to close when overdrafted
	// it will try to settle, as there were no deposits state must not change
	err = ekeeper.AccountClose(ctx, aid)
	assert.NoError(t, err)

	acct, err = ekeeper.GetAccount(ctx, aid)
	require.NoError(t, err)

	require.Equal(t, etypes.StateOverdrawn, acct.State.State)
	require.True(t, acct.State.Funds[0].Amount.IsNegative())
	require.Equal(t, sdk.NewDecCoins(sdk.NewDecCoinFromCoin(amt)), acct.State.Transferred)
	require.Equal(t, expectedOverdraft, acct.State.Funds[0].Amount.Abs())

	// attempting to close account 2nd time should not change the state
	err = ekeeper.AccountClose(ctx, aid)
	assert.NoError(t, err)

	acct, err = ekeeper.GetAccount(ctx, aid)
	require.NoError(t, err)

	require.Equal(t, etypes.StateOverdrawn, acct.State.State)
	require.True(t, acct.State.Funds[0].Amount.IsNegative())
	require.Equal(t, sdk.NewDecCoins(sdk.NewDecCoinFromCoin(amt)), acct.State.Transferred)
	require.Equal(t, expectedOverdraft, acct.State.Funds[0].Amount.Abs())

	payment, err = ekeeper.GetPayment(ctx, pid)
	require.NoError(t, err)

	require.Equal(t, etypes.StateOverdrawn, payment.State.State)
	require.Equal(t, amt, payment.State.Withdrawn)
	require.Equal(t, testutil.AkashDecCoin(t, 0), payment.State.Balance)

	// deposit more funds into account
	// this will trigger settlement and payoff if the deposit balance is sufficient
	// 1st transfer: actual deposit of 1000uakt
	// 2nd transfer: take rate 1uakt = 50 * 0.02
	// 3rd transfer: payment withdraw of 49uakt
	// 4th transfer: return a remainder of 950uakt to the owner
	bkeeper.
		On("SendCoinsFromAccountToModule", ctx, aowner, module.ModuleName, sdk.NewCoins(amt)).
		Return(nil).Once().
		On("SendCoinsFromModuleToModule", mock.Anything, module.ModuleName, distrtypes.ModuleName, sdk.NewCoins(testutil.AkashCoin(t, 1))).
		Return(nil).Once().
		On("SendCoinsFromModuleToAccount", mock.Anything, module.ModuleName, powner, sdk.NewCoins(testutil.AkashCoin(t, 49))).
		Return(nil).Once().
		On("SendCoinsFromModuleToAccount", mock.Anything, module.ModuleName, aowner, sdk.NewCoins(testutil.AkashCoin(t, 950))).
		Return(nil).Once()

	err = ekeeper.AccountDeposit(ctx, aid, []etypes.Depositor{{
		Owner:   aowner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}})
	assert.NoError(t, err)

	acct, err = ekeeper.GetAccount(ctx, aid)
	assert.NoError(t, err)

	require.Equal(t, etypes.StateClosed, acct.State.State)
	require.Equal(t, acct.State.Funds[0].Amount, sdkmath.LegacyZeroDec())

	payment, err = ekeeper.GetPayment(ctx, pid)
	require.NoError(t, err)
	require.Equal(t, etypes.StateClosed, payment.State.State)
}

func Test_PaymentCreate_later(t *testing.T) {
	ssuite := state.SetupTestSuite(t)
	ctx := ssuite.Context()

	bkeeper := ssuite.BankKeeper()
	ekeeper := ssuite.EscrowKeeper()

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
	assert.NoError(t, ekeeper.AccountCreate(ctx, aid, aowner, []etypes.Depositor{{
		Owner:   aowner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}}))

	blkdelta := int64(10)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)

	// create payment
	assert.NoError(t, ekeeper.PaymentCreate(ctx, pid, powner, sdk.NewDecCoinFromCoin(rate)))

	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 1)

	{
		acct, err := ekeeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight()-1, acct.State.SettledAt)
	}
}
