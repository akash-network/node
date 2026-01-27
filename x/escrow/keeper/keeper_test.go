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

	"pkg.akt.dev/node/v2/testutil/state"
	bmemodule "pkg.akt.dev/node/v2/x/bme"
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

	amt := testutil.ACTCoin(t, 1000)
	powner := testutil.AccAddress(t)
	// Payment rate must be in uact to match account funds (10 uakt/block * 3 = 30 uact/block)
	rate := sdk.NewCoin("uact", sdkmath.NewInt(30))

	// create account with BME
	ssuite.MockBMEForDeposit(aowner, amt)
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
	// trigger settlement by closing the account
	// Mock BME for withdrawals and settlement transfers
	bkeeper.
		On("SendCoinsFromModuleToModule", mock.Anything, module.ModuleName, mock.MatchedBy(func(dest string) bool {
			return dest == "bme" || dest == distrtypes.ModuleName
		}), mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("SendCoinsFromModuleToModule", mock.Anything, bmemodule.ModuleName, mock.Anything, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("MintCoins", mock.Anything, bmemodule.ModuleName, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("BurnCoins", mock.Anything, bmemodule.ModuleName, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Maybe()
	err = ekeeper.AccountClose(ctx, aid)
	assert.NoError(t, err)

	acct, err := ekeeper.GetAccount(ctx, aid)
	require.NoError(t, err)
	require.Equal(t, ctx.BlockHeight(), acct.State.SettledAt)
	require.Equal(t, etypes.StateClosed, acct.State.State)
	// Transferred is in uact: 30 uact/block * blocks
	require.Equal(t, sdk.NewDecCoin(rate.Denom, sdkmath.NewInt(rate.Amount.Int64()*ctx.BlockHeight())), acct.State.Transferred[0])
}

func Test_AccountCreate(t *testing.T) {
	ssuite := state.SetupTestSuite(t)
	ctx := ssuite.Context()

	bkeeper := ssuite.BankKeeper()
	ekeeper := ssuite.EscrowKeeper()

	id := testutil.DeploymentID(t).ToEscrowAccountID()

	owner := testutil.AccAddress(t)
	amt := testutil.ACTCoinRandom(t)
	amt2 := testutil.ACTCoinRandom(t)

	// create account with BME deposit flow
	// BME will convert uakt -> uact (3x swap rate)
	ssuite.MockBMEForDeposit(owner, amt)
	assert.NoError(t, ekeeper.AccountCreate(ctx, id, owner, []etypes.Depositor{{
		Owner:   owner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}}))

	// deposit more tokens with BME
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 10)
	ssuite.MockBMEForDeposit(owner, amt2)
	assert.NoError(t, ekeeper.AccountDeposit(ctx, id, []etypes.Depositor{{
		Owner:   owner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt2),
	}}))

	// close account - BME converts uact back to uakt when withdrawing
	// Each depositor gets their funds returned via BME: uact -> uakt (1/3 swap rate)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 10)

	// Mock BME withdrawal flow for each deposit
	// BME handles the conversion, use flexible matchers since decimal rounding may occur
	bkeeper.
		On("SendCoinsFromModuleToModule", mock.Anything, module.ModuleName, bmemodule.ModuleName, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("MintCoins", mock.Anything, bmemodule.ModuleName, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("BurnCoins", mock.Anything, bmemodule.ModuleName, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("SendCoinsFromModuleToAccount", mock.Anything, module.ModuleName, owner, mock.Anything).
		Return(nil).Maybe()

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

	amt := testutil.ACTCoin(t, 1000)
	powner := testutil.AccAddress(t)
	// Payment rate must match account funds denom, which is uact after BME conversion
	// 10 uakt/block * 3 (swap rate) = 30 uact/block
	rate := sdk.NewCoin("uact", sdkmath.NewInt(30))

	// create account with BME
	ssuite.MockBMEForDeposit(aowner, amt)
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

	// create payment with rate in uact (matching account funds denom)
	err := ekeeper.PaymentCreate(ctx, pid, powner, sdk.NewDecCoinFromCoin(rate))
	assert.NoError(t, err)

	// withdraw some funds - BME will handle conversion
	blkdelta := int64(10)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)
	// Mock BME operations for payment withdrawal
	bkeeper.
		On("SendCoinsFromModuleToModule", mock.Anything, module.ModuleName, bmemodule.ModuleName, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("SendCoinsFromModuleToModule", mock.Anything, bmemodule.ModuleName, mock.Anything, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("MintCoins", mock.Anything, bmemodule.ModuleName, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("BurnCoins", mock.Anything, bmemodule.ModuleName, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, powner, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, aowner, mock.Anything).
		Return(nil).Maybe()
	err = ekeeper.PaymentWithdraw(ctx, pid)
	assert.NoError(t, err)

	{
		acct, err := ekeeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.State.SettledAt)

		require.Equal(t, etypes.StateOpen, acct.State.State)
		// Balance is in uact: 3000 uact initial - (30 uact/block * blocks)
		expectedBalance := sdk.NewDecCoin("uact", sdkmath.NewInt(amt.Amount.Int64()-rate.Amount.Int64()*ctx.BlockHeight()))
		require.Equal(t, expectedBalance.Denom, acct.State.Funds[0].Denom)
		require.True(t, expectedBalance.Amount.Sub(acct.State.Funds[0].Amount).Abs().LTE(sdkmath.LegacyNewDec(1)))

		payment, err := ekeeper.GetPayment(ctx, pid)
		require.NoError(t, err)
		require.Equal(t, etypes.StateOpen, payment.State.State)
	}

	// close payment
	blkdelta = 20
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)
	bkeeper.
		On("SendCoinsFromModuleToModule", mock.Anything, module.ModuleName, mock.MatchedBy(func(dest string) bool {
			return dest == bmemodule.ModuleName || dest == distrtypes.ModuleName
		}), mock.Anything).
		Return(nil).Maybe()
	assert.NoError(t, ekeeper.PaymentClose(ctx, pid))

	{
		acct, err := ekeeper.GetAccount(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, ctx.BlockHeight(), acct.State.SettledAt)
		require.Equal(t, etypes.StateOpen, acct.State.State)

		payment, err := ekeeper.GetPayment(ctx, pid)
		require.NoError(t, err)
		require.Equal(t, etypes.StateClosed, payment.State.State)
	}

	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 30)

	// can't withdraw from a closed payment
	assert.Error(t, ekeeper.PaymentWithdraw(ctx, pid))

	// can't re-created a closed payment
	assert.Error(t, ekeeper.PaymentCreate(ctx, pid, powner, sdk.NewDecCoinFromCoin(rate)))

	// closing the account transfers all remaining funds via BME
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
	amt := testutil.ACTCoin(t, 1000)
	powner := testutil.AccAddress(t)
	// Payment rate must be in uact to match account funds (10 uakt/block * 3 = 30 uact/block)
	rate := sdk.NewCoin("uact", sdkmath.NewInt(30))

	// Setup BME mocks for withdrawal and settlement operations BEFORE AccountCreate
	bkeeper.
		On("SendCoinsFromModuleToModule", mock.Anything, module.ModuleName, mock.MatchedBy(func(dest string) bool {
			return dest == bmemodule.ModuleName || dest == distrtypes.ModuleName
		}), mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("SendCoinsFromModuleToModule", mock.Anything, bmemodule.ModuleName, mock.Anything, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("MintCoins", mock.Anything, bmemodule.ModuleName, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("BurnCoins", mock.Anything, bmemodule.ModuleName, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Maybe()

	// create the account with BME
	ssuite.MockBMEForDeposit(aowner, amt)
	err := ekeeper.AccountCreate(ctx, aid, aowner, []etypes.Depositor{{
		Owner:   aowner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}})
	require.NoError(t, err)

	// create payment
	err = ekeeper.PaymentCreate(ctx, pid, powner, sdk.NewDecCoinFromCoin(rate))
	require.NoError(t, err)

	// withdraw after 105 blocks - account will be overdrafted
	// With BME: 1000 uakt -> 3000 uact, 105 blocks * 10 uakt/block * 3 = 3150 uact
	// Overdraft: 3150 - 3000 = 150 uact
	blkdelta := int64(1000/10 + 5)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)

	// Mock BME operations for withdrawal
	bkeeper.
		On("SendCoinsFromModuleToModule", mock.Anything, module.ModuleName, mock.MatchedBy(func(dest string) bool {
			return dest == bmemodule.ModuleName || dest == distrtypes.ModuleName
		}), mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("SendCoinsFromModuleToModule", mock.Anything, bmemodule.ModuleName, mock.Anything, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("MintCoins", mock.Anything, bmemodule.ModuleName, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("BurnCoins", mock.Anything, bmemodule.ModuleName, mock.Anything).
		Return(nil).Maybe()
	bkeeper.
		On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Maybe()

	err = ekeeper.PaymentWithdraw(ctx, pid)
	require.NoError(t, err)

	acct, err := ekeeper.GetAccount(ctx, aid)
	require.NoError(t, err)
	require.Equal(t, ctx.BlockHeight(), acct.State.SettledAt)

	require.Equal(t, etypes.StateOverdrawn, acct.State.State)
	require.True(t, acct.State.Funds[0].Amount.IsNegative())

	payment, err := ekeeper.GetPayment(ctx, pid)
	require.NoError(t, err)
	require.Equal(t, etypes.StateOverdrawn, payment.State.State)

	// account close should not error when overdrafted
	err = ekeeper.AccountClose(ctx, aid)
	assert.NoError(t, err)

	acct, err = ekeeper.GetAccount(ctx, aid)
	require.NoError(t, err)
	require.Equal(t, etypes.StateOverdrawn, acct.State.State)
	require.True(t, acct.State.Funds[0].Amount.IsNegative())

	// attempting to close account 2nd time should not change the state
	err = ekeeper.AccountClose(ctx, aid)
	assert.NoError(t, err)

	acct, err = ekeeper.GetAccount(ctx, aid)
	require.NoError(t, err)
	require.Equal(t, etypes.StateOverdrawn, acct.State.State)
	require.True(t, acct.State.Funds[0].Amount.IsNegative())

	payment, err = ekeeper.GetPayment(ctx, pid)
	require.NoError(t, err)
	require.Equal(t, etypes.StateOverdrawn, payment.State.State)

	dep := sdk.NewCoin(amt.Denom, acct.State.Funds[0].Amount.Abs().TruncateInt())

	// deposit more funds into account - this will trigger settlement
	ssuite.MockBMEForDeposit(aowner, dep)
	err = ekeeper.AccountDeposit(ctx, aid, []etypes.Depositor{{
		Owner:   aowner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(dep),
	}})
	assert.NoError(t, err)

	acct, err = ekeeper.GetAccount(ctx, aid)
	assert.NoError(t, err)
	require.Equal(t, etypes.StateClosed, acct.State.State)

	payment, err = ekeeper.GetPayment(ctx, pid)
	require.NoError(t, err)
	require.Equal(t, etypes.StateClosed, payment.State.State)
}

func Test_PaymentCreate_later(t *testing.T) {
	ssuite := state.SetupTestSuite(t)
	ctx := ssuite.Context()

	ekeeper := ssuite.EscrowKeeper()

	lid := testutil.LeaseID(t)
	did := lid.DeploymentID()

	aid := did.ToEscrowAccountID()
	pid := lid.ToEscrowPaymentID()

	aowner := testutil.AccAddress(t)

	amt := testutil.ACTCoin(t, 1000)
	powner := testutil.AccAddress(t)
	// Payment rate must be in uact to match account funds (10 uakt/block * 3 = 30 uact/block)
	rate := sdk.NewCoin("uact", sdkmath.NewInt(30))

	// create account with BME
	ssuite.MockBMEForDeposit(aowner, amt)
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
