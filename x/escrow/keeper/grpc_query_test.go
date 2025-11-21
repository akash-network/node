package keeper_test

import (
	"fmt"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/node/deployment/v1beta4"
	eid "pkg.akt.dev/go/node/escrow/id/v1"
	types "pkg.akt.dev/go/node/escrow/types/v1"
	"pkg.akt.dev/go/node/escrow/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	deposit "pkg.akt.dev/go/node/types/deposit/v1"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/app"
	"pkg.akt.dev/node/testutil/state"
	ekeeper "pkg.akt.dev/node/x/escrow/keeper"
)

type grpcTestSuite struct {
	*state.TestSuite
	t           *testing.T
	app         *app.AkashApp
	ctx         sdk.Context
	keeper      ekeeper.Keeper
	authzKeeper ekeeper.AuthzKeeper
	bankKeeper  ekeeper.BankKeeper

	queryClient v1.QueryClient
}

func setupTest(t *testing.T) *grpcTestSuite {
	ssuite := state.SetupTestSuite(t)
	suite := &grpcTestSuite{
		TestSuite: ssuite,

		t:           t,
		app:         ssuite.App(),
		ctx:         ssuite.Context(),
		keeper:      ssuite.EscrowKeeper(),
		authzKeeper: ssuite.AuthzKeeper(),
		bankKeeper:  ssuite.BankKeeper(),
	}

	querier := suite.keeper.NewQuerier()

	queryHelper := baseapp.NewQueryServerTestHelper(suite.ctx, suite.app.InterfaceRegistry())
	v1.RegisterQueryServer(queryHelper, querier)
	suite.queryClient = v1.NewQueryClient(queryHelper)

	return suite
}

func TestGRPCQueryAccounts(t *testing.T) {
	suite := setupTest(t)

	did1 := testutil.DeploymentID(t)
	eid1 := suite.createEscrowAccount(did1)

	expAccounts1 := types.Accounts{
		{
			ID: eid1,
			State: types.AccountState{
				Owner: did1.Owner,
				State: types.StateOpen,
				Transferred: sdk.DecCoins{
					sdk.NewDecCoin("uakt", sdkmath.ZeroInt()),
				},
				SettledAt: 0,
				Funds: []types.Balance{
					{
						Denom:  "uakt",
						Amount: sdkmath.LegacyNewDec(500000),
					},
				},
				Deposits: []types.Depositor{
					{
						Owner:   did1.Owner,
						Height:  0,
						Source:  deposit.SourceBalance,
						Balance: sdk.NewDecCoin("uakt", sdkmath.NewInt(500000)),
					},
				},
			},
		},
	}

	testCases := []struct {
		msg     string
		req     *v1.QueryAccountsRequest
		expResp v1.QueryAccountsResponse
		expPass bool
	}{
		{
			"empty request",
			&v1.QueryAccountsRequest{},
			v1.QueryAccountsResponse{
				Accounts: expAccounts1,
			},
			true,
		},
		{
			"no closed accounts",
			&v1.QueryAccountsRequest{State: "closed"},
			v1.QueryAccountsResponse{},
			true,
		},
		{
			"no overdrawn accounts",
			&v1.QueryAccountsRequest{State: "overdrawn"},
			v1.QueryAccountsResponse{},
			true,
		},
		{
			"invalid state",
			&v1.QueryAccountsRequest{State: "inv"},
			v1.QueryAccountsResponse{},
			false,
		},
		{
			"account with full XID",
			&v1.QueryAccountsRequest{State: "open", XID: fmt.Sprintf("deployment/%s", did1.Owner)},
			v1.QueryAccountsResponse{
				Accounts: expAccounts1,
			},
			true,
		},
		{
			"account with full XID",
			&v1.QueryAccountsRequest{State: "open", XID: fmt.Sprintf("deployment/%s/%d", did1.Owner, did1.DSeq)},
			v1.QueryAccountsResponse{
				Accounts: expAccounts1,
			},
			true,
		},
		{
			"account not found",
			&v1.QueryAccountsRequest{State: "open", XID: fmt.Sprintf("deployment/%s/%d", did1.Owner, did1.DSeq+1)},
			v1.QueryAccountsResponse{},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			ctx := suite.ctx

			res, err := suite.queryClient.Accounts(ctx, tc.req)

			if tc.expPass {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.Equal(t, tc.expResp.Accounts, res.Accounts)
			} else {
				require.Error(t, err)
				require.Nil(t, res)
			}

		})
	}
}

func TestGRPCQueryPayments(t *testing.T) {
	suite := setupTest(t)

	lid1 := testutil.LeaseID(t)
	did1 := lid1.DeploymentID()

	_ = suite.createEscrowAccount(did1)
	pid1 := suite.createEscrowPayment(lid1, sdk.NewDecCoin("uakt", sdkmath.NewInt(1)))

	expPayments1 := types.Payments{
		{
			ID: pid1,
			State: types.PaymentState{
				Owner:     lid1.Provider,
				State:     types.StateOpen,
				Rate:      sdk.NewDecCoin("uakt", sdkmath.NewInt(1)),
				Balance:   sdk.NewDecCoin("uakt", sdkmath.NewInt(0)),
				Unsettled: sdk.NewDecCoin("uakt", sdkmath.ZeroInt()),
				Withdrawn: sdk.NewCoin("uakt", sdkmath.NewInt(0)),
			},
		},
	}

	testCases := []struct {
		msg     string
		req     *v1.QueryPaymentsRequest
		expResp v1.QueryPaymentsResponse
		expPass bool
	}{
		{
			"empty request",
			&v1.QueryPaymentsRequest{},
			v1.QueryPaymentsResponse{
				Payments: expPayments1,
			},
			true,
		},
		{
			"no closed accounts",
			&v1.QueryPaymentsRequest{State: "closed"},
			v1.QueryPaymentsResponse{},
			true,
		},
		{
			"no overdrawn accounts",
			&v1.QueryPaymentsRequest{State: "overdrawn"},
			v1.QueryPaymentsResponse{},
			true,
		},
		{
			"invalid state",
			&v1.QueryPaymentsRequest{State: "inv"},
			v1.QueryPaymentsResponse{},
			false,
		},
		{
			"account with full XID",
			&v1.QueryPaymentsRequest{State: "open", XID: fmt.Sprintf("deployment/%s", lid1.Owner)},
			v1.QueryPaymentsResponse{
				Payments: expPayments1,
			},
			true,
		},
		{
			"account with full XID",
			&v1.QueryPaymentsRequest{State: "open", XID: fmt.Sprintf("deployment/%s/%d", lid1.Owner, lid1.DSeq)},
			v1.QueryPaymentsResponse{
				Payments: expPayments1,
			},
			true,
		},
		{
			"account not found",
			&v1.QueryPaymentsRequest{State: "open", XID: fmt.Sprintf("deployment/%s/%d", lid1.Owner, lid1.DSeq+1)},
			v1.QueryPaymentsResponse{},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			ctx := suite.ctx

			res, err := suite.queryClient.Payments(ctx, tc.req)

			if tc.expPass {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.Equal(t, tc.expResp.Payments, res.Payments)
			} else {
				require.Error(t, err)
				require.Nil(t, res)
			}

		})
	}
}

func (suite *grpcTestSuite) createEscrowAccount(id dv1.DeploymentID) eid.Account {
	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()

		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
	})

	owner, err := sdk.AccAddressFromBech32(id.Owner)
	require.NoError(suite.t, err)

	aid := id.ToEscrowAccountID()
	defaultDeposit, err := v1beta4.DefaultParams().MinDepositFor("uakt")
	require.NoError(suite.t, err)

	msg := &v1beta4.MsgCreateDeployment{
		ID: id,
		Deposit: deposit.Deposit{
			Amount:  defaultDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		}}

	deposits, err := suite.keeper.AuthorizeDeposits(suite.ctx, msg)
	require.NoError(suite.t, err)

	err = suite.keeper.AccountCreate(suite.ctx, aid, owner, deposits)
	require.NoError(suite.t, err)

	return aid
}

func (suite *grpcTestSuite) createEscrowPayment(id mv1.LeaseID, rate sdk.DecCoin) eid.Payment {
	owner, err := sdk.AccAddressFromBech32(id.Provider)
	require.NoError(suite.t, err)

	pid := id.ToEscrowPaymentID()

	err = suite.keeper.PaymentCreate(suite.ctx, pid, owner, rate)
	require.NoError(suite.t, err)

	return pid
}
