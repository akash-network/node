package handler_test

import (
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/akash-network/node/sdl"

	"github.com/akash-network/node/x/deployment/handler"

	sdktestdata "github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"

	"github.com/akash-network/node/testutil"
	"github.com/akash-network/node/testutil/state"
	"github.com/akash-network/node/x/deployment/handler/mocks"
	"github.com/akash-network/node/x/deployment/keeper"
	mkeeper "github.com/akash-network/node/x/market/keeper"
)

type testSuite struct {
	*state.TestSuite
	t           *testing.T
	ctx         sdk.Context
	mkeeper     mkeeper.IKeeper
	dkeeper     keeper.IKeeper
	authzKeeper handler.AuthzKeeper
	depositor   string
	handler     sdk.Handler

	defaultDeposit sdk.Coin
}

func setupTestSuite(t *testing.T) *testSuite {
	ssuite := state.SetupTestSuite(t)

	defaultDeposit, err := types.DefaultParams().MinDepositFor("uakt")
	require.NoError(t, err)

	depositor := testutil.AccAddress(t)
	authzKeeper := &mocks.AuthzKeeper{}
	authzKeeper.
		On("GetCleanAuthorization", mock.Anything, mock.Anything, depositor, sdk.MsgTypeURL(&types.MsgDepositDeployment{})).
		Return(&types.DepositDeploymentAuthorization{
			SpendLimit: defaultDeposit.Add(defaultDeposit),
		}, time.Time{}).
		Once().
		On("GetCleanAuthorization", mock.Anything, mock.Anything, depositor, sdk.MsgTypeURL(&types.MsgDepositDeployment{})).
		Return(&types.DepositDeploymentAuthorization{
			SpendLimit: defaultDeposit,
		}, time.Time{}).
		Once().
		On("GetCleanAuthorization", mock.Anything, mock.Anything,
			mock.MatchedBy(func(addr sdk.AccAddress) bool {
				return !depositor.Equals(addr)
			}), sdk.MsgTypeURL(&types.MsgDepositDeployment{})).
		Return(nil, time.Time{})
	authzKeeper.
		On("DeleteGrant", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
	authzKeeper.
		On("SaveGrant", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	suite := &testSuite{
		TestSuite:      ssuite,
		t:              t,
		ctx:            ssuite.Context(),
		mkeeper:        ssuite.MarketKeeper(),
		dkeeper:        ssuite.DeploymentKeeper(),
		authzKeeper:    authzKeeper,
		depositor:      depositor.String(),
		defaultDeposit: defaultDeposit,
	}

	suite.handler = handler.NewHandler(suite.dkeeper, suite.mkeeper, ssuite.EscrowKeeper(),
		suite.authzKeeper)

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

	msg := &types.MsgCreateDeployment{
		ID:        deployment.ID(),
		Groups:    make([]types.GroupSpec, 0, len(groups)),
		Deposit:   suite.defaultDeposit,
		Depositor: deployment.ID().Owner,
	}

	for _, group := range groups {
		msg.Groups = append(msg.Groups, group.GroupSpec)
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	t.Run("ensure event created", func(t *testing.T) {
		t.Skip("now has more events")
		iev := testutil.ParseDeploymentEvent(t, res.Events)
		require.IsType(t, types.EventDeploymentCreated{}, iev)

		dev := iev.(types.EventDeploymentCreated)

		require.Equal(t, msg.ID, dev.ID)
	})

	deploymentResult, exists := suite.dkeeper.GetDeployment(suite.ctx, deployment.ID())
	require.True(t, exists)
	require.Equal(t, deploymentResult.Version, msg.Version)

	groupsResult := suite.dkeeper.GetGroups(suite.ctx, deployment.ID())
	require.NotEmpty(t, groupsResult)
	require.Equal(t, len(groupsResult), len(groups))

	for i, g := range groupsResult {
		require.Equal(t, groups[i].GetName(), g.GetName())
	}

	res, err = suite.handler(suite.ctx, msg)
	require.EqualError(t, err, types.ErrDeploymentExists.Error())
	require.Nil(t, res)
}

func TestCreateDeploymentEmptyGroups(t *testing.T) {
	suite := setupTestSuite(t)

	deployment := testutil.Deployment(suite.t)

	msg := &types.MsgCreateDeployment{
		ID:      deployment.ID(),
		Deposit: suite.defaultDeposit,
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.True(t, errors.Is(err, types.ErrInvalidGroups))
}

func TestUpdateDeploymentNonExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment := testutil.Deployment(suite.t)

	msg := &types.MsgUpdateDeployment{
		ID: deployment.ID(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrDeploymentNotFound.Error())
}

func TestUpdateDeploymentExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()

	sdlObj, err := sdl.ReadFile("../../../sdl/_testdata/simple.yaml")
	require.NoError(t, err)

	dgroups, err := sdlObj.DeploymentGroups()
	require.NoError(t, err)

	msgGroups := make([]types.GroupSpec, 0)
	for _, g := range dgroups {
		msgGroups = append(msgGroups, *g)
	}
	require.NotEmpty(t, msgGroups)
	require.Equal(t, len(msgGroups), 1)

	msg := &types.MsgCreateDeployment{
		ID:        deployment.ID(),
		Groups:    msgGroups,
		Version:   testutil.DefaultDeploymentVersion[:],
		Deposit:   suite.defaultDeposit,
		Depositor: deployment.ID().Owner,
	}

	for _, group := range groups {
		msg.Groups = append(msg.Groups, group.GroupSpec)
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	t.Run("assert deployment version", func(t *testing.T) {
		d, ok := suite.dkeeper.GetDeployment(suite.ctx, deployment.DeploymentID)
		require.True(t, ok)
		assert.Equal(t, d.Version, testutil.DefaultDeploymentVersion[:])
	})

	// Change the version
	depSum := sha256.Sum256(testutil.DefaultDeploymentVersion[:])

	msgUpdate := &types.MsgUpdateDeployment{
		ID:      msg.ID,
		Version: depSum[:],
	}
	res, err = suite.handler(suite.ctx, msgUpdate)
	require.NoError(t, err)
	require.NotNil(t, res)

	t.Run("ensure event created", func(t *testing.T) {
		t.Skip("now has more events")
		iev := testutil.ParseDeploymentEvent(t, res.Events[1:])
		require.IsType(t, types.EventDeploymentUpdated{}, iev)

		dev := iev.(types.EventDeploymentUpdated)

		require.Equal(t, msg.ID, dev.ID)
	})
	t.Run("assert version updated", func(t *testing.T) {
		d, ok := suite.dkeeper.GetDeployment(suite.ctx, deployment.DeploymentID)
		require.True(t, ok)
		assert.Equal(t, d.Version, depSum[:])
	})

	// Run the same update, should fail since nothing is different
	res, err = suite.handler(suite.ctx, msgUpdate)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid: deployment version")

}

func TestCloseDeploymentNonExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment := testutil.Deployment(suite.t)

	msg := &types.MsgCloseDeployment{
		ID: deployment.ID(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrDeploymentNotFound.Error())
}

func TestCloseDeploymentExisting(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()

	msg := &types.MsgCreateDeployment{
		ID:        deployment.ID(),
		Groups:    make([]types.GroupSpec, 0, len(groups)),
		Deposit:   suite.defaultDeposit,
		Depositor: deployment.ID().Owner,
	}

	for _, group := range groups {
		msg.Groups = append(msg.Groups, group.GroupSpec)
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	t.Run("ensure event created", func(t *testing.T) {
		t.Skip("now has more events")
		iev := testutil.ParseDeploymentEvent(t, res.Events)
		require.IsType(t, types.EventDeploymentCreated{}, iev)

		dev := iev.(types.EventDeploymentCreated)

		require.Equal(t, msg.ID, dev.ID)
	})

	msgClose := &types.MsgCloseDeployment{
		ID: deployment.ID(),
	}

	res, err = suite.handler(suite.ctx, msgClose)
	require.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure event updated", func(t *testing.T) {
		t.Skip("now has more events")
		iev := testutil.ParseDeploymentEvent(t, res.Events[1:2])
		require.IsType(t, types.EventDeploymentUpdated{}, iev)

		dev := iev.(types.EventDeploymentUpdated)

		require.Equal(t, msg.ID, dev.ID)
	})

	t.Run("ensure event close", func(t *testing.T) {
		t.Skip("now has more events")
		iev := testutil.ParseDeploymentEvent(t, res.Events[2:])
		require.IsType(t, types.EventDeploymentClosed{}, iev)

		dev := iev.(types.EventDeploymentClosed)

		require.Equal(t, msg.ID, dev.ID)
	})

	res, err = suite.handler(suite.ctx, msgClose)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrDeploymentClosed.Error())
}

func TestFundedDeployment(t *testing.T) {
	suite := setupTestSuite(t)

	deployment, groups := suite.createDeployment()

	// create a funded deployment
	msg := &types.MsgCreateDeployment{
		ID:        deployment.ID(),
		Groups:    make([]types.GroupSpec, 0, len(groups)),
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
	_, exists := suite.dkeeper.GetDeployment(suite.ctx, deployment.ID())
	require.True(t, exists)

	// ensure that the escrow account has correct state
	accID := types.EscrowAccountForDeployment(deployment.ID())
	acc, err := suite.EscrowKeeper().GetAccount(suite.ctx, accID)
	require.NoError(t, err)
	require.Equal(t, deployment.ID().Owner, acc.Owner)
	require.Equal(t, suite.depositor, acc.Depositor)
	require.Equal(t, sdk.NewDecCoin(msg.Deposit.Denom, sdk.ZeroInt()), acc.Balance)
	require.Equal(t, sdk.NewDecCoinFromCoin(msg.Deposit), acc.Funds)

	// deposit additional amount from the owner
	depositMsg := &types.MsgDepositDeployment{
		ID:        deployment.ID(),
		Amount:    suite.defaultDeposit,
		Depositor: deployment.ID().Owner,
	}
	res, err = suite.handler(suite.ctx, depositMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// ensure that the escrow account's state gets updated correctly
	acc, err = suite.EscrowKeeper().GetAccount(suite.ctx, accID)
	require.NoError(t, err)
	require.Equal(t, deployment.ID().Owner, acc.Owner)
	require.Equal(t, suite.depositor, acc.Depositor)
	require.Equal(t, sdk.NewDecCoinFromCoin(depositMsg.Amount), acc.Balance)
	require.Equal(t, sdk.NewDecCoinFromCoin(msg.Deposit), acc.Funds)

	// deposit additional amount from the depositor
	depositMsg1 := &types.MsgDepositDeployment{
		ID:        deployment.ID(),
		Amount:    suite.defaultDeposit,
		Depositor: suite.depositor,
	}
	res, err = suite.handler(suite.ctx, depositMsg1)
	require.NoError(t, err)
	require.NotNil(t, res)

	// ensure that the escrow account's state gets updated correctly
	acc, err = suite.EscrowKeeper().GetAccount(suite.ctx, accID)
	require.NoError(t, err)
	require.Equal(t, deployment.ID().Owner, acc.Owner)
	require.Equal(t, suite.depositor, acc.Depositor)
	require.Equal(t, sdk.NewDecCoinFromCoin(depositMsg.Amount), acc.Balance)
	require.Equal(t, sdk.NewDecCoinFromCoin(msg.Deposit.Add(depositMsg1.Amount)), acc.Funds)

	// depositing additional amount from a random depositor should fail
	depositMsg2 := &types.MsgDepositDeployment{
		ID:        deployment.ID(),
		Amount:    suite.defaultDeposit,
		Depositor: testutil.AccAddress(t).String(),
	}
	res, err = suite.handler(suite.ctx, depositMsg2)
	require.Error(t, err)
	require.Nil(t, res)

	// make some payment from the escrow account
	pid := "test_pid"
	providerAddr := testutil.AccAddress(t)
	rate := sdk.NewDecCoin(msg.Deposit.Denom, sdk.NewInt(12500000))
	require.NoError(t, suite.EscrowKeeper().PaymentCreate(suite.ctx, accID, pid, providerAddr, rate))
	ctx := suite.ctx.WithBlockHeight(acc.SettledAt + 1)
	require.NoError(t, suite.EscrowKeeper().PaymentWithdraw(ctx, accID, pid))

	// ensure that the escrow account's state gets updated correctly
	acc, err = suite.EscrowKeeper().GetAccount(ctx, accID)
	require.NoError(t, err)
	require.Equal(t, sdk.NewDecCoin(msg.Deposit.Denom, sdk.NewInt(2500000)), acc.Balance)
	require.Equal(t, sdk.NewDecCoin(msg.Deposit.Denom, sdk.ZeroInt()), acc.Funds)

	// close the deployment
	closeMsg := &types.MsgCloseDeployment{ID: deployment.ID()}
	res, err = suite.handler(ctx, closeMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// ensure that the escrow account has no balance left
	acc, err = suite.EscrowKeeper().GetAccount(ctx, accID)
	require.NoError(t, err)
	require.Equal(t, sdk.NewDecCoin(msg.Deposit.Denom, sdk.ZeroInt()), acc.Balance)
	require.Equal(t, sdk.NewDecCoin(msg.Deposit.Denom, sdk.ZeroInt()), acc.Funds)
}

func (st *testSuite) createDeployment() (types.Deployment, []types.Group) {
	st.t.Helper()

	deployment := testutil.Deployment(st.t)
	group := testutil.DeploymentGroup(st.t, deployment.ID(), 0)
	group.GroupSpec.Resources = []types.Resource{
		{
			Resources: testutil.ResourceUnits(st.t),
			Count:     1,
			Price:     testutil.AkashDecCoinRandom(st.t),
		},
	}
	groups := []types.Group{
		group,
	}

	for i := range groups {
		groups[i].State = types.GroupOpen
	}

	return deployment, groups
}
