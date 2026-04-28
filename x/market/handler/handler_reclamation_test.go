package handler_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
	deposit "pkg.akt.dev/go/node/types/deposit/v1"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/v2/testutil/state"
	bmemodule "pkg.akt.dev/node/v2/x/bme"
)

// ===========================
// Helpers
// ===========================

// prepareBlanketMocks sets up blanket bank mocks for tests that need escrow operations.
func prepareBlanketMocks(suite *testSuite) {
	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()
		bkeeper.On("SendCoinsFromAccountToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
		bkeeper.On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
		bkeeper.On("SendCoinsFromModuleToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
		bkeeper.On("MintCoins", mock.Anything, bmemodule.ModuleName, mock.Anything).Return(nil).Maybe()
		bkeeper.On("BurnCoins", mock.Anything, bmemodule.ModuleName, mock.Anything).Return(nil).Maybe()
	})
}

// setupEscrowAccount creates only the escrow account for a deployment,
// without creating the payment. Use this when the handler will create the payment.
func (st *testSuite) setupEscrowAccount(bid mvbeta.Bid, order mvbeta.Order) {
	st.t.Helper()
	ctx := st.Context()

	owner, err := sdk.AccAddressFromBech32(bid.ID.Owner)
	require.NoError(st.t, err)

	denom := bid.Price.Denom
	defaultDeposit, err := dtypes.DefaultParams().MinDepositFor(denom)
	if err != nil {
		defaultDeposit, err = dtypes.DefaultParams().MinDepositFor("uact")
		require.NoError(st.t, err)
	}

	msg := &dtypes.MsgCreateDeployment{
		ID: order.ID.GroupID().DeploymentID(),
		Deposit: deposit.Deposit{
			Amount:  defaultDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	deposits, err := st.EscrowKeeper().AuthorizeDeposits(ctx, msg)
	require.NoError(st.t, err)

	err = st.EscrowKeeper().AccountCreate(ctx, bid.ID.DeploymentID().ToEscrowAccountID(), owner, deposits)
	require.NoError(st.t, err)
}

// setupLeaseEscrow creates the escrow account and payment for a lease.
func (st *testSuite) setupLeaseEscrow(bid mvbeta.Bid, order mvbeta.Order) {
	st.t.Helper()
	ctx := st.Context()

	owner, err := sdk.AccAddressFromBech32(bid.ID.Owner)
	require.NoError(st.t, err)
	provider, err := sdk.AccAddressFromBech32(bid.ID.Provider)
	require.NoError(st.t, err)

	// Use the bid price denom to determine the deposit denom
	denom := bid.Price.Denom
	defaultDeposit, err := dtypes.DefaultParams().MinDepositFor(denom)
	if err != nil {
		// Fallback: try uact
		defaultDeposit, err = dtypes.DefaultParams().MinDepositFor("uact")
		require.NoError(st.t, err)
	}

	msg := &dtypes.MsgCreateDeployment{
		ID: order.ID.GroupID().DeploymentID(),
		Deposit: deposit.Deposit{
			Amount:  defaultDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
	}

	deposits, err := st.EscrowKeeper().AuthorizeDeposits(ctx, msg)
	require.NoError(st.t, err)

	err = st.EscrowKeeper().AccountCreate(ctx, bid.ID.DeploymentID().ToEscrowAccountID(), owner, deposits)
	require.NoError(st.t, err)

	err = st.EscrowKeeper().PaymentCreate(ctx, bid.ID.LeaseID().ToEscrowPaymentID(), provider, bid.Price)
	require.NoError(st.t, err)
}

func (st *testSuite) createOrderWithReclamation(resources dtypes.ResourceUnits, reclamation *dv1.DeploymentReclamation) (mvbeta.Order, dtypes.GroupSpec) {
	st.t.Helper()

	deployment := testutil.Deployment(st.t)
	deployment.Reclamation = reclamation
	group := testutil.DeploymentGroup(st.t, deployment.ID, 0)
	group.GroupSpec.Resources = resources

	err := st.DeploymentKeeper().Create(st.Context(), deployment, []dtypes.Group{group})
	require.NoError(st.t, err)

	order, err := st.MarketKeeper().CreateOrder(st.Context(), group.ID, group.GroupSpec, reclamation)
	require.NoError(st.t, err)

	return order, group.GroupSpec
}

func (st *testSuite) createBidWithReclamation(reclaimWindow *time.Duration) (mvbeta.Bid, mvbeta.Order) {
	st.t.Helper()
	order, gspec := st.createOrder(testutil.Resources(st.t, testutil.WithDenom("uact")))
	provider := testutil.AccAddress(st.t)

	price := order.Price() // use order price to ensure bid doesn't exceed it
	roffer := mvbeta.ResourceOfferFromRU(gspec.Resources)

	bidID := mv1.MakeBidID(order.ID, provider)

	bid, err := st.MarketKeeper().CreateBid(st.Context(), bidID, price, roffer, reclaimWindow)
	require.NoError(st.t, err)

	return bid, order
}

func (st *testSuite) createLeaseWithReclamation(reclaimWindow *time.Duration) (mv1.LeaseID, mvbeta.Bid, mvbeta.Order) {
	st.t.Helper()
	bid, order := st.createBidWithReclamation(reclaimWindow)

	err := st.MarketKeeper().CreateLease(st.Context(), bid)
	require.NoError(st.t, err)

	// Store reclamation on the lease if bid has it
	if reclaimWindow != nil {
		lease, found := st.MarketKeeper().GetLease(st.Context(), bid.ID.LeaseID())
		require.True(st.t, found)
		lease.Reclamation = &mv1.Reclamation{
			Window: *reclaimWindow,
		}
		err = st.MarketKeeper().SaveLease(st.Context(), lease)
		require.NoError(st.t, err)
	}

	st.MarketKeeper().OnBidMatched(st.Context(), bid)
	st.MarketKeeper().OnOrderMatched(st.Context(), order)

	lid := mv1.MakeLeaseID(bid.ID)
	return lid, bid, order
}

// ===========================
// LeaseStartReclaim Tests
// ===========================

func TestLeaseStartReclaim_Success(t *testing.T) {
	suite := setupTestSuite(t)

	window := 24 * time.Hour
	lid, _, _ := suite.createLeaseWithReclamation(&window)

	blockTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	suite.SetBlockHeight(100)
	ctx := suite.Context().WithBlockTime(blockTime)

	msg := &mvbeta.MsgLeaseStartReclaim{
		ID:     lid,
		Reason: mv1.LeaseClosedReason(10001),
	}

	res, err := suite.handler(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify lease state
	lease, found := suite.MarketKeeper().GetLease(ctx, lid)
	require.True(t, found)
	assert.Equal(t, mv1.LeaseReclaiming, lease.State)
	assert.Equal(t, int64(100), lease.Reclamation.StartedAt)
	assert.Equal(t, blockTime.Add(24*time.Hour).Unix(), lease.Reclamation.Deadline)
	assert.Equal(t, mv1.LeaseClosedReason(10001), lease.Reclamation.Reason)
}

func TestLeaseStartReclaim_NoReclamation(t *testing.T) {
	suite := setupTestSuite(t)

	// Create lease without reclamation
	lid, _, _ := suite.createLease()

	msg := &mvbeta.MsgLeaseStartReclaim{
		ID:     lid,
		Reason: mv1.LeaseClosedReason(10001),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
	require.ErrorIs(t, err, mv1.ErrLeaseNotReclamable)
}

func TestLeaseStartReclaim_AlreadyReclaiming(t *testing.T) {
	suite := setupTestSuite(t)

	window := 24 * time.Hour
	lid, _, _ := suite.createLeaseWithReclamation(&window)

	blockTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	suite.SetBlockHeight(100)
	ctx := suite.Context().WithBlockTime(blockTime)

	msg := &mvbeta.MsgLeaseStartReclaim{
		ID:     lid,
		Reason: mv1.LeaseClosedReason(10001),
	}

	// First call succeeds
	res, err := suite.handler(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Second call fails -- lease is now in LeaseReclaiming state, not LeaseActive
	res, err = suite.handler(ctx, msg)
	require.Nil(t, res)
	require.Error(t, err)
	require.ErrorIs(t, err, mv1.ErrLeaseNotActive)
}

func TestLeaseStartReclaim_LeaseNotActive(t *testing.T) {
	suite := setupTestSuite(t)

	window := 24 * time.Hour
	lid, _, _ := suite.createLeaseWithReclamation(&window)

	// Close the lease first
	lease, found := suite.MarketKeeper().GetLease(suite.Context(), lid)
	require.True(t, found)
	_ = suite.MarketKeeper().OnLeaseClosed(suite.Context(), lease, mv1.LeaseClosed, mv1.LeaseClosedReasonOwner)

	msg := &mvbeta.MsgLeaseStartReclaim{
		ID:     lid,
		Reason: mv1.LeaseClosedReason(10001),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
	require.ErrorIs(t, err, mv1.ErrLeaseNotActive)
}

func TestLeaseStartReclaim_UnknownLease(t *testing.T) {
	suite := setupTestSuite(t)

	msg := &mvbeta.MsgLeaseStartReclaim{
		ID:     testutil.LeaseID(t),
		Reason: mv1.LeaseClosedReason(10001),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
	require.ErrorIs(t, err, mv1.ErrUnknownLease)
}

// ===========================
// CloseBid Reclamation Enforcement Tests
// ===========================

func TestCloseBid_ReclamationNotStarted(t *testing.T) {
	suite := setupTestSuite(t)

	window := 24 * time.Hour
	lid, bid, _ := suite.createLeaseWithReclamation(&window)

	_ = lid // used implicitly through bid

	msg := &mvbeta.MsgCloseBid{
		ID:     bid.ID,
		Reason: mv1.LeaseClosedReason(10001),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
	require.ErrorIs(t, err, mv1.ErrReclamationNotStarted)
}

func TestCloseBid_ReclamationWindowNotElapsed(t *testing.T) {
	suite := setupTestSuite(t)

	window := 24 * time.Hour
	lid, bid, _ := suite.createLeaseWithReclamation(&window)

	blockTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	suite.SetBlockHeight(100)
	ctx := suite.Context().WithBlockTime(blockTime)

	// Start reclamation
	reclaimMsg := &mvbeta.MsgLeaseStartReclaim{
		ID:     lid,
		Reason: mv1.LeaseClosedReason(10001),
	}
	res, err := suite.handler(ctx, reclaimMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Try to close DURING the window (advance 12h, but window is 24h)
	midWindowTime := blockTime.Add(12 * time.Hour)
	ctx = suite.Context().WithBlockTime(midWindowTime)

	closeMsg := &mvbeta.MsgCloseBid{
		ID:     bid.ID,
		Reason: mv1.LeaseClosedReason(10001),
	}

	res, err = suite.handler(ctx, closeMsg)
	require.Nil(t, res)
	require.Error(t, err)
	require.ErrorIs(t, err, mv1.ErrReclamationWindowNotElapsed)
}

func TestCloseBid_AfterReclamationWindow(t *testing.T) {
	suite := setupTestSuite(t)
	prepareBlanketMocks(suite)

	window := 1 * time.Hour
	lid, bid, order := suite.createLeaseWithReclamation(&window)
	suite.setupLeaseEscrow(bid, order)

	blockTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	suite.SetBlockHeight(100)
	ctx := suite.Context().WithBlockTime(blockTime)

	// Start reclamation
	reclaimMsg := &mvbeta.MsgLeaseStartReclaim{
		ID:     lid,
		Reason: mv1.LeaseClosedReason(10001),
	}
	res, err := suite.handler(ctx, reclaimMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Advance past the window (window is 1h, advance 2h)
	afterWindowTime := blockTime.Add(2 * time.Hour)
	ctx = suite.Context().WithBlockTime(afterWindowTime)

	closeMsg := &mvbeta.MsgCloseBid{
		ID:     bid.ID,
		Reason: mv1.LeaseClosedReason(10001),
	}

	res, err = suite.handler(ctx, closeMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify lease is closed
	lease, found := suite.MarketKeeper().GetLease(ctx, lid)
	require.True(t, found)
	assert.Equal(t, mv1.LeaseClosed, lease.State)
}

func TestCloseBid_NoReclamation_StillWorks(t *testing.T) {
	suite := setupTestSuite(t)
	prepareBlanketMocks(suite)

	// Create lease WITHOUT reclamation
	lid, bid, order := suite.createLease()
	suite.setupLeaseEscrow(bid, order)

	closeMsg := &mvbeta.MsgCloseBid{
		ID:     bid.ID,
		Reason: mv1.LeaseClosedReason(10001),
	}

	res, err := suite.handler(suite.Context(), closeMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	lease, found := suite.MarketKeeper().GetLease(suite.Context(), lid)
	require.True(t, found)
	assert.Equal(t, mv1.LeaseClosed, lease.State)
}

// ===========================
// CloseLease During Reclamation Tests
// ===========================

func TestCloseLease_DuringReclamation_TenantCanAlwaysClose(t *testing.T) {
	suite := setupTestSuite(t)
	prepareBlanketMocks(suite)

	window := 24 * time.Hour
	lid, bid, order := suite.createLeaseWithReclamation(&window)
	suite.setupLeaseEscrow(bid, order)

	blockTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	suite.SetBlockHeight(100)
	ctx := suite.Context().WithBlockTime(blockTime)

	// Start reclamation
	reclaimMsg := &mvbeta.MsgLeaseStartReclaim{
		ID:     lid,
		Reason: mv1.LeaseClosedReason(10001),
	}
	res, err := suite.handler(ctx, reclaimMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Tenant closes DURING the window (only 1 minute into the 24h window)
	earlyTime := blockTime.Add(1 * time.Minute)
	ctx = suite.Context().WithBlockTime(earlyTime)

	closeMsg := &mvbeta.MsgCloseLease{
		ID: lid,
	}

	res, err = suite.handler(ctx, closeMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify lease is closed
	lease, found := suite.MarketKeeper().GetLease(ctx, lid)
	require.True(t, found)
	assert.Equal(t, mv1.LeaseClosed, lease.State)
}

func TestCloseLease_ActiveWithReclamation_TenantCanClose(t *testing.T) {
	suite := setupTestSuite(t)
	prepareBlanketMocks(suite)

	window := 24 * time.Hour
	lid, bid, order := suite.createLeaseWithReclamation(&window)
	suite.setupLeaseEscrow(bid, order)

	// Tenant closes without reclamation being started (lease is Active, has reclamation config)
	closeMsg := &mvbeta.MsgCloseLease{
		ID: lid,
	}

	res, err := suite.handler(suite.Context(), closeMsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	lease, found := suite.MarketKeeper().GetLease(suite.Context(), lid)
	require.True(t, found)
	assert.Equal(t, mv1.LeaseClosed, lease.State)
}

// ===========================
// CreateLease Reclamation Storage Tests
// ===========================

func TestCreateLease_StoresReclamation(t *testing.T) {
	suite := setupTestSuite(t)
	prepareBlanketMocks(suite)

	window := 48 * time.Hour
	bid, order := suite.createBidWithReclamation(&window)
	suite.setupEscrowAccount(bid, order)

	msg := &mvbeta.MsgCreateLease{
		BidID: bid.ID,
	}

	res, err := suite.handler(suite.Context(), msg)
	require.NoError(t, err)
	require.NotNil(t, res)

	lid := mv1.MakeLeaseID(bid.ID)
	lease, found := suite.MarketKeeper().GetLease(suite.Context(), lid)
	require.True(t, found)
	require.NotNil(t, lease.Reclamation)
	assert.Equal(t, 48*time.Hour, lease.Reclamation.Window)
	assert.Equal(t, int64(0), lease.Reclamation.StartedAt)
	assert.Equal(t, int64(0), lease.Reclamation.Deadline)
}

func TestCreateLease_NoReclamation(t *testing.T) {
	suite := setupTestSuite(t)

	lid, _, _ := suite.createLease()

	lease, found := suite.MarketKeeper().GetLease(suite.Context(), lid)
	require.True(t, found)
	assert.Nil(t, lease.Reclamation)
}

// ===========================
// CreateBid Reclamation Validation Tests
// ===========================

func TestCreateBid_ReclamationRequired_NoBidWindow(t *testing.T) {
	suite := setupTestSuite(t)

	reclamation := &dv1.DeploymentReclamation{
		MinWindow: 24 * time.Hour,
	}
	order, gspec := suite.createOrderWithReclamation(
		testutil.Resources(t, testutil.WithDenom("uact")),
		reclamation,
	)

	provider := suite.createProvider(gspec.Requirements.Attributes)
	providerAddr, _ := sdk.AccAddressFromBech32(provider.Owner)
	roffer := mvbeta.ResourceOfferFromRU(gspec.Resources)

	suite.PrepareMocks(func(ts *state.TestSuite) {
		ts.MockBMEForDeposit(providerAddr, mvbeta.DefaultBidMinDepositACT)
	})

	bmsg := &mvbeta.MsgCreateBid{
		ID:    mv1.MakeBidID(order.ID, providerAddr),
		Price: order.Price(),
		Deposit: deposit.Deposit{
			Amount:  mvbeta.DefaultBidMinDepositACT,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
		ResourcesOffer:    roffer,
		ReclamationWindow: nil, // no reclamation offered
	}

	res, err := suite.handler(suite.Context(), bmsg)
	require.Nil(t, res)
	require.Error(t, err)
	require.ErrorIs(t, err, mv1.ErrReclamationRequired)
}

func TestCreateBid_ReclamationWindowTooShort(t *testing.T) {
	suite := setupTestSuite(t)

	reclamation := &dv1.DeploymentReclamation{
		MinWindow: 24 * time.Hour,
	}
	order, gspec := suite.createOrderWithReclamation(
		testutil.Resources(t, testutil.WithDenom("uact")),
		reclamation,
	)

	provider := suite.createProvider(gspec.Requirements.Attributes)
	providerAddr, _ := sdk.AccAddressFromBech32(provider.Owner)
	roffer := mvbeta.ResourceOfferFromRU(gspec.Resources)

	suite.PrepareMocks(func(ts *state.TestSuite) {
		ts.MockBMEForDeposit(providerAddr, mvbeta.DefaultBidMinDepositACT)
	})

	shortWindow := 1 * time.Hour // less than 24h min
	bmsg := &mvbeta.MsgCreateBid{
		ID:    mv1.MakeBidID(order.ID, providerAddr),
		Price: order.Price(),
		Deposit: deposit.Deposit{
			Amount:  mvbeta.DefaultBidMinDepositACT,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
		ResourcesOffer:    roffer,
		ReclamationWindow: &shortWindow,
	}

	res, err := suite.handler(suite.Context(), bmsg)
	require.Nil(t, res)
	require.Error(t, err)
	require.ErrorIs(t, err, mv1.ErrReclamationWindowTooShort)
}

func TestCreateBid_ReclamationWindowBelowGovernanceMin(t *testing.T) {
	suite := setupTestSuite(t)

	// Order does NOT require reclamation, but provider offers a window below governance min
	order, gspec := suite.createOrder(testutil.Resources(t, testutil.WithDenom("uact")))

	provider := suite.createProvider(gspec.Requirements.Attributes)
	providerAddr, _ := sdk.AccAddressFromBech32(provider.Owner)
	roffer := mvbeta.ResourceOfferFromRU(gspec.Resources)

	suite.PrepareMocks(func(ts *state.TestSuite) {
		ts.MockBMEForDeposit(providerAddr, mvbeta.DefaultBidMinDepositACT)
	})

	tinyWindow := 1 * time.Second // below 1h governance min
	bmsg := &mvbeta.MsgCreateBid{
		ID:    mv1.MakeBidID(order.ID, providerAddr),
		Price: order.Price(),
		Deposit: deposit.Deposit{
			Amount:  mvbeta.DefaultBidMinDepositACT,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
		ResourcesOffer:    roffer,
		ReclamationWindow: &tinyWindow,
	}

	res, err := suite.handler(suite.Context(), bmsg)
	require.Nil(t, res)
	require.Error(t, err)
	require.ErrorIs(t, err, mv1.ErrReclamationWindowInvalid)
}

func TestCreateBid_ReclamationWindowAboveGovernanceMax(t *testing.T) {
	suite := setupTestSuite(t)

	order, gspec := suite.createOrder(testutil.Resources(t, testutil.WithDenom("uact")))

	provider := suite.createProvider(gspec.Requirements.Attributes)
	providerAddr, _ := sdk.AccAddressFromBech32(provider.Owner)
	roffer := mvbeta.ResourceOfferFromRU(gspec.Resources)

	suite.PrepareMocks(func(ts *state.TestSuite) {
		ts.MockBMEForDeposit(providerAddr, mvbeta.DefaultBidMinDepositACT)
	})

	hugeWindow := 10000 * time.Hour // above 720h governance max
	bmsg := &mvbeta.MsgCreateBid{
		ID:    mv1.MakeBidID(order.ID, providerAddr),
		Price: order.Price(),
		Deposit: deposit.Deposit{
			Amount:  mvbeta.DefaultBidMinDepositACT,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
		ResourcesOffer:    roffer,
		ReclamationWindow: &hugeWindow,
	}

	res, err := suite.handler(suite.Context(), bmsg)
	require.Nil(t, res)
	require.Error(t, err)
	require.ErrorIs(t, err, mv1.ErrReclamationWindowInvalid)
}

func TestCreateBid_ReclamationWindowValid_NoOrderRequirement(t *testing.T) {
	suite := setupTestSuite(t)

	// Order does NOT require reclamation, but provider voluntarily offers it
	order, gspec := suite.createOrder(testutil.Resources(t, testutil.WithDenom("uact")))

	provider := suite.createProvider(gspec.Requirements.Attributes)
	providerAddr, _ := sdk.AccAddressFromBech32(provider.Owner)
	roffer := mvbeta.ResourceOfferFromRU(gspec.Resources)

	suite.PrepareMocks(func(ts *state.TestSuite) {
		ts.MockBMEForDeposit(providerAddr, mvbeta.DefaultBidMinDepositACT)
	})

	validWindow := 24 * time.Hour
	bmsg := &mvbeta.MsgCreateBid{
		ID:    mv1.MakeBidID(order.ID, providerAddr),
		Price: order.Price(),
		Deposit: deposit.Deposit{
			Amount:  mvbeta.DefaultBidMinDepositACT,
			Sources: deposit.Sources{deposit.SourceBalance},
		},
		ResourcesOffer:    roffer,
		ReclamationWindow: &validWindow,
	}

	res, err := suite.handler(suite.Context(), bmsg)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify bid has reclamation window
	bidID := mv1.MakeBidID(order.ID, providerAddr)
	bid, found := suite.MarketKeeper().GetBid(suite.Context(), bidID)
	require.True(t, found)
	require.NotNil(t, bid.ReclamationWindow)
	assert.Equal(t, 24*time.Hour, *bid.ReclamationWindow)
}

// ===========================
// Order Reclamation Propagation Tests
// ===========================

func TestOrder_RequiresReclamation(t *testing.T) {
	suite := setupTestSuite(t)

	reclamation := &dv1.DeploymentReclamation{
		MinWindow: 24 * time.Hour,
	}
	order, _ := suite.createOrderWithReclamation(testutil.Resources(t), reclamation)

	assert.True(t, order.RequiresReclamation())
	require.NotNil(t, order.Reclamation)
	assert.Equal(t, 24*time.Hour, order.Reclamation.MinWindow)
}

func TestOrder_DoesNotRequireReclamation(t *testing.T) {
	suite := setupTestSuite(t)

	order, _ := suite.createOrder(testutil.Resources(t))

	assert.False(t, order.RequiresReclamation())
	assert.Nil(t, order.Reclamation)
}
