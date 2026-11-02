package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/go/node/escrow/module"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/v2/testutil/state"
)

func TestSameHeightSettlement(t *testing.T) {
	for _, elapsed := range []int64{0, 10} {
		for _, action := range []string{"withdraw", "payment-close", "account-close"} {
			t.Run(action+"/elapsed="+sdkmath.NewInt(elapsed).String(), func(t *testing.T) {
				s := state.SetupTestSuite(t)
				ctx := s.Context()
				k := s.EscrowKeeper()
				lid := testutil.LeaseID(t)
				aid := lid.DeploymentID().ToEscrowAccountID()
				pid := lid.ToEscrowPaymentID()
				lid.OSeq++
				pid2 := lid.ToEscrowPaymentID()
				owner := testutil.AccAddress(t)
				recipient := testutil.AccAddress(t)
				amount := sdk.NewInt64Coin("uact", 10000)
				s.MockBMEForDeposit(owner, amount)
				s.BankKeeper().On("SendCoinsFromModuleToAccount", mock.Anything, module.ModuleName, mock.Anything, mock.Anything).Return(nil).Maybe()
				require.NoError(t, k.AccountCreate(ctx, aid, owner, []etypes.Depositor{{Owner: owner.String(), Height: ctx.BlockHeight(), Balance: sdk.NewDecCoinFromCoin(amount)}}))
				require.NoError(t, k.PaymentCreate(ctx, pid, recipient, sdk.NewDecCoinFromDec("uact", sdkmath.LegacyMustNewDecFromStr("30"))))
				require.NoError(t, k.PaymentCreate(ctx, pid2, recipient, sdk.NewDecCoinFromDec("uact", sdkmath.LegacyMustNewDecFromStr("7.5"))))
				if elapsed > 0 {
					ctx = ctx.WithBlockHeight(ctx.BlockHeight() + elapsed)
					require.NoError(t, k.PaymentWithdraw(ctx, pid))
				}
				before, err := k.GetAccount(ctx, aid)
				require.NoError(t, err)
				require.Equal(t, ctx.BlockHeight(), before.State.SettledAt)
				switch action {
				case "withdraw":
					require.NotPanics(t, func() {
						require.NoError(t, k.PaymentWithdraw(ctx, pid))
						require.NoError(t, k.PaymentWithdraw(ctx, pid2))
					})
				case "payment-close":
					require.NoError(t, k.PaymentClose(ctx, pid))
				case "account-close":
					require.NoError(t, k.AccountClose(ctx, aid))
				}
				after, err := k.GetAccount(ctx, aid)
				require.NoError(t, err)
				require.Equal(t, before.State.Transferred, after.State.Transferred, "zero-height settlement must not accrue funds")
				p, err := k.GetPayment(ctx, pid)
				require.NoError(t, err)
				p2, err := k.GetPayment(ctx, pid2)
				require.NoError(t, err)
				if action == "withdraw" {
					require.Equal(t, etypes.StateOpen, p.State.State)
					require.Equal(t, sdkmath.NewInt(30*elapsed), p.State.Withdrawn.Amount)
					require.Equal(t, sdkmath.NewInt(75*elapsed/10), p2.State.Withdrawn.Amount)
					require.Equal(t, before.State.Funds, after.State.Funds)
				} else {
					require.Equal(t, etypes.StateClosed, p.State.State)
					if action == "account-close" {
						require.Equal(t, etypes.StateClosed, p2.State.State)
						require.Equal(t, etypes.StateClosed, after.State.State)
						require.True(t, after.State.Funds[0].Amount.IsZero())
					} else {
						require.Equal(t, etypes.StateOpen, p2.State.State)
						require.Equal(t, before.State.Funds, after.State.Funds)
					}
				}
			})
		}
	}
}
