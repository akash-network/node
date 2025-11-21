package handler_test

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	sdkmath "cosmossdk.io/math"
	"github.com/cometbft/cometbft/libs/rand"
	"github.com/cosmos/cosmos-sdk/baseapp"
	sdktestdata "github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
	emodule "pkg.akt.dev/go/node/escrow/module"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	ev1 "pkg.akt.dev/go/node/escrow/v1"
	v1 "pkg.akt.dev/go/node/market/v1"
	types "pkg.akt.dev/go/node/market/v1beta5"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	attr "pkg.akt.dev/go/node/types/attributes/v1"
	deposit "pkg.akt.dev/go/node/types/deposit/v1"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/testutil/state"
	dhandler "pkg.akt.dev/node/x/deployment/handler"
	ehandler "pkg.akt.dev/node/x/escrow/handler"
	"pkg.akt.dev/node/x/market/handler"
)

type testSuite struct {
	*state.TestSuite
	handler  baseapp.MsgServiceHandler
	dhandler baseapp.MsgServiceHandler
	ehandler baseapp.MsgServiceHandler
	t        testing.TB
}

func setupTestSuite(t *testing.T) *testSuite {
	ssuite := state.SetupTestSuite(t)
	suite := &testSuite{
		t:         t,
		TestSuite: ssuite,
		handler: handler.NewHandler(handler.Keepers{
			Escrow:     ssuite.EscrowKeeper(),
			Audit:      ssuite.AuditKeeper(),
			Market:     ssuite.MarketKeeper(),
			Deployment: ssuite.DeploymentKeeper(),
			Provider:   ssuite.ProviderKeeper(),
			Bank:       ssuite.BankKeeper(),
		}),
	}

	suite.dhandler = dhandler.NewHandler(suite.DeploymentKeeper(), suite.MarketKeeper(), ssuite.EscrowKeeper())
	suite.ehandler = ehandler.NewHandler(suite.EscrowKeeper(), suite.AuthzKeeper(), suite.BankKeeper())

	return suite
}

func TestProviderBadMessageType(t *testing.T) {
	suite := setupTestSuite(t)

	res, err := suite.handler(suite.Context(), sdk.Msg(sdktestdata.NewTestMsg()))
	require.Nil(t, res)
	require.Error(t, err)
	require.True(t, errors.Is(err, sdkerrors.ErrUnknownRequest))
}

func TestMarketFullFlowCloseDeployment(t *testing.T) {
	defaultDeposit, err := dtypes.DefaultParams().MinDepositFor("uakt")
	require.NoError(t, err)

	suite := setupTestSuite(t)

	ctx := suite.Context()

	deployment := testutil.Deployment(t)
	group := testutil.DeploymentGroup(t, deployment.ID, 0)
	group.GroupSpec.Resources = testutil.Resources(t)

	owner := sdk.MustAccAddressFromBech32(deployment.ID.Owner)

	// we can create provider via keeper in this test
	provider := suite.createProvider(group.GroupSpec.Requirements.Attributes).Owner
	providerAddr, err := sdk.AccAddressFromBech32(provider)
	require.NoError(t, err)

	escrowBalance := sdk.NewCoins(sdk.NewInt64Coin("uakt", 0))
	distrBalance := sdk.NewCoins(sdk.NewInt64Coin("uakt", 0))

	dmsg := &dtypes.MsgCreateDeployment{
		ID:     deployment.ID,
		Groups: dtypes.GroupSpecs{group.GroupSpec},
		Deposit: deposit.Deposit{
			Amount:  defaultDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	balances := map[string]sdk.Coin{
		deployment.ID.Owner: sdk.NewInt64Coin("uakt", 10000000),
		provider:            sdk.NewInt64Coin("uakt", 10000000),
	}

	sendCoinsFromAccountToModule := func(args mock.Arguments) {
		addr := args[1].(sdk.AccAddress)
		module := args[2].(string)
		amount := args[3].(sdk.Coins)

		require.Len(t, amount, 1)

		balances[addr.String()] = balances[addr.String()].Sub(amount[0])
		switch module {
		case emodule.ModuleName:
			escrowBalance = escrowBalance.Add(amount...)
		default:
			t.Fatalf("unexpected send to module %s", module)
		}
	}

	sendCoinsFromModuleToAccount := func(args mock.Arguments) {
		module := args[1].(string)
		addr := args[2].(sdk.AccAddress)
		amount := args[3].(sdk.Coins)

		require.Len(t, amount, 1)

		balances[addr.String()] = balances[addr.String()].Add(amount[0])

		switch module {
		case emodule.ModuleName:
			escrowBalance = escrowBalance.Sub(amount...)
		default:
			t.Fatalf("unexpected send from module %s", module)
		}
	}

	sendCoinsFromModuleToModule := func(args mock.Arguments) {
		from := args[1].(string)
		to := args[2].(string)
		amount := args[3].(sdk.Coins)

		require.Equal(t, emodule.ModuleName, from)
		require.Equal(t, distrtypes.ModuleName, to)
		require.Len(t, amount, 1)

		distrBalance = distrBalance.Add(amount...)
		escrowBalance = escrowBalance.Sub(amount...)
	}
	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()
		bkeeper.
			On("SpendableCoin", mock.Anything, mock.Anything, mock.Anything).
			Return(func(args mock.Arguments) sdk.Coin {
				addr := args[1].(sdk.AccAddress)
				denom := args[2].(string)

				require.Equal(t, "uakt", denom)

				return balances[addr.String()]
			})
	})

	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()
		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(sendCoinsFromAccountToModule).Return(nil).Once()
	})
	res, err := suite.dhandler(ctx, dmsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	order, found := suite.MarketKeeper().GetOrder(ctx, v1.OrderID{
		Owner: deployment.ID.Owner,
		DSeq:  deployment.ID.DSeq,
		GSeq:  1,
		OSeq:  1,
	})

	require.True(t, found)

	bmsg := &types.MsgCreateBid{
		ID:    v1.MakeBidID(order.ID, providerAddr),
		Price: sdk.NewDecCoin(testutil.CoinDenom, sdkmath.NewInt(1)),
		Deposit: deposit.Deposit{
			Amount:  types.DefaultBidMinDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()
		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(sendCoinsFromAccountToModule).Return(nil).Once()
	})

	res, err = suite.handler(ctx, bmsg)
	require.NotNil(t, res)
	require.NoError(t, err)

	bid := v1.MakeBidID(order.ID, providerAddr)

	t.Run("ensure bid event created", func(t *testing.T) {
		iev, err := sdk.ParseTypedEvent(res.Events[3])
		require.NoError(t, err)
		require.IsType(t, &v1.EventBidCreated{}, iev)

		dev := iev.(*v1.EventBidCreated)

		require.Equal(t, bid, dev.ID)
	})

	_, found = suite.MarketKeeper().GetBid(ctx, bid)
	require.True(t, found)

	lmsg := &types.MsgCreateLease{
		BidID: bid,
	}

	lid := v1.MakeLeaseID(bid)
	res, err = suite.handler(ctx, lmsg)
	require.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure lease event created", func(t *testing.T) {
		iev, err := sdk.ParseTypedEvent(res.Events[4])
		require.NoError(t, err)
		require.IsType(t, &v1.EventLeaseCreated{}, iev)

		dev := iev.(*v1.EventLeaseCreated)

		require.Equal(t, lid, dev.ID)
	})

	// find just created escrow account
	eacc, err := suite.EscrowKeeper().GetAccount(ctx, deployment.ID.ToEscrowAccountID())
	require.NoError(t, err)
	require.NotNil(t, eacc)

	// find just created escrow payment
	epmnt, err := suite.EscrowKeeper().GetPayment(ctx, lid.ToEscrowPaymentID())
	require.NoError(t, err)
	require.NotNil(t, epmnt)

	blocks := eacc.State.Funds[0].Amount.Quo(epmnt.State.Rate.Amount)

	ctx = ctx.WithBlockHeight(blocks.TruncateInt64() + 100)

	dcmsg := &dtypes.MsgCloseDeployment{
		ID: deployment.ID,
	}

	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()

		bkeeper.
			On("SendCoinsFromModuleToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(sendCoinsFromModuleToModule).Return(nil).Once().
			On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(sendCoinsFromModuleToAccount).Return(nil).Once().
			On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(sendCoinsFromModuleToAccount).Return(nil).Once()
	})

	// this will trigger settlement and payoff if the deposit balance is sufficient
	// 1nd transfer: take rate 10000uakt = 500,000 * 0.02
	// 2nd transfer: returned bid deposit back to the provider
	// 3rd transfer: payment withdraw of 490,000uakt
	res, err = suite.dhandler(ctx, dcmsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	eacc, err = suite.EscrowKeeper().GetAccount(ctx, deployment.ID.ToEscrowAccountID())
	require.NoError(t, err)
	require.NotNil(t, eacc)

	// find just created escrow payment
	epmnt, err = suite.EscrowKeeper().GetPayment(ctx, lid.ToEscrowPaymentID())
	require.NoError(t, err)
	require.NotNil(t, epmnt)

	// both escrow account and payment are expected to be in overdrawn state
	require.Equal(t, etypes.StateOverdrawn, eacc.State.State)
	require.Equal(t, etypes.StateOverdrawn, epmnt.State.State)

	expectedOverdraft := epmnt.State.Rate.Amount.MulInt64(100)
	require.True(t, eacc.State.Funds[0].Amount.IsNegative())
	require.Equal(t, expectedOverdraft, eacc.State.Funds[0].Amount.Abs())
	require.Equal(t, expectedOverdraft, epmnt.State.Unsettled.Amount)

	// lease must be in closed state
	lease, found := suite.MarketKeeper().GetLease(ctx, lid)
	require.True(t, found)
	require.Equal(t, v1.LeaseClosed, lease.State)

	// lease must be in closed state
	bidObj, found := suite.MarketKeeper().GetBid(ctx, bid)
	require.True(t, found)
	require.Equal(t, types.BidClosed, bidObj.State)

	// deployment must be in closed state
	depl, found := suite.DeploymentKeeper().GetDeployment(ctx, lid.DeploymentID())
	require.True(t, found)
	require.Equal(t, dv1.DeploymentClosed, depl.State)

	// should ont be able to close escrow account in overdrawn state
	err = suite.EscrowKeeper().AccountClose(ctx, deployment.ID.ToEscrowAccountID())
	require.NoError(t, err)

	// both account and payment should remain in overdrawn state
	eacc, err = suite.EscrowKeeper().GetAccount(ctx, deployment.ID.ToEscrowAccountID())
	require.NoError(t, err)
	require.NotNil(t, eacc)

	// find just created escrow payment
	epmnt, err = suite.EscrowKeeper().GetPayment(ctx, lid.ToEscrowPaymentID())
	require.NoError(t, err)
	require.NotNil(t, epmnt)

	// both escrow account and payment are expected to be in overdrawn state
	require.Equal(t, etypes.StateOverdrawn, eacc.State.State)
	require.Equal(t, etypes.StateOverdrawn, epmnt.State.State)

	require.Equal(t, expectedOverdraft, eacc.State.Funds[0].Amount.Abs())
	require.Equal(t, expectedOverdraft, epmnt.State.Unsettled.Amount)

	depositMsg := &ev1.MsgAccountDeposit{
		Signer: owner.String(),
		ID:     deployment.ID.ToEscrowAccountID(),
		Deposit: deposit.Deposit{
			Amount:  sdk.NewCoin(defaultDeposit.Denom, expectedOverdraft.TruncateInt()),
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()
		bkeeper.
			On("SendCoinsFromModuleToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(sendCoinsFromModuleToModule).Return(nil).Once().
			On("SendCoinsFromAccountToModule", mock.Anything, mock.Anything, emodule.ModuleName, mock.Anything).Run(sendCoinsFromAccountToModule).Return(nil).Once().
			On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(sendCoinsFromModuleToAccount).Return(nil).Once()
	})

	res, err = suite.ehandler(ctx, depositMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// after deposit into an overdrawn account, account and payments should be settled and closed (if sufficient balance is provided)
	eacc, err = suite.EscrowKeeper().GetAccount(ctx, deployment.ID.ToEscrowAccountID())
	require.NoError(t, err)
	require.NotNil(t, eacc)

	// find just created escrow payment
	epmnt, err = suite.EscrowKeeper().GetPayment(ctx, lid.ToEscrowPaymentID())
	require.NoError(t, err)
	require.NotNil(t, epmnt)

	// both escrow account and payment are expected to be in overdrawn state
	require.Equal(t, etypes.StateClosed, eacc.State.State)
	require.Equal(t, etypes.StateClosed, epmnt.State.State)

	require.True(t, eacc.State.Funds[0].Amount.IsZero())
	require.True(t, epmnt.State.Unsettled.Amount.IsZero())

	// at the end of the test module escrow account should be 0
	require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("uakt", 0)), escrowBalance)

	// at the end of the test module distribution account should be 10002uakt
	require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("uakt", 10002)), distrBalance)

	// at the end of the test provider account should be 10490098uakt
	require.Equal(t, sdk.NewInt64Coin("uakt", 10490098), balances[provider])

	// at the end of the test owner account should be 9499900uakt
	require.Equal(t, sdk.NewInt64Coin("uakt", 9499900), balances[owner.String()])
}

func TestMarketFullFlowCloseLease(t *testing.T) {
	defaultDeposit, err := dtypes.DefaultParams().MinDepositFor("uakt")
	require.NoError(t, err)

	suite := setupTestSuite(t)

	ctx := suite.Context()

	deployment := testutil.Deployment(t)
	group := testutil.DeploymentGroup(t, deployment.ID, 0)
	group.GroupSpec.Resources = testutil.Resources(t)

	owner := sdk.MustAccAddressFromBech32(deployment.ID.Owner)

	dmsg := &dtypes.MsgCreateDeployment{
		ID:     deployment.ID,
		Groups: dtypes.GroupSpecs{group.GroupSpec},
		Deposit: deposit.Deposit{
			Amount:  defaultDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()
		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, owner, emodule.ModuleName, sdk.Coins{dmsg.Deposit.Amount}).
			Return(nil).Once()
	})

	res, err := suite.dhandler(ctx, dmsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	order, found := suite.MarketKeeper().GetOrder(ctx, v1.OrderID{
		Owner: deployment.ID.Owner,
		DSeq:  deployment.ID.DSeq,
		GSeq:  1,
		OSeq:  1,
	})

	require.True(t, found)

	// we can create provider via keeper in this test
	provider := suite.createProvider(group.GroupSpec.Requirements.Attributes).Owner
	providerAddr, err := sdk.AccAddressFromBech32(provider)
	require.NoError(t, err)

	bmsg := &types.MsgCreateBid{
		ID:    v1.MakeBidID(order.ID, providerAddr),
		Price: sdk.NewDecCoin(testutil.CoinDenom, sdkmath.NewInt(1)),
		Deposit: deposit.Deposit{
			Amount:  types.DefaultBidMinDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()

		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, providerAddr, emodule.ModuleName, sdk.Coins{types.DefaultBidMinDeposit}).
			Return(nil).Once()
	})
	res, err = suite.handler(ctx, bmsg)
	require.NotNil(t, res)
	require.NoError(t, err)

	bid := v1.MakeBidID(order.ID, providerAddr)

	t.Run("ensure bid event created", func(t *testing.T) {
		iev, err := sdk.ParseTypedEvent(res.Events[3])
		require.NoError(t, err)
		require.IsType(t, &v1.EventBidCreated{}, iev)

		dev := iev.(*v1.EventBidCreated)

		require.Equal(t, bid, dev.ID)
	})

	_, found = suite.MarketKeeper().GetBid(ctx, bid)
	require.True(t, found)

	lmsg := &types.MsgCreateLease{
		BidID: bid,
	}

	lid := v1.MakeLeaseID(bid)
	res, err = suite.handler(ctx, lmsg)
	require.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure lease event created", func(t *testing.T) {
		iev, err := sdk.ParseTypedEvent(res.Events[4])
		require.NoError(t, err)
		require.IsType(t, &v1.EventLeaseCreated{}, iev)

		dev := iev.(*v1.EventLeaseCreated)

		require.Equal(t, lid, dev.ID)
	})

	// find just created escrow account
	eacc, err := suite.EscrowKeeper().GetAccount(ctx, deployment.ID.ToEscrowAccountID())
	require.NoError(t, err)
	require.NotNil(t, eacc)

	// find just created escrow payment
	epmnt, err := suite.EscrowKeeper().GetPayment(ctx, lid.ToEscrowPaymentID())
	require.NoError(t, err)
	require.NotNil(t, epmnt)

	blocks := eacc.State.Funds[0].Amount.Quo(epmnt.State.Rate.Amount)

	ctx = ctx.WithBlockHeight(blocks.TruncateInt64() + 100)

	dcmsg := &types.MsgCloseLease{
		ID: lid,
	}

	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()
		// this will trigger settlement and payoff if the deposit balance is sufficient
		// 1nd transfer: take rate 10000uakt = 500,000 * 0.02
		// 2nd transfer: returned bid deposit back to the provider
		// 3rd transfer: payment withdraw of 490,000uakt
		takeRate := sdkmath.LegacyNewDecFromInt(defaultDeposit.Amount)
		takeRate.MulMut(sdkmath.LegacyMustNewDecFromStr("0.02"))

		bkeeper.
			On("SendCoinsFromModuleToModule", mock.Anything, emodule.ModuleName, distrtypes.ModuleName, sdk.Coins{sdk.NewCoin(defaultDeposit.Denom, takeRate.TruncateInt())}).
			Return(nil).Once().
			On("SendCoinsFromModuleToAccount", mock.Anything, emodule.ModuleName, providerAddr, sdk.NewCoins(testutil.AkashCoin(t, 500_000))).
			Return(nil).Once().
			On("SendCoinsFromModuleToAccount", mock.Anything, emodule.ModuleName, providerAddr, sdk.NewCoins(testutil.AkashCoin(t, 490_000))).
			Return(nil).Once()
	})

	res, err = suite.handler(ctx, dcmsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	eacc, err = suite.EscrowKeeper().GetAccount(ctx, deployment.ID.ToEscrowAccountID())
	require.NoError(t, err)
	require.NotNil(t, eacc)

	// find just created escrow payment
	epmnt, err = suite.EscrowKeeper().GetPayment(ctx, lid.ToEscrowPaymentID())
	require.NoError(t, err)
	require.NotNil(t, epmnt)

	// both escrow account and payment are expected to be in overdrawn state
	require.Equal(t, etypes.StateOverdrawn, eacc.State.State)
	require.Equal(t, etypes.StateOverdrawn, epmnt.State.State)

	expectedOverdraft := epmnt.State.Rate.Amount.MulInt64(100)
	require.True(t, eacc.State.Funds[0].Amount.IsNegative())
	require.Equal(t, expectedOverdraft, eacc.State.Funds[0].Amount.Abs())
	require.Equal(t, expectedOverdraft, epmnt.State.Unsettled.Amount)

	// lease must be in closed state
	lease, found := suite.MarketKeeper().GetLease(ctx, lid)
	require.True(t, found)
	require.Equal(t, v1.LeaseClosed, lease.State)

	// lease must be in closed state
	bidObj, found := suite.MarketKeeper().GetBid(ctx, bid)
	require.True(t, found)
	require.Equal(t, types.BidClosed, bidObj.State)

	// deployment must be in closed state
	depl, found := suite.DeploymentKeeper().GetDeployment(ctx, lid.DeploymentID())
	require.True(t, found)
	require.Equal(t, dv1.DeploymentClosed, depl.State)

	// should ont be able to close escrow account in overdrawn state
	err = suite.EscrowKeeper().AccountClose(ctx, deployment.ID.ToEscrowAccountID())
	require.NoError(t, err)

	// both account and payment should remain in overdrawn state
	eacc, err = suite.EscrowKeeper().GetAccount(ctx, deployment.ID.ToEscrowAccountID())
	require.NoError(t, err)
	require.NotNil(t, eacc)

	// find just created escrow payment
	epmnt, err = suite.EscrowKeeper().GetPayment(ctx, lid.ToEscrowPaymentID())
	require.NoError(t, err)
	require.NotNil(t, epmnt)

	// both escrow account and payment are expected to be in overdrawn state
	require.Equal(t, etypes.StateOverdrawn, eacc.State.State)
	require.Equal(t, etypes.StateOverdrawn, epmnt.State.State)

	require.Equal(t, expectedOverdraft, eacc.State.Funds[0].Amount.Abs())
	require.Equal(t, expectedOverdraft, epmnt.State.Unsettled.Amount)

	depositMsg := &ev1.MsgAccountDeposit{
		Signer: owner.String(),
		ID:     deployment.ID.ToEscrowAccountID(),
		Deposit: deposit.Deposit{
			Amount:  sdk.NewCoin(defaultDeposit.Denom, expectedOverdraft.TruncateInt()),
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()
		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, owner, emodule.ModuleName, sdk.Coins{depositMsg.Deposit.Amount}).
			Return(nil).Once().
			On("SendCoinsFromModuleToModule", mock.Anything, emodule.ModuleName, distrtypes.ModuleName, sdk.Coins{sdk.NewInt64Coin(depositMsg.Deposit.Amount.Denom, 2)}).
			Return(nil).Once().
			On("SendCoinsFromModuleToAccount", mock.Anything, emodule.ModuleName, providerAddr, sdk.NewCoins(testutil.AkashCoin(t, 98))).
			Return(nil).Once()
	})

	res, err = suite.ehandler(ctx, depositMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// after deposit into overdrawn account, account and payments should be settled and closed (if sufficient balance is provided)
	eacc, err = suite.EscrowKeeper().GetAccount(ctx, deployment.ID.ToEscrowAccountID())
	require.NoError(t, err)
	require.NotNil(t, eacc)

	// find just created escrow payment
	epmnt, err = suite.EscrowKeeper().GetPayment(ctx, lid.ToEscrowPaymentID())
	require.NoError(t, err)
	require.NotNil(t, epmnt)

	// both escrow account and payment are expected to be in overdrawn state
	require.Equal(t, etypes.StateClosed, eacc.State.State)
	require.Equal(t, etypes.StateClosed, epmnt.State.State)

	require.True(t, eacc.State.Funds[0].Amount.IsZero())
	require.True(t, epmnt.State.Unsettled.Amount.IsZero())
}

func TestMarketFullFlowCloseBid(t *testing.T) {
	defaultDeposit, err := dtypes.DefaultParams().MinDepositFor("uakt")
	require.NoError(t, err)

	suite := setupTestSuite(t)

	ctx := suite.Context()

	deployment := testutil.Deployment(t)
	group := testutil.DeploymentGroup(t, deployment.ID, 0)
	group.GroupSpec.Resources = testutil.Resources(t)

	owner := sdk.MustAccAddressFromBech32(deployment.ID.Owner)

	dmsg := &dtypes.MsgCreateDeployment{
		ID:     deployment.ID,
		Groups: dtypes.GroupSpecs{group.GroupSpec},
		Deposit: deposit.Deposit{
			Amount:  defaultDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()
		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, owner, emodule.ModuleName, sdk.Coins{dmsg.Deposit.Amount}).
			Return(nil).Once()
	})

	res, err := suite.dhandler(ctx, dmsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	order, found := suite.MarketKeeper().GetOrder(ctx, v1.OrderID{
		Owner: deployment.ID.Owner,
		DSeq:  deployment.ID.DSeq,
		GSeq:  1,
		OSeq:  1,
	})

	require.True(t, found)

	// we can create provider via keeper in this test
	provider := suite.createProvider(group.GroupSpec.Requirements.Attributes).Owner
	providerAddr, err := sdk.AccAddressFromBech32(provider)
	require.NoError(t, err)

	bmsg := &types.MsgCreateBid{
		ID:    v1.MakeBidID(order.ID, providerAddr),
		Price: sdk.NewDecCoin(testutil.CoinDenom, sdkmath.NewInt(1)),
		Deposit: deposit.Deposit{
			Amount:  types.DefaultBidMinDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()

		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, providerAddr, emodule.ModuleName, sdk.Coins{types.DefaultBidMinDeposit}).
			Return(nil).Once()
	})
	res, err = suite.handler(ctx, bmsg)
	require.NotNil(t, res)
	require.NoError(t, err)

	bid := v1.MakeBidID(order.ID, providerAddr)

	t.Run("ensure bid event created", func(t *testing.T) {
		iev, err := sdk.ParseTypedEvent(res.Events[3])
		require.NoError(t, err)
		require.IsType(t, &v1.EventBidCreated{}, iev)

		dev := iev.(*v1.EventBidCreated)

		require.Equal(t, bid, dev.ID)
	})

	_, found = suite.MarketKeeper().GetBid(ctx, bid)
	require.True(t, found)

	lmsg := &types.MsgCreateLease{
		BidID: bid,
	}

	lid := v1.MakeLeaseID(bid)
	res, err = suite.handler(ctx, lmsg)
	require.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure lease event created", func(t *testing.T) {
		iev, err := sdk.ParseTypedEvent(res.Events[4])
		require.NoError(t, err)
		require.IsType(t, &v1.EventLeaseCreated{}, iev)

		dev := iev.(*v1.EventLeaseCreated)

		require.Equal(t, lid, dev.ID)
	})

	// find just created escrow account
	eacc, err := suite.EscrowKeeper().GetAccount(ctx, deployment.ID.ToEscrowAccountID())
	require.NoError(t, err)
	require.NotNil(t, eacc)

	// find just created escrow payment
	epmnt, err := suite.EscrowKeeper().GetPayment(ctx, lid.ToEscrowPaymentID())
	require.NoError(t, err)
	require.NotNil(t, epmnt)

	blocks := eacc.State.Funds[0].Amount.Quo(epmnt.State.Rate.Amount)

	ctx = ctx.WithBlockHeight(blocks.TruncateInt64() + 100)

	dcmsg := &types.MsgCloseBid{
		ID: bid,
	}

	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()
		// this will trigger settlement and payoff if the deposit balance is sufficient
		// 1nd transfer: take rate 10000uakt = 500,000 * 0.02
		// 2nd transfer: returned bid deposit back to the provider
		// 3rd transfer: payment withdraw of 490,000uakt
		takeRate := sdkmath.LegacyNewDecFromInt(defaultDeposit.Amount)
		takeRate.MulMut(sdkmath.LegacyMustNewDecFromStr("0.02"))

		bkeeper.
			On("SendCoinsFromModuleToModule", mock.Anything, emodule.ModuleName, distrtypes.ModuleName, sdk.Coins{sdk.NewCoin(defaultDeposit.Denom, takeRate.TruncateInt())}).
			Return(nil).Once().
			On("SendCoinsFromModuleToAccount", mock.Anything, emodule.ModuleName, providerAddr, sdk.NewCoins(testutil.AkashCoin(t, 500_000))).
			Return(nil).Once().
			On("SendCoinsFromModuleToAccount", mock.Anything, emodule.ModuleName, providerAddr, sdk.NewCoins(testutil.AkashCoin(t, 490_000))).
			Return(nil).Once()
	})

	res, err = suite.handler(ctx, dcmsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	eacc, err = suite.EscrowKeeper().GetAccount(ctx, deployment.ID.ToEscrowAccountID())
	require.NoError(t, err)
	require.NotNil(t, eacc)

	// find just created escrow payment
	epmnt, err = suite.EscrowKeeper().GetPayment(ctx, lid.ToEscrowPaymentID())
	require.NoError(t, err)
	require.NotNil(t, epmnt)

	// both escrow account and payment are expected to be in overdrawn state
	require.Equal(t, etypes.StateOverdrawn, eacc.State.State)
	require.Equal(t, etypes.StateOverdrawn, epmnt.State.State)

	expectedOverdraft := epmnt.State.Rate.Amount.MulInt64(100)
	require.True(t, eacc.State.Funds[0].Amount.IsNegative())
	require.Equal(t, expectedOverdraft, eacc.State.Funds[0].Amount.Abs())
	require.Equal(t, expectedOverdraft, epmnt.State.Unsettled.Amount)

	// lease must be in closed state
	lease, found := suite.MarketKeeper().GetLease(ctx, lid)
	require.True(t, found)
	require.Equal(t, v1.LeaseClosed, lease.State)

	// lease must be in closed state
	bidObj, found := suite.MarketKeeper().GetBid(ctx, bid)
	require.True(t, found)
	require.Equal(t, types.BidClosed, bidObj.State)

	// deployment must be in closed state
	depl, found := suite.DeploymentKeeper().GetDeployment(ctx, lid.DeploymentID())
	require.True(t, found)
	require.Equal(t, dv1.DeploymentClosed, depl.State)

	// should ont be able to close escrow account in overdrawn state
	err = suite.EscrowKeeper().AccountClose(ctx, deployment.ID.ToEscrowAccountID())
	require.NoError(t, err)

	// both account and payment should remain in overdrawn state
	eacc, err = suite.EscrowKeeper().GetAccount(ctx, deployment.ID.ToEscrowAccountID())
	require.NoError(t, err)
	require.NotNil(t, eacc)

	// find just created escrow payment
	epmnt, err = suite.EscrowKeeper().GetPayment(ctx, lid.ToEscrowPaymentID())
	require.NoError(t, err)
	require.NotNil(t, epmnt)

	// both escrow account and payment are expected to be in overdrawn state
	require.Equal(t, etypes.StateOverdrawn, eacc.State.State)
	require.Equal(t, etypes.StateOverdrawn, epmnt.State.State)

	require.Equal(t, expectedOverdraft, eacc.State.Funds[0].Amount.Abs())
	require.Equal(t, expectedOverdraft, epmnt.State.Unsettled.Amount)

	depositMsg := &ev1.MsgAccountDeposit{
		Signer: owner.String(),
		ID:     deployment.ID.ToEscrowAccountID(),
		Deposit: deposit.Deposit{
			Amount:  sdk.NewCoin(defaultDeposit.Denom, expectedOverdraft.TruncateInt()),
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()
		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, owner, emodule.ModuleName, sdk.Coins{depositMsg.Deposit.Amount}).
			Return(nil).Once().
			On("SendCoinsFromModuleToModule", mock.Anything, emodule.ModuleName, distrtypes.ModuleName, sdk.Coins{sdk.NewInt64Coin(depositMsg.Deposit.Amount.Denom, 2)}).
			Return(nil).Once().
			On("SendCoinsFromModuleToAccount", mock.Anything, emodule.ModuleName, providerAddr, sdk.NewCoins(testutil.AkashCoin(t, 98))).
			Return(nil).Once()
	})

	res, err = suite.ehandler(ctx, depositMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// after deposit into overdrawn account, account and payments should be settled and closed (if sufficient balance is provided)
	eacc, err = suite.EscrowKeeper().GetAccount(ctx, deployment.ID.ToEscrowAccountID())
	require.NoError(t, err)
	require.NotNil(t, eacc)

	// find just created escrow payment
	epmnt, err = suite.EscrowKeeper().GetPayment(ctx, lid.ToEscrowPaymentID())
	require.NoError(t, err)
	require.NotNil(t, epmnt)

	// both escrow account and payment are expected to be in overdrawn state
	require.Equal(t, etypes.StateClosed, eacc.State.State)
	require.Equal(t, etypes.StateClosed, epmnt.State.State)

	require.True(t, eacc.State.Funds[0].Amount.IsZero())
	require.True(t, epmnt.State.Unsettled.Amount.IsZero())
}

func TestCreateBidValid(t *testing.T) {
	suite := setupTestSuite(t)

	order, gspec := suite.createOrder(testutil.Resources(t))

	provider := suite.createProvider(gspec.Requirements.Attributes).Owner
	providerAddr, err := sdk.AccAddressFromBech32(provider)
	require.NoError(t, err)

	msg := &types.MsgCreateBid{
		ID:    v1.MakeBidID(order.ID, providerAddr),
		Price: sdk.NewDecCoin(testutil.CoinDenom, sdkmath.NewInt(1)),
		Deposit: deposit.Deposit{
			Amount:  types.DefaultBidMinDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

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

	res, err := suite.handler(suite.Context(), msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	bid := v1.MakeBidID(order.ID, providerAddr)

	t.Run("ensure event created", func(t *testing.T) {
		iev, err := sdk.ParseTypedEvent(res.Events[3])
		require.NoError(t, err)

		require.IsType(t, &v1.EventBidCreated{}, iev)

		dev := iev.(*v1.EventBidCreated)

		require.Equal(t, bid, dev.ID)
	})

	_, found := suite.MarketKeeper().GetBid(suite.Context(), bid)
	require.True(t, found)
}

func TestCreateBidInvalidPrice(t *testing.T) {
	suite := setupTestSuite(t)
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

	order, gspec := suite.createOrder(nil)

	provider := suite.createProvider(gspec.Requirements.Attributes).Owner
	providerAddr, err := sdk.AccAddressFromBech32(provider)
	require.NoError(t, err)

	msg := &types.MsgCreateBid{
		ID:    v1.MakeBidID(order.ID, providerAddr),
		Price: sdk.DecCoin{},
	}
	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)

	_, found := suite.MarketKeeper().GetBid(suite.Context(), v1.MakeBidID(order.ID, providerAddr))
	require.False(t, found)
}

func TestCreateBidNonExistingOrder(t *testing.T) {
	suite := setupTestSuite(t)
	orderID := v1.OrderID{Owner: testutil.AccAddress(t).String()}
	providerAddr := testutil.AccAddress(t)

	msg := &types.MsgCreateBid{
		ID:    v1.MakeBidID(orderID, providerAddr),
		Price: testutil.AkashDecCoinRandom(t),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)

	_, found := suite.MarketKeeper().GetBid(suite.Context(), v1.MakeBidID(orderID, providerAddr))
	require.False(t, found)
}

func TestCreateBidClosedOrder(t *testing.T) {
	suite := setupTestSuite(t)
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

	order, gspec := suite.createOrder(nil)
	provider := suite.createProvider(gspec.Requirements.Attributes).Owner
	providerAddr, err := sdk.AccAddressFromBech32(provider)

	require.NoError(t, err)

	_ = suite.MarketKeeper().OnOrderClosed(suite.Context(), order)

	msg := &types.MsgCreateBid{
		ID:    v1.MakeBidID(order.ID, providerAddr),
		Price: sdk.NewDecCoin(testutil.CoinDenom, sdkmath.NewInt(math.MaxInt64)),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func TestCreateBidOverprice(t *testing.T) {
	suite := setupTestSuite(t)
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

	resources := dtypes.ResourceUnits{
		{
			Price: sdk.NewDecCoin(testutil.CoinDenom, sdkmath.NewInt(1)),
		},
	}
	order, gspec := suite.createOrder(resources)

	providerAddr, err := sdk.AccAddressFromBech32(suite.createProvider(gspec.Requirements.Attributes).Owner)
	require.NoError(t, err)

	msg := &types.MsgCreateBid{
		ID:    v1.MakeBidID(order.ID, providerAddr),
		Price: sdk.NewDecCoin(testutil.CoinDenom, sdkmath.NewInt(math.MaxInt64)),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func TestCreateBidInvalidProvider(t *testing.T) {
	suite := setupTestSuite(t)
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

	order, _ := suite.createOrder(testutil.Resources(t))

	msg := &types.MsgCreateBid{
		ID:    v1.MakeBidID(order.ID, sdk.AccAddress{}),
		Price: sdk.NewDecCoin(testutil.CoinDenom, sdkmath.NewInt(1)),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func TestCreateBidInvalidAttributes(t *testing.T) {
	suite := setupTestSuite(t)
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

	order, _ := suite.createOrder(testutil.Resources(t))
	providerAddr, err := sdk.AccAddressFromBech32(suite.createProvider(nil).Owner)
	require.NoError(t, err)

	msg := &types.MsgCreateBid{
		ID:    v1.MakeBidID(order.ID, providerAddr),
		Price: sdk.NewDecCoin(testutil.CoinDenom, sdkmath.NewInt(1)),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func TestCreateBidAlreadyExists(t *testing.T) {
	suite := setupTestSuite(t)

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

	order, gspec := suite.createOrder(testutil.Resources(t))
	provider := suite.createProvider(gspec.Requirements.Attributes).Owner
	providerAddr, err := sdk.AccAddressFromBech32(provider)
	require.NoError(t, err)

	msg := &types.MsgCreateBid{
		ID:    v1.MakeBidID(order.ID, providerAddr),
		Price: sdk.NewDecCoin(testutil.CoinDenom, sdkmath.NewInt(1)),
		Deposit: deposit.Deposit{
			Amount:  types.DefaultBidMinDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	res, err := suite.handler(suite.Context(), msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	res, err = suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func TestCloseOrderNonExisting(t *testing.T) {
	t.Skip("TODO CLOSE LEASE")
	// suite := setupTestSuite(t)

	// dgroup := testutil.DeploymentGroup(suite.t, testutil.DeploymentID(suite.t), 0)
	// msg := &types.MsgCloseOrder{
	// 	OrderID: types.MakeOrderID(dgroup.ID(), 1),
	// }

	// res, err := suite.handler(suite.Context(), msg)
	// require.Nil(t, res)
	// require.Error(t, err)
}

func TestCloseOrderWithoutLease(t *testing.T) {
	t.Skip("TODO CLOSE LEASE")
	// suite := setupTestSuite(t)

	// order, _ := suite.createOrder(testutil.Resources(t))

	// msg := &types.MsgCloseOrder{
	// 	OrderID: order.ID(),
	// }

	// res, err := suite.handler(suite.Context(), msg)
	// require.Nil(t, res)
	// require.Error(t, err)
}

func TestCloseOrderValid(t *testing.T) {
	t.Skip("TODO CLOSE LEASE")
	// suite := setupTestSuite(t)

	// _, _, order := suite.createLease()

	// msg := &types.MsgCloseOrder{
	// 	OrderID: order.ID(),
	// }

	// res, err := suite.handler(suite.Context(), msg)
	// require.NotNil(t, res)
	// require.NoError(t, err)

	// t.Run("ensure event created", func(t *testing.T) {
	// 	iev := testutil.ParseMarketEvent(t, res.Events[3:4])
	// 	require.IsType(t, types.EventOrderClosed{}, iev)

	// 	dev := iev.(types.EventOrderClosed)

	// 	require.Equal(t, msg.OrderID, dev.ID)
	// })
}

func TestCloseBidNonExisting(t *testing.T) {
	suite := setupTestSuite(t)
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

	order, gspec := suite.createOrder(testutil.Resources(t))

	provider := suite.createProvider(gspec.Requirements.Attributes).Owner

	providerAddr, err := sdk.AccAddressFromBech32(provider)
	require.NoError(t, err)

	msg := &types.MsgCloseBid{
		ID: v1.MakeBidID(order.ID, providerAddr),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func TestCloseBidUnknownLease(t *testing.T) {
	suite := setupTestSuite(t)
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

	bid, _ := suite.createBid()

	suite.MarketKeeper().OnBidMatched(suite.Context(), bid)

	msg := &types.MsgCloseBid{
		ID: bid.ID,
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func TestCloseBidValid(t *testing.T) {
	suite := setupTestSuite(t)
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

	_, bid, _ := suite.createLease()

	msg := &types.MsgCloseBid{
		ID: bid.ID,
	}

	res, err := suite.handler(suite.Context(), msg)
	assert.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure event created", func(t *testing.T) {
		iev, err := sdk.ParseTypedEvent(res.Events[6])
		require.NoError(t, err)

		// iev := testutil.ParseMarketEvent(t, res.Events[3:4])
		require.IsType(t, &v1.EventBidClosed{}, iev)

		dev := iev.(*v1.EventBidClosed)

		require.Equal(t, msg.ID, dev.ID)
	})
}

func TestCloseBidWithStateOpen(t *testing.T) {
	suite := setupTestSuite(t)
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

	bid, _ := suite.createBid()

	msg := &types.MsgCloseBid{
		ID: bid.ID,
	}

	res, err := suite.handler(suite.Context(), msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure event created", func(t *testing.T) {
		iev, err := sdk.ParseTypedEvent(res.Events[3])
		require.NoError(t, err)

		// iev := testutil.ParseMarketEvent(t, res.Events[2:])
		require.IsType(t, &v1.EventBidClosed{}, iev)

		dev := iev.(*v1.EventBidClosed)

		require.Equal(t, msg.ID, dev.ID)
	})
}

func TestCloseBidNotActiveLease(t *testing.T) {
	t.Skip("TODO CLOSE LEASE")
	// suite := setupTestSuite(t)

	// lease, bid, _ := suite.createLease()

	// suite.MarketKeeper().OnLeaseClosed(suite.Context(), types.Lease{
	// 	LeaseID: lease,
	// })
	// msg := &types.MsgCloseBid{
	// 	BidID: bid.ID(),
	// }

	// res, err := suite.handler(suite.Context(), msg)
	// require.Nil(t, res)
	// require.Error(t, err)
}

func TestCloseBidUnknownOrder(t *testing.T) {
	suite := setupTestSuite(t)

	group := testutil.DeploymentGroup(t, testutil.DeploymentID(t), 0)
	orderID := v1.MakeOrderID(group.ID, 1)
	provider := testutil.AccAddress(t)
	price := sdk.NewDecCoin(testutil.CoinDenom, sdkmath.NewInt(int64(rand.Uint16())))
	roffer := types.ResourceOfferFromRU(group.GroupSpec.Resources)

	bidID := v1.MakeBidID(orderID, provider)
	bid, err := suite.MarketKeeper().CreateBid(suite.Context(), bidID, price, roffer)
	require.NoError(t, err)

	err = suite.MarketKeeper().CreateLease(suite.Context(), bid)
	require.NoError(t, err)

	msg := &types.MsgCloseBid{
		ID: bid.ID,
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func (st *testSuite) createLease() (v1.LeaseID, types.Bid, types.Order) {
	st.t.Helper()
	bid, order := st.createBid()

	err := st.MarketKeeper().CreateLease(st.Context(), bid)
	require.NoError(st.t, err)

	st.MarketKeeper().OnBidMatched(st.Context(), bid)
	st.MarketKeeper().OnOrderMatched(st.Context(), order)

	lid := v1.MakeLeaseID(bid.ID)
	return lid, bid, order
}

func (st *testSuite) createBid() (types.Bid, types.Order) {
	st.t.Helper()
	order, gspec := st.createOrder(testutil.Resources(st.t))
	provider := testutil.AccAddress(st.t)
	price := sdk.NewDecCoin(testutil.CoinDenom, sdkmath.NewInt(int64(rand.Uint16())))
	roffer := types.ResourceOfferFromRU(gspec.Resources)

	bidID := v1.MakeBidID(order.ID, provider)

	bid, err := st.MarketKeeper().CreateBid(st.Context(), bidID, price, roffer)
	require.NoError(st.t, err)
	require.Equal(st.t, order.ID, bid.ID.OrderID())
	require.Equal(st.t, price, bid.Price)
	require.Equal(st.t, provider.String(), bid.ID.Provider)
	return bid, order
}

func (st *testSuite) createOrder(resources dtypes.ResourceUnits) (types.Order, dtypes.GroupSpec) {
	st.t.Helper()

	deployment := testutil.Deployment(st.t)
	group := testutil.DeploymentGroup(st.t, deployment.ID, 0)
	group.GroupSpec.Resources = resources

	err := st.DeploymentKeeper().Create(st.Context(), deployment, []dtypes.Group{group})
	require.NoError(st.t, err)

	order, err := st.MarketKeeper().CreateOrder(st.Context(), group.ID, group.GroupSpec)
	require.NoError(st.t, err)
	require.Equal(st.t, group.ID, order.ID.GroupID())
	require.Equal(st.t, uint32(1), order.ID.OSeq)
	require.Equal(st.t, types.OrderOpen, order.State)

	return order, group.GroupSpec
}

func (st *testSuite) createProvider(attr attr.Attributes) ptypes.Provider {
	st.t.Helper()

	prov := ptypes.Provider{
		Owner:      testutil.AccAddress(st.t).String(),
		HostURI:    "thinker://tailor.soldier?sailor",
		Attributes: attr,
	}

	err := st.ProviderKeeper().Create(st.Context(), prov)
	require.NoError(st.t, err)

	return prov
}

func (st *testSuite) createDeployment() (dv1.Deployment, dtypes.Groups) {
	st.t.Helper()

	deployment := testutil.Deployment(st.t)
	group := testutil.DeploymentGroup(st.t, deployment.ID, 0)
	group.GroupSpec.Resources = dtypes.ResourceUnits{
		{
			Resources: testutil.ResourceUnits(st.t),
			Count:     1,
			Price:     testutil.AkashDecCoinRandom(st.t),
		},
	}
	groups := dtypes.Groups{
		group,
	}

	for i := range groups {
		groups[i].State = dtypes.GroupOpen
	}

	return deployment, groups
}
