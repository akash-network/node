package handler_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	mtypes "pkg.akt.dev/go/node/market/v1"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdktestdata "github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/node/deployment/v1beta4"
	ev1 "pkg.akt.dev/go/node/escrow/v1"
	deposit "pkg.akt.dev/go/node/types/deposit/v1"
	"pkg.akt.dev/go/testutil"

	cmocks "pkg.akt.dev/node/v2/testutil/cosmos/mocks"
	"pkg.akt.dev/node/v2/testutil/state"
	"pkg.akt.dev/node/v2/x/deployment/handler"
	"pkg.akt.dev/node/v2/x/deployment/keeper"
	ehandler "pkg.akt.dev/node/v2/x/escrow/handler"
	mkeeper "pkg.akt.dev/node/v2/x/market/keeper"
)

type testSuite struct {
	*state.TestSuite
	t              *testing.T
	ctx            sdk.Context
	mkeeper        mkeeper.IKeeper
	dkeeper        keeper.IKeeper
	authzKeeper    handler.AuthzKeeper
	bankKeeper     handler.BankKeeper
	owner          sdk.AccAddress
	granter        sdk.AccAddress
	dhandler       baseapp.MsgServiceHandler
	ehandler       baseapp.MsgServiceHandler
	defaultDeposit sdk.Coin
}

func setupTestSuite(t *testing.T) *testSuite {
	defaultDeposit, err := v1beta4.DefaultParams().MinDepositFor("uakt")
	require.NoError(t, err)

	owner := testutil.AccAddress(t)
	granter := testutil.AccAddress(t)
	authzKeeper := &cmocks.AuthzKeeper{}
	bankKeeper := &cmocks.BankKeeper{}
	msgTypeUrl := (&ev1.DepositAuthorization{}).MsgTypeURL()

	authzKeeper.
		On("GetGranteeGrantsByMsgType", mock.Anything, owner, msgTypeUrl, mock.Anything).
		Run(func(args mock.Arguments) {
			onGrant := args.Get(3).(authzkeeper.OnGrantFn)
			authorization := &ev1.DepositAuthorization{
				Scopes:     ev1.DepositAuthorizationScopes{ev1.DepositScopeDeployment},
				SpendLimit: defaultDeposit.Add(defaultDeposit),
			}

			_ = onGrant(context.TODO(), granter, authorization, &time.Time{})
		}).Once().
		On("GetGranteeGrantsByMsgType", mock.Anything, owner, msgTypeUrl, mock.Anything).
		Run(func(args mock.Arguments) {
			onGrant := args.Get(3).(authzkeeper.OnGrantFn)
			authorization := &ev1.DepositAuthorization{
				Scopes:     ev1.DepositAuthorizationScopes{ev1.DepositScopeDeployment},
				SpendLimit: defaultDeposit,
			}

			_ = onGrant(context.TODO(), granter, authorization, &time.Time{})
		}).Once().
		On("GetGranteeGrantsByMsgType", mock.Anything, owner, msgTypeUrl, mock.Anything).
		Run(func(args mock.Arguments) {
			onGrant := args.Get(3).(authzkeeper.OnGrantFn)
			authorization := &ev1.DepositAuthorization{
				Scopes:     ev1.DepositAuthorizationScopes{ev1.DepositScopeDeployment},
				SpendLimit: defaultDeposit,
			}

			_ = onGrant(context.TODO(), granter, authorization, &time.Time{})
		}).Once().
		On("GetGranteeGrantsByMsgType", mock.Anything, owner, msgTypeUrl, mock.Anything).
		Run(func(args mock.Arguments) {
			onGrant := args.Get(3).(authzkeeper.OnGrantFn)
			authorization := &ev1.DepositAuthorization{
				Scopes:     ev1.DepositAuthorizationScopes{ev1.DepositScopeDeployment},
				SpendLimit: defaultDeposit,
			}

			_ = onGrant(context.TODO(), granter, authorization, &time.Time{})
		}).
		Once().
		On("GetAuthorization", mock.Anything, mock.Anything, mock.Anything, msgTypeUrl).
		Return(nil, nil)
	authzKeeper.
		On("DeleteGrant", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
	authzKeeper.
		On("SaveGrant", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	bankKeeper.
		On("SpendableCoin", mock.Anything, mock.Anything, mock.Anything).
		Return(sdk.NewInt64Coin("uakt", 10000000))
	bankKeeper.
		On("SendCoinsFromAccountToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
	bankKeeper.
		On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
	bankKeeper.
		On("SendCoinsFromModuleToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	keepers := state.Keepers{
		Authz: authzKeeper,
		Bank:  bankKeeper,
	}
	ssuite := state.SetupTestSuiteWithKeepers(t, keepers)

	suite := &testSuite{
		TestSuite:      ssuite,
		t:              t,
		ctx:            ssuite.Context(),
		mkeeper:        ssuite.MarketKeeper(),
		dkeeper:        ssuite.DeploymentKeeper(),
		authzKeeper:    ssuite.AuthzKeeper(),
		bankKeeper:     ssuite.BankKeeper(),
		owner:          owner,
		granter:        granter,
		defaultDeposit: defaultDeposit,
	}

	suite.dhandler = handler.NewHandler(suite.dkeeper, suite.mkeeper, ssuite.EscrowKeeper())
	suite.ehandler = ehandler.NewHandler(suite.EscrowKeeper(), suite.authzKeeper, suite.BankKeeper())

	return suite
}

func TestProviderBadMessageType(t *testing.T) {
	suite := setupTestSuite(t)

	res, err := suite.dhandler(suite.ctx, sdk.Msg(sdktestdata.NewTestMsg()))
	require.Nil(t, res)
	require.Error(t, err)
	require.True(t, errors.Is(err, sdkerrors.ErrUnknownRequest))
}

func TestCreateDeployment(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()

	msg := &v1beta4.MsgCreateDeployment{
		ID:     deployment.ID,
		Groups: make(v1beta4.GroupSpecs, 0, len(groups)),
		Deposit: deposit.Deposit{
			Amount:  suite.defaultDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	for _, group := range groups {
		msg.Groups = append(msg.Groups, group.GroupSpec)
	}

	res, err := suite.dhandler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	t.Run("ensure event created", func(t *testing.T) {
		iev, err := sdk.ParseTypedEvent(res.Events[0])
		require.NoError(t, err)
		require.IsType(t, &v1.EventDeploymentCreated{}, iev)

		dev := iev.(*v1.EventDeploymentCreated)

		require.Equal(t, msg.ID, dev.ID)
	})

	deploymentResult, exists := suite.dkeeper.GetDeployment(suite.ctx, deployment.ID)
	require.True(t, exists)
	require.Equal(t, deploymentResult.Hash, msg.Hash)

	groupsResult := suite.dkeeper.GetGroups(suite.ctx, deployment.ID)
	require.NotEmpty(t, groupsResult)
	require.Equal(t, len(groupsResult), len(groups))

	for i, g := range groupsResult {
		require.Equal(t, groups[i].GetName(), g.GetName())
	}

	res, err = suite.dhandler(suite.ctx, msg)
	require.EqualError(t, err, v1.ErrDeploymentExists.Error())
	require.Nil(t, res)
}

func TestCreateDeploymentEmptyGroups(t *testing.T) {
	suite := setupTestSuite(t)

	deployment := testutil.Deployment(suite.t)

	msg := &v1beta4.MsgCreateDeployment{
		ID: deployment.ID,
		Deposit: deposit.Deposit{
			Amount:  suite.defaultDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	res, err := suite.dhandler(suite.ctx, msg)
	require.Nil(t, res)
	require.True(t, errors.Is(err, v1.ErrInvalidGroups))
}

func TestUpdateDeploymentNonExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment := testutil.Deployment(suite.t)

	msg := &v1beta4.MsgUpdateDeployment{
		ID: deployment.ID,
	}

	res, err := suite.dhandler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, v1.ErrDeploymentNotFound.Error())
}

func TestUpdateDeploymentExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()

	msgGroupSpecs := make(v1beta4.GroupSpecs, 0)
	for _, g := range groups {
		msgGroupSpecs = append(msgGroupSpecs, g.GroupSpec)
	}
	require.NotEmpty(t, msgGroupSpecs)
	require.Equal(t, len(msgGroupSpecs), 1)

	msg := &v1beta4.MsgCreateDeployment{
		ID:     deployment.ID,
		Groups: msgGroupSpecs,
		Hash:   testutil.DefaultDeploymentHash[:],
		Deposit: deposit.Deposit{
			Amount:  suite.defaultDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	res, err := suite.dhandler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	t.Run("assert deployment version", func(t *testing.T) {
		d, ok := suite.dkeeper.GetDeployment(suite.ctx, deployment.ID)
		require.True(t, ok)
		assert.Equal(t, d.Hash, testutil.DefaultDeploymentHash[:])
	})

	// Change the version
	depSum := sha256.Sum256(testutil.DefaultDeploymentHash[:])

	msgUpdate := &v1beta4.MsgUpdateDeployment{
		ID:   msg.ID,
		Hash: depSum[:],
	}
	res, err = suite.dhandler(suite.ctx, msgUpdate)
	require.NoError(t, err)
	require.NotNil(t, res)

	t.Run("ensure event created", func(t *testing.T) {
		iev, err := sdk.ParseTypedEvent(res.Events[2])
		require.NoError(t, err)
		require.IsType(t, &v1.EventDeploymentUpdated{}, iev)

		dev := iev.(*v1.EventDeploymentUpdated)

		require.Equal(t, msg.ID, dev.ID)
	})

	t.Run("assert version updated", func(t *testing.T) {
		d, ok := suite.dkeeper.GetDeployment(suite.ctx, deployment.ID)
		require.True(t, ok)
		assert.Equal(t, d.Hash, depSum[:])
	})

	// Run the same update, should fail since nothing is different
	res, err = suite.dhandler(suite.ctx, msgUpdate)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid: deployment hash")

}

func TestCloseDeploymentNonExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment := testutil.Deployment(suite.t)

	msg := &v1beta4.MsgCloseDeployment{
		ID: deployment.ID,
	}

	res, err := suite.dhandler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, v1.ErrDeploymentNotFound.Error())
}

func TestCloseDeploymentExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()

	msg := &v1beta4.MsgCreateDeployment{
		ID:     deployment.ID,
		Groups: make(v1beta4.GroupSpecs, 0, len(groups)),
		Deposit: deposit.Deposit{
			Amount:  suite.defaultDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	for _, group := range groups {
		msg.Groups = append(msg.Groups, group.GroupSpec)
	}

	res, err := suite.dhandler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	t.Run("ensure event created", func(t *testing.T) {
		iev, err := sdk.ParseTypedEvent(res.Events[0])
		require.NoError(t, err)

		require.IsType(t, &v1.EventDeploymentCreated{}, iev)

		dev := iev.(*v1.EventDeploymentCreated)

		require.Equal(t, msg.ID, dev.ID)
	})

	msgClose := &v1beta4.MsgCloseDeployment{
		ID: deployment.ID,
	}

	res, err = suite.dhandler(suite.ctx, msgClose)
	require.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure event close", func(t *testing.T) {
		iev, err := sdk.ParseTypedEvent(res.Events[2])
		require.NoError(t, err)

		require.IsType(t, &v1.EventDeploymentClosed{}, iev)

		dev := iev.(*v1.EventDeploymentClosed)

		require.Equal(t, msg.ID, dev.ID)
	})

	res, err = suite.dhandler(suite.ctx, msgClose)
	require.Nil(t, res)
	require.EqualError(t, err, v1.ErrDeploymentClosed.Error())
}

func TestFundedDeployment(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()
	deployment.ID.Owner = suite.owner.String()

	// create a funded deployment
	msg := &v1beta4.MsgCreateDeployment{
		ID:     deployment.ID,
		Groups: make(v1beta4.GroupSpecs, 0, len(groups)),
		Deposit: deposit.Deposit{
			Amount:  suite.defaultDeposit,
			Sources: deposit.Sources{deposit.SourceGrant},
		},
	}

	for _, group := range groups {
		msg.Groups = append(msg.Groups, group.GroupSpec)
	}

	res, err := suite.dhandler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// ensure that it got created
	_, exists := suite.dkeeper.GetDeployment(suite.ctx, deployment.ID)
	require.True(t, exists)

	fundsAmount := sdkmath.LegacyZeroDec()
	fundsAmount.AddMut(sdkmath.LegacyNewDecFromInt(msg.Deposit.Amount.Amount))

	// ensure that the escrow account has the correct state
	accID := deployment.ID.ToEscrowAccountID()
	acc, err := suite.EscrowKeeper().GetAccount(suite.ctx, accID)
	require.NoError(t, err)
	require.Equal(t, deployment.ID.Owner, acc.State.Owner)
	require.Len(t, acc.State.Deposits, 1)
	require.Len(t, acc.State.Funds, 1)
	require.Equal(t, msg.Deposit.Amount.Denom, acc.State.Funds[0].Denom)
	require.Equal(t, suite.granter.String(), acc.State.Deposits[0].Owner)
	require.Equal(t, deposit.SourceGrant, acc.State.Deposits[0].Source)
	require.Equal(t, fundsAmount, acc.State.Funds[0].Amount)

	// deposit additional amount from the owner
	depositMsg := &ev1.MsgAccountDeposit{
		Signer: deployment.ID.Owner,
		ID:     deployment.ID.ToEscrowAccountID(),
		Deposit: deposit.Deposit{
			Amount:  suite.defaultDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}
	res, err = suite.ehandler(suite.ctx, depositMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	fundsAmount.AddMut(sdkmath.LegacyNewDecFromInt(depositMsg.Deposit.Amount.Amount))

	// ensure that the escrow account's state gets updated correctly
	acc, err = suite.EscrowKeeper().GetAccount(suite.ctx, accID)
	require.NoError(t, err)
	require.Equal(t, deployment.ID.Owner, acc.State.Owner)
	require.Len(t, acc.State.Deposits, 2)
	require.Len(t, acc.State.Funds, 1)
	require.Equal(t, suite.owner.String(), acc.State.Deposits[1].Owner)
	require.Equal(t, sdk.NewDecCoinFromCoin(depositMsg.Deposit.Amount).Amount, acc.State.Deposits[1].Balance.Amount)
	require.Equal(t, fundsAmount, acc.State.Funds[0].Amount)

	// deposit additional amount from the grant
	depositMsg1 := &ev1.MsgAccountDeposit{
		Signer: deployment.ID.Owner,
		ID:     deployment.ID.ToEscrowAccountID(),
		Deposit: deposit.Deposit{
			Amount:  suite.defaultDeposit,
			Sources: deposit.Sources{deposit.SourceGrant},
		},
	}
	res, err = suite.ehandler(suite.ctx, depositMsg1)
	require.NoError(t, err)
	require.NotNil(t, res)

	fundsAmount.AddMut(sdkmath.LegacyNewDecFromInt(depositMsg1.Deposit.Amount.Amount))

	// ensure that the escrow account's state gets updated correctly
	acc, err = suite.EscrowKeeper().GetAccount(suite.ctx, accID)
	require.NoError(t, err)
	require.Equal(t, deployment.ID.Owner, acc.State.Owner)
	require.Len(t, acc.State.Deposits, 3)
	require.Len(t, acc.State.Funds, 1)
	require.Equal(t, suite.granter.String(), acc.State.Deposits[2].Owner)
	require.Equal(t, sdk.NewDecCoinFromCoin(depositMsg1.Deposit.Amount).Amount, acc.State.Deposits[2].Balance.Amount)
	require.Equal(t, fundsAmount, acc.State.Funds[0].Amount)

	// depositing additional amount from a random depositor should pass
	depositMsg2 := &ev1.MsgAccountDeposit{
		Signer: testutil.AccAddress(t).String(),
		ID:     deployment.ID.ToEscrowAccountID(),
		Deposit: deposit.Deposit{
			Amount:  suite.defaultDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}
	res, err = suite.ehandler(suite.ctx, depositMsg2)
	require.NoError(t, err)
	require.NotNil(t, res)

	fundsAmount.AddMut(sdkmath.LegacyNewDecFromInt(depositMsg2.Deposit.Amount.Amount))

	// ensure that the escrow account's state gets updated correctly
	acc, err = suite.EscrowKeeper().GetAccount(suite.ctx, accID)
	require.NoError(t, err)
	require.Equal(t, deployment.ID.Owner, acc.State.Owner)
	require.Len(t, acc.State.Deposits, 4)
	require.Len(t, acc.State.Funds, 1)
	require.Equal(t, depositMsg2.Signer, acc.State.Deposits[3].Owner)
	require.Equal(t, sdk.NewDecCoinFromCoin(depositMsg2.Deposit.Amount).Amount, acc.State.Deposits[3].Balance.Amount)
	require.Equal(t, fundsAmount, acc.State.Funds[0].Amount)

	// make some payment from the escrow account
	providerAddr := testutil.AccAddress(t)

	lid := mtypes.LeaseID{
		Owner:    deployment.ID.Owner,
		DSeq:     deployment.ID.DSeq,
		GSeq:     0,
		OSeq:     0,
		Provider: providerAddr.String(),
	}

	pid := lid.ToEscrowPaymentID()

	rate := sdk.NewDecCoin(msg.Deposit.Amount.Denom, suite.defaultDeposit.Amount)
	err = suite.EscrowKeeper().PaymentCreate(suite.ctx, pid, providerAddr, rate)
	require.NoError(t, err)

	ctx := suite.ctx.WithBlockHeight(acc.State.SettledAt + 1)

	err = suite.EscrowKeeper().PaymentWithdraw(ctx, pid)
	require.NoError(t, err)

	fundsAmount.SubMut(sdkmath.LegacyNewDecFromInt(suite.defaultDeposit.Amount))

	// ensure that the escrow account's state gets updated correctly
	acc, err = suite.EscrowKeeper().GetAccount(ctx, accID)
	require.NoError(t, err)
	require.Equal(t, deployment.ID.Owner, acc.State.Owner)
	require.Len(t, acc.State.Deposits, 3)
	require.Len(t, acc.State.Funds, 1)
	require.Equal(t, fundsAmount, acc.State.Funds[0].Amount)
	require.Equal(t, sdkmath.LegacyNewDecFromInt(suite.defaultDeposit.Amount), acc.State.Transferred[0].Amount)

	// close the deployment
	closeMsg := &v1beta4.MsgCloseDeployment{ID: deployment.ID}
	res, err = suite.dhandler(ctx, closeMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// ensure that the escrow account has no balance left
	acc, err = suite.EscrowKeeper().GetAccount(ctx, accID)
	require.NoError(t, err)
	require.Equal(t, sdkmath.LegacyZeroDec(), acc.State.Funds[0].Amount)
	require.Len(t, acc.State.Deposits, 0)
}

func (st *testSuite) createDeployment() (v1.Deployment, v1beta4.Groups) {
	st.t.Helper()

	deployment := testutil.Deployment(st.t)
	group := testutil.DeploymentGroup(st.t, deployment.ID, 0)
	group.GroupSpec.Resources = v1beta4.ResourceUnits{
		{
			Resources: testutil.ResourceUnits(st.t),
			Count:     1,
			Price:     testutil.AkashDecCoinRandom(st.t),
		},
	}
	groups := v1beta4.Groups{
		group,
	}

	for i := range groups {
		groups[i].State = v1beta4.GroupOpen
	}

	return deployment, groups
}
