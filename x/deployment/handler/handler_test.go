package handler_test

import (
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/baseapp"
	sdktestdata "github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/node/deployment/v1beta4"

	"pkg.akt.dev/node/testutil"
	cmocks "pkg.akt.dev/node/testutil/cosmos/mocks"
	"pkg.akt.dev/node/testutil/state"
	"pkg.akt.dev/node/x/deployment/handler"
	"pkg.akt.dev/node/x/deployment/keeper"
	mkeeper "pkg.akt.dev/node/x/market/keeper"
)

type testSuite struct {
	*state.TestSuite
	t              *testing.T
	ctx            sdk.Context
	mkeeper        mkeeper.IKeeper
	dkeeper        keeper.IKeeper
	authzKeeper    handler.AuthzKeeper
	depositor      string
	handler        baseapp.MsgServiceHandler
	defaultDeposit sdk.Coin
}

func setupTestSuite(t *testing.T) *testSuite {
	defaultDeposit, err := v1beta4.DefaultParams().MinDepositFor("uakt")
	require.NoError(t, err)

	depositor := testutil.AccAddress(t)
	authzKeeper := &cmocks.AuthzKeeper{}
	authzKeeper.
		On("GetAuthorization", mock.Anything, mock.Anything, depositor, sdk.MsgTypeURL(&v1.MsgDepositDeployment{})).
		Return(&v1.DepositAuthorization{
			SpendLimit: defaultDeposit.Add(defaultDeposit),
		}, &time.Time{}).
		Once().
		On("GetAuthorization", mock.Anything, mock.Anything, depositor, sdk.MsgTypeURL(&v1.MsgDepositDeployment{})).
		Return(&v1.DepositAuthorization{
			SpendLimit: defaultDeposit,
		}, &time.Time{}).
		Once().
		On("GetAuthorization", mock.Anything, mock.Anything, depositor, sdk.MsgTypeURL(&v1.MsgDepositDeployment{})).
		Return(&v1.DepositAuthorization{
			SpendLimit: defaultDeposit,
		}, &time.Time{}).
		Once().
		On("GetAuthorization", mock.Anything, mock.Anything, depositor, sdk.MsgTypeURL(&v1.MsgDepositDeployment{})).
		Return(&v1.DepositAuthorization{
			SpendLimit: defaultDeposit,
		}, &time.Time{}).
		Once().
		On("GetAuthorization", mock.Anything, mock.Anything,
			mock.MatchedBy(func(addr sdk.AccAddress) bool {
				return !depositor.Equals(addr)
			}), sdk.MsgTypeURL(&v1.MsgDepositDeployment{})).
		Return(nil, &time.Time{})
	authzKeeper.
		On("DeleteGrant", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
	authzKeeper.
		On("SaveGrant", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	keepers := state.Keepers{
		Authz: authzKeeper,
	}
	ssuite := state.SetupTestSuiteWithKeepers(t, keepers)

	suite := &testSuite{
		TestSuite:      ssuite,
		t:              t,
		ctx:            ssuite.Context(),
		mkeeper:        ssuite.MarketKeeper(),
		dkeeper:        ssuite.DeploymentKeeper(),
		authzKeeper:    ssuite.AuthzKeeper(),
		depositor:      depositor.String(),
		defaultDeposit: defaultDeposit,
	}

	suite.handler = handler.NewHandler(suite.dkeeper, suite.mkeeper, ssuite.EscrowKeeper(), suite.authzKeeper)

	return suite
}

func TestProviderBadMessageType(t *testing.T) {
	suite := setupTestSuite(t)

	res, err := suite.handler(suite.ctx, sdk.Msg(sdktestdata.NewTestMsg()))
	require.Nil(t, res)
	require.Error(t, err)
	require.True(t, errors.Is(err, sdkerrors.ErrUnknownRequest))
}

func TestCreateDeployment(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()

	msg := &v1beta4.MsgCreateDeployment{
		ID:        deployment.ID,
		Groups:    make(v1beta4.GroupSpecs, 0, len(groups)),
		Deposit:   suite.defaultDeposit,
		Depositor: deployment.ID.Owner,
	}

	for _, group := range groups {
		msg.Groups = append(msg.Groups, group.GroupSpec)
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// t.Run("ensure event created", func(t *testing.T) {
	// 	t.Skip("now has more events")
	// 	iev := testutil.ParseDeploymentEvent(t, res.Events)
	// 	require.IsType(t, v1beta4.EventDeploymentCreated{}, iev)
	//
	// 	dev := iev.(v1beta4.EventDeploymentCreated)
	//
	// 	require.Equal(t, msg.ID, dev.ID)
	// })

	deploymentResult, exists := suite.dkeeper.GetDeployment(suite.ctx, deployment.ID)
	require.True(t, exists)
	require.Equal(t, deploymentResult.Hash, msg.Hash)

	groupsResult := suite.dkeeper.GetGroups(suite.ctx, deployment.ID)
	require.NotEmpty(t, groupsResult)
	require.Equal(t, len(groupsResult), len(groups))

	for i, g := range groupsResult {
		require.Equal(t, groups[i].GetName(), g.GetName())
	}

	res, err = suite.handler(suite.ctx, msg)
	require.EqualError(t, err, v1.ErrDeploymentExists.Error())
	require.Nil(t, res)
}

func TestCreateDeploymentEmptyGroups(t *testing.T) {
	suite := setupTestSuite(t)

	deployment := testutil.Deployment(suite.t)

	msg := &v1beta4.MsgCreateDeployment{
		ID:      deployment.ID,
		Deposit: suite.defaultDeposit,
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.True(t, errors.Is(err, v1.ErrInvalidGroups))
}

func TestUpdateDeploymentNonExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment := testutil.Deployment(suite.t)

	msg := &v1beta4.MsgUpdateDeployment{
		ID: deployment.ID,
	}

	res, err := suite.handler(suite.ctx, msg)
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
		ID:        deployment.ID,
		Groups:    msgGroupSpecs,
		Hash:      testutil.DefaultDeploymentHash[:],
		Deposit:   suite.defaultDeposit,
		Depositor: deployment.ID.Owner,
	}

	res, err := suite.handler(suite.ctx, msg)
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
	res, err = suite.handler(suite.ctx, msgUpdate)
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
	res, err = suite.handler(suite.ctx, msgUpdate)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid: deployment hash")

}

func TestCloseDeploymentNonExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment := testutil.Deployment(suite.t)

	msg := &v1beta4.MsgCloseDeployment{
		ID: deployment.ID,
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, v1.ErrDeploymentNotFound.Error())
}

func TestCloseDeploymentExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()

	msg := &v1beta4.MsgCreateDeployment{
		ID:        deployment.ID,
		Groups:    make(v1beta4.GroupSpecs, 0, len(groups)),
		Deposit:   suite.defaultDeposit,
		Depositor: deployment.ID.Owner,
	}

	for _, group := range groups {
		msg.Groups = append(msg.Groups, group.GroupSpec)
	}

	res, err := suite.handler(suite.ctx, msg)
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

	res, err = suite.handler(suite.ctx, msgClose)
	require.NotNil(t, res)
	require.NoError(t, err)

	// t.Run("ensure event updated", func(t *testing.T) {
	// 	iev, err := sdk.ParseTypedEvent(res.Events[2])
	// 	require.NoError(t, err)
	// 	require.IsType(t, &v1.EventDeploymentUpdated{}, iev)
	//
	// 	dev := iev.(*v1.EventDeploymentUpdated)
	//
	// 	require.Equal(t, msg.ID, dev.ID)
	// })

	t.Run("ensure event close", func(t *testing.T) {
		iev, err := sdk.ParseTypedEvent(res.Events[2])
		require.NoError(t, err)

		require.IsType(t, &v1.EventDeploymentClosed{}, iev)

		dev := iev.(*v1.EventDeploymentClosed)

		require.Equal(t, msg.ID, dev.ID)
	})

	res, err = suite.handler(suite.ctx, msgClose)
	require.Nil(t, res)
	require.EqualError(t, err, v1.ErrDeploymentClosed.Error())
}

func TestFundedDeployment(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()

	// create a funded deployment
	msg := &v1beta4.MsgCreateDeployment{
		ID:        deployment.ID,
		Groups:    make(v1beta4.GroupSpecs, 0, len(groups)),
		Deposit:   suite.defaultDeposit,
		Depositor: suite.depositor,
	}

	for _, group := range groups {
		msg.Groups = append(msg.Groups, group.GroupSpec)
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// ensure that it got created
	_, exists := suite.dkeeper.GetDeployment(suite.ctx, deployment.ID)
	require.True(t, exists)

	// ensure that the escrow account has correct state
	accID := v1beta4.EscrowAccountForDeployment(deployment.ID)
	acc, err := suite.EscrowKeeper().GetAccount(suite.ctx, accID)
	require.NoError(t, err)
	require.Equal(t, deployment.ID.Owner, acc.Owner)
	require.Equal(t, suite.depositor, acc.Depositor)
	require.Equal(t, sdk.NewDecCoin(msg.Deposit.Denom, sdkmath.ZeroInt()), acc.Balance)
	require.Equal(t, sdk.NewDecCoinFromCoin(msg.Deposit), acc.Funds)

	// deposit additional amount from the owner
	depositMsg := &v1.MsgDepositDeployment{
		ID:        deployment.ID,
		Amount:    suite.defaultDeposit,
		Depositor: deployment.ID.Owner,
	}
	res, err = suite.handler(suite.ctx, depositMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// ensure that the escrow account's state gets updated correctly
	acc, err = suite.EscrowKeeper().GetAccount(suite.ctx, accID)
	require.NoError(t, err)
	require.Equal(t, deployment.ID.Owner, acc.Owner)
	require.Equal(t, suite.depositor, acc.Depositor)
	require.Equal(t, sdk.NewDecCoinFromCoin(depositMsg.Amount), acc.Balance)
	require.Equal(t, sdk.NewDecCoinFromCoin(msg.Deposit), acc.Funds)

	// deposit additional amount from the depositor
	depositMsg1 := &v1.MsgDepositDeployment{
		ID:        deployment.ID,
		Amount:    suite.defaultDeposit,
		Depositor: suite.depositor,
	}
	res, err = suite.handler(suite.ctx, depositMsg1)
	require.NoError(t, err)
	require.NotNil(t, res)

	// ensure that the escrow account's state gets updated correctly
	acc, err = suite.EscrowKeeper().GetAccount(suite.ctx, accID)
	require.NoError(t, err)
	require.Equal(t, deployment.ID.Owner, acc.Owner)
	require.Equal(t, suite.depositor, acc.Depositor)
	require.Equal(t, sdk.NewDecCoinFromCoin(depositMsg.Amount), acc.Balance)
	require.Equal(t, sdk.NewDecCoinFromCoin(msg.Deposit.Add(depositMsg1.Amount)), acc.Funds)

	// depositing additional amount from a random depositor should fail
	depositMsg2 := &v1.MsgDepositDeployment{
		ID:        deployment.ID,
		Amount:    suite.defaultDeposit,
		Depositor: testutil.AccAddress(t).String(),
	}
	res, err = suite.handler(suite.ctx, depositMsg2)
	require.Error(t, err)
	require.Nil(t, res)

	// make some payment from the escrow account
	pid := "test_pid"
	providerAddr := testutil.AccAddress(t)
	rate := sdk.NewDecCoin(msg.Deposit.Denom, suite.defaultDeposit.Amount)
	require.NoError(t, suite.EscrowKeeper().PaymentCreate(suite.ctx, accID, pid, providerAddr, rate))
	ctx := suite.ctx.WithBlockHeight(acc.SettledAt + 1)
	require.NoError(t, suite.EscrowKeeper().PaymentWithdraw(ctx, accID, pid))

	// ensure that the escrow account's state gets updated correctly
	acc, err = suite.EscrowKeeper().GetAccount(ctx, accID)
	require.NoError(t, err)
	require.Equal(t, sdk.NewDecCoin(msg.Deposit.Denom, suite.defaultDeposit.Amount), acc.Balance)
	require.Equal(t, sdk.NewDecCoin(msg.Deposit.Denom, suite.defaultDeposit.Amount), acc.Funds)

	// close the deployment
	closeMsg := &v1beta4.MsgCloseDeployment{ID: deployment.ID}
	res, err = suite.handler(ctx, closeMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// ensure that the escrow account has no balance left
	acc, err = suite.EscrowKeeper().GetAccount(ctx, accID)
	require.NoError(t, err)
	require.Equal(t, sdk.NewDecCoin(msg.Deposit.Denom, sdkmath.ZeroInt()), acc.Balance)
	require.Equal(t, sdk.NewDecCoin(msg.Deposit.Denom, sdkmath.ZeroInt()), acc.Funds)
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
