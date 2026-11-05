package handler_test

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	emodule "pkg.akt.dev/go/node/escrow/module"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
	"pkg.akt.dev/go/testutil"
)

func TestWithdrawLeaseState(t *testing.T) {
	for _, tc := range []struct {
		state   mv1.Lease_State
		payable bool
	}{
		{mv1.LeaseActive, true},
		{mv1.LeaseReclaiming, true},
		{mv1.LeaseClosed, false},
		{mv1.LeaseInsufficientFunds, true},
		{mv1.LeaseStateInvalid, true},
	} {
		t.Run(tc.state.String(), func(t *testing.T) {
			suite := setupTestSuite(t)
			prepareBlanketMocks(suite)
			lid, bid, order := suite.createLeaseWithReclamation(nil)
			suite.setupLeaseEscrow(bid, order)
			ctx := suite.Context()
			lease, found := suite.MarketKeeper().GetLease(ctx, lid)
			require.True(t, found)
			lease.State = tc.state
			require.NoError(t, suite.MarketKeeper().SaveLease(ctx, lease))

			// The market guard blocks closed leases; escrow handles the remaining states.
			pid := lid.ToEscrowPaymentID()
			beforePayment, err := suite.EscrowKeeper().GetPayment(ctx, pid)
			require.NoError(t, err)
			beforeAccount, err := suite.EscrowKeeper().GetAccount(ctx, pid.AID)
			require.NoError(t, err)
			ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 1)
			_, err = suite.handler(ctx, &mvbeta.MsgWithdrawLease{ID: lid})
			if tc.payable {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, mv1.ErrLeaseNotActive)
			}
			afterPayment, err := suite.EscrowKeeper().GetPayment(ctx, pid)
			require.NoError(t, err)
			afterAccount, err := suite.EscrowKeeper().GetAccount(ctx, pid.AID)
			require.NoError(t, err)
			if tc.payable {
				amount := sdk.NewCoin(bid.Price.Denom, bid.Price.Amount.TruncateInt())
				require.True(t, amount.IsPositive())
				require.Equal(t, amount, afterPayment.State.Withdrawn)
				suite.BankKeeper().AssertCalled(t, "SendCoinsFromModuleToAccount", mock.Anything,
					emodule.ModuleName, sdk.MustAccAddressFromBech32(lid.Provider), sdk.NewCoins(amount))
			} else {
				require.Equal(t, beforePayment, afterPayment)
				require.Equal(t, beforeAccount, afterAccount)
				suite.BankKeeper().AssertNotCalled(t, "SendCoinsFromModuleToAccount", mock.Anything,
					emodule.ModuleName, mock.Anything, mock.Anything)
			}
		})
	}
}

func TestWithdrawUnknownLease(t *testing.T) {
	suite := setupTestSuite(t)
	_, err := suite.handler(suite.Context(), &mvbeta.MsgWithdrawLease{ID: testutil.LeaseID(t)})
	require.ErrorIs(t, err, mv1.ErrUnknownLease)
}
