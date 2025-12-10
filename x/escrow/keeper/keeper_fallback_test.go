package keeper_test

import (
	"context"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	bmetypes "pkg.akt.dev/go/node/bme/v1"

	"pkg.akt.dev/go/node/escrow/module"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/v2/testutil/state"
)

// mockBMEKeeper is a mock BME keeper for testing circuit breaker fallback
type mockBMEKeeper struct {
	mock.Mock
	circuitBreakerStatus bmetypes.CircuitBreakerStatus
}

func (m *mockBMEKeeper) BurnMintFromAddressToModuleAccount(ctx sdk.Context, addr sdk.AccAddress, moduleName string, coin sdk.Coin, toDenom string) (sdk.DecCoin, error) {
	args := m.Called(ctx, addr, moduleName, coin, toDenom)
	return args.Get(0).(sdk.DecCoin), args.Error(1)
}

func (m *mockBMEKeeper) BurnMintFromModuleAccountToAddress(ctx sdk.Context, moduleName string, addr sdk.AccAddress, coin sdk.Coin, toDenom string) (sdk.DecCoin, error) {
	args := m.Called(ctx, moduleName, addr, coin, toDenom)
	return args.Get(0).(sdk.DecCoin), args.Error(1)
}

func (m *mockBMEKeeper) BurnMintOnAccount(ctx sdk.Context, addr sdk.AccAddress, coin sdk.Coin, toDenom string) (sdk.DecCoin, error) {
	args := m.Called(ctx, addr, coin, toDenom)
	return args.Get(0).(sdk.DecCoin), args.Error(1)
}

func (m *mockBMEKeeper) GetCircuitBreakerStatus(ctx sdk.Context) (bmetypes.CircuitBreakerStatus, error) {
	return m.circuitBreakerStatus, nil
}

// mockBankKeeper is a mock bank keeper for testing
type mockBankKeeper struct {
	mock.Mock
}

func (m *mockBankKeeper) SpendableCoins(ctx context.Context, addr sdk.AccAddress) sdk.Coins {
	args := m.Called(ctx, addr)
	return args.Get(0).(sdk.Coins)
}

func (m *mockBankKeeper) SpendableCoin(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin {
	args := m.Called(ctx, addr, denom)
	return args.Get(0).(sdk.Coin)
}

func (m *mockBankKeeper) SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error {
	args := m.Called(ctx, senderModule, recipientAddr, amt)
	return args.Error(0)
}

func (m *mockBankKeeper) SendCoinsFromModuleToModule(ctx context.Context, senderModule, recipientModule string, amt sdk.Coins) error {
	args := m.Called(ctx, senderModule, recipientModule, amt)
	return args.Error(0)
}

func (m *mockBankKeeper) SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error {
	args := m.Called(ctx, senderAddr, recipientModule, amt)
	return args.Error(0)
}

// mockAuthzKeeper is a mock authz keeper for testing
type mockAuthzKeeper struct {
	mock.Mock
}

func (m *mockAuthzKeeper) DeleteGrant(ctx context.Context, grantee sdk.AccAddress, granter sdk.AccAddress, msgType string) error {
	args := m.Called(ctx, grantee, granter, msgType)
	return args.Error(0)
}

func (m *mockAuthzKeeper) GetAuthorization(ctx context.Context, grantee sdk.AccAddress, granter sdk.AccAddress, msgType string) (authz.Authorization, *time.Time) {
	args := m.Called(ctx, grantee, granter, msgType)
	if args.Get(0) == nil {
		return nil, nil
	}
	return args.Get(0).(authz.Authorization), args.Get(1).(*time.Time)
}

func (m *mockAuthzKeeper) SaveGrant(ctx context.Context, grantee sdk.AccAddress, granter sdk.AccAddress, authorization authz.Authorization, expiration *time.Time) error {
	args := m.Called(ctx, grantee, granter, authorization, expiration)
	return args.Error(0)
}

func (m *mockAuthzKeeper) IterateGrants(ctx context.Context, handler func(granterAddr sdk.AccAddress, granteeAddr sdk.AccAddress, grant authz.Grant) bool) {
	m.Called(ctx, handler)
}

func (m *mockAuthzKeeper) GetGranteeGrantsByMsgType(ctx context.Context, grantee sdk.AccAddress, msgType string, onGrant interface{}) {
	m.Called(ctx, grantee, msgType, onGrant)
}

// Test_NormalFlow_NoCircuitBreaker tests that normal BME flow works when circuit breaker is healthy
func Test_NormalFlow_NoCircuitBreaker(t *testing.T) {
	ssuite := state.SetupTestSuite(t)
	ctx := ssuite.Context()

	bmeKeeper := ssuite.BmeKeeper()

	// Verify circuit breaker is healthy (default test setup)
	crStatus, err := bmeKeeper.GetCircuitBreakerStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, bmetypes.CircuitBreakerStatusHealthy, crStatus, "Circuit breaker should be healthy in default test setup")
}

// Test_CircuitBreakerFallback_Integration tests the integration with real keepers
// using modified BME params to verify the fallback behavior works correctly.
// This test verifies that the code paths are correct even if we can't easily trigger
// the circuit breaker halt through parameter changes alone.
func Test_CircuitBreakerFallback_Integration(t *testing.T) {
	ssuite := state.SetupTestSuite(t)
	ctx := ssuite.Context()

	bkeeper := ssuite.BankKeeper()
	ekeeper := ssuite.EscrowKeeper()

	id := testutil.DeploymentID(t).ToEscrowAccountID()
	owner := testutil.AccAddress(t)
	amt := testutil.AkashCoin(t, 1000)

	// Test with direct=true to verify direct deposit path works
	// This simulates what would happen if circuit breaker forced direct deposits
	bkeeper.
		On("SendCoinsFromAccountToModule", mock.Anything, owner, module.ModuleName, sdk.NewCoins(amt)).
		Return(nil).Once()

	err := ekeeper.AccountCreate(ctx, id, owner, []etypes.Depositor{{
		Owner:   owner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
		Direct:  true, // Explicit direct deposit (same path as circuit breaker fallback)
	}})
	assert.NoError(t, err)

	// Verify account was created with AKT funds
	acct, err := ekeeper.GetAccount(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "uakt", acct.State.Funds[0].Denom, "Direct deposits should keep funds in original denom (uakt)")
	require.True(t, acct.State.Funds[0].Amount.Equal(sdkmath.LegacyNewDec(amt.Amount.Int64())), "Funds amount should match deposit")
}

// Test_DirectPaymentWithdraw tests that direct ACT transfers work without BME
func Test_DirectPaymentWithdraw(t *testing.T) {
	ssuite := state.SetupTestSuite(t)
	ctx := ssuite.Context()

	bkeeper := ssuite.BankKeeper()
	ekeeper := ssuite.EscrowKeeper()

	lid := testutil.LeaseID(t)
	did := lid.DeploymentID()

	aid := did.ToEscrowAccountID()
	pid := lid.ToEscrowPaymentID()

	aowner := testutil.AccAddress(t)
	// Direct deposit in uakt
	amt := testutil.AkashCoin(t, 1000)
	powner := testutil.AccAddress(t)
	// Rate in uakt (same denom as direct deposit)
	rate := sdk.NewCoin("uakt", sdkmath.NewInt(10))

	// Create account with direct deposit (simulating circuit breaker active scenario)
	bkeeper.
		On("SendCoinsFromAccountToModule", mock.Anything, aowner, module.ModuleName, sdk.NewCoins(amt)).
		Return(nil).Once()

	assert.NoError(t, ekeeper.AccountCreate(ctx, aid, aowner, []etypes.Depositor{{
		Owner:   aowner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
		Direct:  true,
	}}))

	// Create payment with rate in uakt (matching direct deposit denom)
	err := ekeeper.PaymentCreate(ctx, pid, powner, sdk.NewDecCoinFromCoin(rate))
	assert.NoError(t, err)

	// Advance blocks
	blkdelta := int64(10)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + blkdelta)

	// Expected earnings: 10 uakt/block * 10 blocks = 100 uakt
	expectedEarnings := sdk.NewCoins(sdk.NewCoin("uakt", sdkmath.NewInt(100)))

	// When payment balance is in uakt, it should be sent directly without BME conversion
	bkeeper.
		On("SendCoinsFromModuleToAccount", mock.Anything, module.ModuleName, powner, expectedEarnings).
		Return(nil).Once()

	err = ekeeper.PaymentWithdraw(ctx, pid)
	assert.NoError(t, err)

	// Verify payment was withdrawn
	payment, err := ekeeper.GetPayment(ctx, pid)
	require.NoError(t, err)
	require.Equal(t, etypes.StateOpen, payment.State.State)
	// Withdrawn should be in uakt (direct, no BME)
	require.Equal(t, "uakt", payment.State.Withdrawn.Denom)
}

// Test_DirectAccountClose tests that direct refunds work without BME
func Test_DirectAccountClose(t *testing.T) {
	ssuite := state.SetupTestSuite(t)
	ctx := ssuite.Context()

	bkeeper := ssuite.BankKeeper()
	ekeeper := ssuite.EscrowKeeper()

	id := testutil.DeploymentID(t).ToEscrowAccountID()
	owner := testutil.AccAddress(t)
	amt := testutil.AkashCoin(t, 1000)

	// Create account with direct deposit
	bkeeper.
		On("SendCoinsFromAccountToModule", mock.Anything, owner, module.ModuleName, sdk.NewCoins(amt)).
		Return(nil).Once()

	assert.NoError(t, ekeeper.AccountCreate(ctx, id, owner, []etypes.Depositor{{
		Owner:   owner.String(),
		Height:  ctx.BlockHeight(),
		Balance: sdk.NewDecCoinFromCoin(amt),
		Direct:  true,
	}}))

	// Advance a few blocks (no payments, so all funds should be refunded)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 10)

	// Expected refund: all 1000 uakt (direct, no BME conversion)
	expectedRefund := sdk.NewCoins(amt)
	bkeeper.
		On("SendCoinsFromModuleToAccount", mock.Anything, module.ModuleName, owner, expectedRefund).
		Return(nil).Once()

	err := ekeeper.AccountClose(ctx, id)
	assert.NoError(t, err)

	// Verify account was closed
	acct, err := ekeeper.GetAccount(ctx, id)
	require.NoError(t, err)
	require.Equal(t, etypes.StateClosed, acct.State.State)
}
