package keeper_test

// Regression tests for the same-block escrow settlement skip.
//
// Root cause: accountSettle only includes StateOpen payments when
// heightDelta (BlockHeight - SettledAt) is nonzero. When a settle has
// already run in the current block (SettledAt == BlockHeight), open
// payments are excluded from the returned set. PaymentClose then
// reports success without closing or settling its payment, and
// PaymentWithdraw panics.

import (
	"fmt"
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	escrowid "pkg.akt.dev/go/node/escrow/id/v1"
	escrowmodule "pkg.akt.dev/go/node/escrow/module"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	"pkg.akt.dev/go/testutil"

	emocks "pkg.akt.dev/node/v2/testutil/cosmos/mocks"
	"pkg.akt.dev/node/v2/testutil/state"
	bmemodule "pkg.akt.dev/node/v2/x/bme"
	ekeeper "pkg.akt.dev/node/v2/x/escrow/keeper"
)

func sameBlockSettleHarness(t *testing.T) (sdk.Context, escrowid.Payment, *emocks.BankKeeper, ekeeper.Keeper) {
	ssuite := state.SetupTestSuite(t)
	ctx := ssuite.Context()
	bkeeper := ssuite.BankKeeper()
	ekeeper := ssuite.EscrowKeeper()

	lid := testutil.LeaseID(t)
	did := lid.DeploymentID()
	aid := did.ToEscrowAccountID()
	pid := lid.ToEscrowPaymentID()

	aowner := testutil.AccAddress(t)
	powner := testutil.AccAddress(t)

	amt := testutil.ACTCoin(t, 1000)
	rate := sdk.NewCoin("uact", sdkmath.NewInt(30))

	ssuite.MockBMEForDeposit(aowner, amt)
	require.NoError(t, ekeeper.AccountCreate(ctx, aid, aowner, []etypes.Depositor{{
		Owner:   aowner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
	}}))
	require.NoError(t, ekeeper.PaymentCreate(ctx, pid, powner, sdk.NewDecCoinFromCoin(rate)))
	bkeeper.On("SendCoinsFromModuleToModule", mock.Anything, escrowmodule.ModuleName,
		mock.MatchedBy(func(dest string) bool {
			return dest == bmemodule.ModuleName || dest == distrtypes.ModuleName
		}), mock.Anything).Return(nil).Maybe()
	bkeeper.On("SendCoinsFromModuleToModule", mock.Anything, bmemodule.ModuleName, mock.Anything, mock.Anything).Return(nil).Maybe()
	bkeeper.On("MintCoins", mock.Anything, bmemodule.ModuleName, mock.Anything).Return(nil).Maybe()
	bkeeper.On("BurnCoins", mock.Anything, bmemodule.ModuleName, mock.Anything).Return(nil).Maybe()
	bkeeper.On("SendCoinsFromModuleToAccount", mock.Anything, escrowmodule.ModuleName, mock.Anything, mock.Anything).Return(nil).Maybe()

	return ctx, pid, bkeeper, ekeeper
}

// PaymentClose in the same block as the last settle must still close and
// settle the payment. Vulnerable code returns success and leaves the
// payment open with its funds stuck in escrow.
func Test_SameBlockSettle_PaymentCloseClosesPayment(t *testing.T) {
	ctx, pid, _, ekeeper := sameBlockSettleHarness(t)

	err := ekeeper.PaymentClose(ctx, pid)
	require.NoError(t, err, "PaymentClose reported success")

	pmnt, err := ekeeper.GetPayment(ctx, pid)
	require.NoError(t, err)
	require.Equal(t, etypes.StateClosed, pmnt.State.State,
		"payment must be closed by PaymentClose even when no blocks elapsed since last settle")
}

// PaymentWithdraw in the same block as the last settle must not panic.
// Vulnerable code panics ("couldn't find payment") because the open
// payment is excluded from the settle result; on-chain this reverts the
// caller's MsgWithdrawLease transaction (gas lost, tx failed).
func Test_SameBlockSettle_PaymentWithdrawNoPanic(t *testing.T) {
	ctx, pid, _, ekeeper := sameBlockSettleHarness(t)

	require.NotPanics(t, func() {
		err := ekeeper.PaymentWithdraw(ctx, pid)
		require.NoError(t, err)
	}, "PaymentWithdraw must handle a zero-height settle window without panicking")
}

// A lease that is closed in the block it was created (create+close in one
// block, e.g. orchestrator flow or a front-run) must not strand the
// payment: the close must settle and close it.
func Test_SameBlockSettle_CreateAndCloseLease(t *testing.T) {
	ctx, pid, _, ekeeper := sameBlockSettleHarness(t)

	err := ekeeper.PaymentClose(ctx, pid)
	require.NoError(t, err)

	pmnt, err := ekeeper.GetPayment(ctx, pid)
	require.NoError(t, err)
	require.Equal(t, etypes.StateClosed, pmnt.State.State,
		fmt.Sprintf("payment stranded open with balance %s after same-block close", pmnt.State.Balance))
}
