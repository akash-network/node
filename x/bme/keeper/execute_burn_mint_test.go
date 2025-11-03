package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/go/sdkutil"
	"pkg.akt.dev/go/testutil"

	types "pkg.akt.dev/go/node/bme/v1"

	"pkg.akt.dev/node/v2/testutil/state"
	"pkg.akt.dev/node/v2/x/bme/keeper"
)

type bmeSuite struct {
	*state.TestSuite
	t      *testing.T
	ctx    sdk.Context
	keeper keeper.Keeper
}

func setupBMETest(t *testing.T) *bmeSuite {
	t.Helper()

	ssuite := state.SetupTestSuite(t)

	// Block height must be > 0 for LedgerRecordID codec to allocate
	// buffer space for Height/Sequence fields
	ssuite.SetBlockHeight(1)
	ctx := ssuite.Context()

	k := ssuite.BmeKeeper()

	// Initialize genesis: sets params, status (HaltCR), mintEpoch
	k.InitGenesis(ctx, types.DefaultGenesisState())

	// Feed oracle price for AKT ($3.00).
	// ACT is hardcoded to $1.00 in the oracle (always pegged).
	// Oracle normalizes uakt→akt internally, so we feed the base denom.
	pf := ssuite.PriceFeeder()
	pf.SetPrice(sdkutil.DenomAkt, sdkmath.LegacyMustNewDecFromStr("3.0"))
	require.NoError(t, pf.FeedPrice(ctx, sdkutil.DenomAkt))

	// Reset ledger sequence
	require.NoError(t, k.BeginBlocker(ctx))

	return &bmeSuite{
		TestSuite: ssuite,
		t:         t,
		ctx:       ctx,
		keeper:    k,
	}
}

// requestBurnMint creates a pending burn/mint record with proper bank mocks.
func (s *bmeSuite) requestBurnMint(srcAddr, dstAddr sdk.AccAddress, burnCoin sdk.Coin, toDenom string) types.LedgerRecordID {
	s.t.Helper()

	// Mock SendCoinsFromAccountToModule for RequestBurnMint
	s.BankKeeper().
		On("SendCoinsFromAccountToModule",
			mock.Anything,
			srcAddr,
			types.ModuleName,
			sdk.Coins{burnCoin},
		).
		Return(nil).Once()

	id, err := s.keeper.RequestBurnMint(s.ctx, srcAddr, dstAddr, burnCoin, toDenom)
	require.NoError(s.t, err)

	return id
}

// assertNoRecords is a helper to verify no pending/failed/executed records remain.
func (s *bmeSuite) assertPendingCount(expected int) {
	s.t.Helper()
	count := 0
	err := s.keeper.IterateLedgerPendingRecords(s.ctx, func(_ types.LedgerRecordID, _ types.LedgerPendingRecord) (bool, error) {
		count++
		return false, nil
	})
	require.NoError(s.t, err)
	require.Equal(s.t, expected, count, "unexpected pending record count")
}

func (s *bmeSuite) assertFailedCount(expected int) {
	s.t.Helper()
	count := 0
	err := s.keeper.IterateLedgerCanceledRecords(s.ctx, func(_ types.LedgerRecordID, _ types.LedgerCanceledRecord) (bool, error) {
		count++
		return false, nil
	})
	require.NoError(s.t, err)
	require.Equal(s.t, expected, count, "unexpected failed record count")
}

func (s *bmeSuite) assertExecutedCount(expected int) {
	s.t.Helper()
	count := 0
	err := s.keeper.IterateLedgerRecords(s.ctx, func(_ types.LedgerRecordID, _ types.LedgerRecord) (bool, error) {
		count++
		return false, nil
	})
	require.NoError(s.t, err)
	require.Equal(s.t, expected, count, "unexpected executed record count")
}

// TestExecuteBurnMint_EpsilonFailure_RefundsAndRecordsFailed tests that when a
// burn/mint conversion result is below the denom precision (ErrEpsilon), the
// operation is recorded as failed, the user is refunded, and the event is emitted.
//
// With default oracle prices (AKT=$3.00, ACT=$1.00):
//
//	1 uact → uakt: swapRate = 1/3 = 0.333...333 (18-decimal)
//	mintAmountDec = 1 * 0.333...333 = 0.333...333 < 1 → ErrEpsilon
func TestExecuteBurnMint_EpsilonFailure_RefundsAndRecordsFailed(t *testing.T) {
	suite := setupBMETest(t)

	srcAddr := testutil.AccAddress(t)
	dstAddr := testutil.AccAddress(t)
	burnCoin := sdk.NewInt64Coin(sdkutil.DenomUact, 1)

	id := suite.requestBurnMint(srcAddr, dstAddr, burnCoin, sdkutil.DenomUakt)

	// Mock refund: failBurnMint sends coins back to srcAddr
	suite.BankKeeper().
		On("SendCoinsFromModuleToAccount",
			mock.Anything,
			types.ModuleName,
			srcAddr,
			sdk.NewCoins(burnCoin),
		).
		Return(nil).Once()

	// Execute EndBlocker — processes uact→uakt pending records
	require.NoError(t, suite.keeper.EndBlocker(suite.ctx))

	// Verify: pending cleared, one failed record, no executed
	suite.assertPendingCount(0)
	suite.assertExecutedCount(0)

	// Verify: one failed record with correct data
	failedCount := 0
	err := suite.keeper.IterateLedgerCanceledRecords(suite.ctx, func(failedID types.LedgerRecordID, record types.LedgerCanceledRecord) (bool, error) {
		failedCount++
		require.Equal(t, id, failedID)
		require.Equal(t, types.BMCancelReasonEpsilon, record.CancelReason)
		require.Equal(t, srcAddr.String(), record.Owner)
		require.Equal(t, dstAddr.String(), record.To)
		require.Equal(t, burnCoin, record.CoinsToBurn)
		require.Equal(t, sdkutil.DenomUakt, record.DenomToMint)
		return false, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, failedCount, "expected exactly 1 failed record")

	// Verify: refund was called
	suite.BankKeeper().AssertCalled(t, "SendCoinsFromModuleToAccount",
		mock.Anything, types.ModuleName, srcAddr, sdk.NewCoins(burnCoin))
}

// TestExecuteBurnMint_EpsilonBoundary_3uact tests that 3 uact still triggers
// ErrEpsilon at AKT=$3.00 due to 18-decimal precision loss:
//
//	swapRate = 1/3 = 0.333333333333333333 (truncated)
//	3 * 0.333333333333333333 = 0.999999999999999999 < 1 → ErrEpsilon
func TestExecuteBurnMint_EpsilonBoundary_3uact(t *testing.T) {
	suite := setupBMETest(t)

	srcAddr := testutil.AccAddress(t)
	dstAddr := testutil.AccAddress(t)
	burnCoin := sdk.NewInt64Coin(sdkutil.DenomUact, 3)

	suite.requestBurnMint(srcAddr, dstAddr, burnCoin, sdkutil.DenomUakt)

	// Mock refund
	suite.BankKeeper().
		On("SendCoinsFromModuleToAccount",
			mock.Anything,
			types.ModuleName,
			srcAddr,
			sdk.NewCoins(burnCoin),
		).
		Return(nil).Once()

	require.NoError(t, suite.keeper.EndBlocker(suite.ctx))

	// Verify: failed due to precision loss, not executed
	suite.assertPendingCount(0)
	suite.assertExecutedCount(0)

	failedCount := 0
	err := suite.keeper.IterateLedgerCanceledRecords(suite.ctx, func(_ types.LedgerRecordID, record types.LedgerCanceledRecord) (bool, error) {
		failedCount++
		require.Equal(t, types.BMCancelReasonEpsilon, record.CancelReason)
		return false, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, failedCount)

	suite.BankKeeper().AssertCalled(t, "SendCoinsFromModuleToAccount",
		mock.Anything, types.ModuleName, srcAddr, sdk.NewCoins(burnCoin))
}

// TestExecuteBurnMint_EpsilonBoundary_4uact_Succeeds tests the minimum amount
// that passes the epsilon check at AKT=$3.00:
//
//	swapRate = 1/3 = 0.333333333333333333
//	4 * 0.333333333333333333 = 1.333333333333333332 → TruncateInt = 1 uakt → succeeds
func TestExecuteBurnMint_EpsilonBoundary_4uact_Succeeds(t *testing.T) {
	suite := setupBMETest(t)

	srcAddr := testutil.AccAddress(t)
	dstAddr := testutil.AccAddress(t)
	burnCoin := sdk.NewInt64Coin(sdkutil.DenomUact, 4)

	suite.requestBurnMint(srcAddr, dstAddr, burnCoin, sdkutil.DenomUakt)

	// burnACT path: burns ACT, mints AKT, sends to dstAddr
	// At $3 AKT price: 4 uact → 1 uakt (after truncation)
	mintCoin := sdk.NewInt64Coin(sdkutil.DenomUakt, 1)

	// Mock BurnCoins for ACT burn
	suite.BankKeeper().
		On("BurnCoins",
			mock.Anything,
			types.ModuleName,
			sdk.NewCoins(burnCoin),
		).
		Return(nil).Once()

	// Mock MintCoins for AKT mint (remintCredit starts at 0, full mint needed)
	suite.BankKeeper().
		On("MintCoins",
			mock.Anything,
			types.ModuleName,
			sdk.NewCoins(mintCoin),
		).
		Return(nil).Once()

	// Mock SendCoinsFromModuleToAccount for delivering minted coins to dstAddr
	suite.BankKeeper().
		On("SendCoinsFromModuleToAccount",
			mock.Anything,
			types.ModuleName,
			dstAddr,
			sdk.NewCoins(mintCoin),
		).
		Return(nil).Once()

	require.NoError(t, suite.keeper.EndBlocker(suite.ctx))

	// Verify: success path
	suite.assertPendingCount(0)
	suite.assertFailedCount(0)
	suite.assertExecutedCount(1)
}

// TestExecuteBurnMint_LargeAmount_Succeeds verifies that normal-sized burn/mint
// operations are unaffected by the epsilon check.
//
//	1,000,000 uact at $3 AKT: mintAmountDec = 1,000,000 * 0.333...333 = 333,333.333...333
//	TruncateInt = 333,333 uakt
func TestExecuteBurnMint_LargeAmount_Succeeds(t *testing.T) {
	suite := setupBMETest(t)

	srcAddr := testutil.AccAddress(t)
	dstAddr := testutil.AccAddress(t)
	burnCoin := sdk.NewInt64Coin(sdkutil.DenomUact, 1000000) // 1 ACT

	suite.requestBurnMint(srcAddr, dstAddr, burnCoin, sdkutil.DenomUakt)

	// At $3 AKT: 1,000,000 uact → 333,333 uakt
	mintCoin := sdk.NewInt64Coin(sdkutil.DenomUakt, 333333)

	suite.BankKeeper().
		On("BurnCoins",
			mock.Anything,
			types.ModuleName,
			sdk.NewCoins(burnCoin),
		).
		Return(nil).Once()

	suite.BankKeeper().
		On("MintCoins",
			mock.Anything,
			types.ModuleName,
			sdk.NewCoins(mintCoin),
		).
		Return(nil).Once()

	suite.BankKeeper().
		On("SendCoinsFromModuleToAccount",
			mock.Anything,
			types.ModuleName,
			dstAddr,
			sdk.NewCoins(mintCoin),
		).
		Return(nil).Once()

	require.NoError(t, suite.keeper.EndBlocker(suite.ctx))

	// Verify success
	suite.assertPendingCount(0)
	suite.assertFailedCount(0)
	suite.assertExecutedCount(1)
}

// TestExecuteBurnMint_SettleSpread_ReducesOutput verifies that a non-zero
// settle_spread_bps mints the full amount but sends less to the user.
// The spread surplus stays in the module account (vault).
//
//	1,000,000 uact at $3 AKT:
//	  mintAmountDec = 333,333.333...  → mintAmount = 333,333 uakt (full mint)
//	  spreadDec = 333,333.333... * 100/10000 = 3,333.333... → spread = 3,333 uakt
//	  userCoin = 333,333 - 3,333 = 330,000 uakt
func TestExecuteBurnMint_SettleSpread_ReducesOutput(t *testing.T) {
	suite := setupBMETest(t)

	// Set settle spread to 100 bps (1%)
	params, err := suite.keeper.GetParams(suite.ctx)
	require.NoError(t, err)
	params.SettleSpreadBps = 100
	require.NoError(t, suite.keeper.SetParams(suite.ctx, params))

	srcAddr := testutil.AccAddress(t)
	dstAddr := testutil.AccAddress(t)
	burnCoin := sdk.NewInt64Coin(sdkutil.DenomUact, 1000000)

	suite.requestBurnMint(srcAddr, dstAddr, burnCoin, sdkutil.DenomUakt)

	// Full mint amount (no reduction)
	fullMintCoin := sdk.NewInt64Coin(sdkutil.DenomUakt, 333333)
	// User receives full minus spread (333,333 - 3,333 = 330,000)
	userCoin := sdk.NewInt64Coin(sdkutil.DenomUakt, 330000)

	suite.BankKeeper().
		On("BurnCoins", mock.Anything, types.ModuleName, sdk.NewCoins(burnCoin)).
		Return(nil).Once()
	suite.BankKeeper().
		On("MintCoins", mock.Anything, types.ModuleName, sdk.NewCoins(fullMintCoin)).
		Return(nil).Once()
	suite.BankKeeper().
		On("SendCoinsFromModuleToAccount", mock.Anything, types.ModuleName, dstAddr, sdk.NewCoins(userCoin)).
		Return(nil).Once()

	require.NoError(t, suite.keeper.EndBlocker(suite.ctx))

	suite.assertPendingCount(0)
	suite.assertFailedCount(0)
	suite.assertExecutedCount(1)
}

// TestExecuteBurnMint_ZeroSettleSpread_NoChange verifies that the default
// settle_spread_bps=0 does not alter the output (no regression).
func TestExecuteBurnMint_ZeroSettleSpread_NoChange(t *testing.T) {
	suite := setupBMETest(t)

	srcAddr := testutil.AccAddress(t)
	dstAddr := testutil.AccAddress(t)
	burnCoin := sdk.NewInt64Coin(sdkutil.DenomUact, 1000000)

	suite.requestBurnMint(srcAddr, dstAddr, burnCoin, sdkutil.DenomUakt)

	// Default SettleSpreadBps=0, so output is unchanged: 333,333 uakt
	mintCoin := sdk.NewInt64Coin(sdkutil.DenomUakt, 333333)

	suite.BankKeeper().
		On("BurnCoins", mock.Anything, types.ModuleName, sdk.NewCoins(burnCoin)).
		Return(nil).Once()
	suite.BankKeeper().
		On("MintCoins", mock.Anything, types.ModuleName, sdk.NewCoins(mintCoin)).
		Return(nil).Once()
	suite.BankKeeper().
		On("SendCoinsFromModuleToAccount", mock.Anything, types.ModuleName, dstAddr, sdk.NewCoins(mintCoin)).
		Return(nil).Once()

	require.NoError(t, suite.keeper.EndBlocker(suite.ctx))

	suite.assertPendingCount(0)
	suite.assertFailedCount(0)
	suite.assertExecutedCount(1)
}

// TestExecuteBurnMint_MinMint_RejectsSmallSettle verifies that when MinMint
// includes a uakt threshold, ACT→AKT settlements below the minimum are canceled.
func TestExecuteBurnMint_MinMint_RejectsSmallSettle(t *testing.T) {
	suite := setupBMETest(t)

	// Set MinMint to require at least 500,000 uakt output
	params, err := suite.keeper.GetParams(suite.ctx)
	require.NoError(t, err)
	params.MinMint = sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 500000))
	require.NoError(t, suite.keeper.SetParams(suite.ctx, params))

	srcAddr := testutil.AccAddress(t)
	dstAddr := testutil.AccAddress(t)
	// 1M uact at $3 AKT → 333,333 uakt — below the 500,000 minimum
	burnCoin := sdk.NewInt64Coin(sdkutil.DenomUact, 1000000)

	suite.requestBurnMint(srcAddr, dstAddr, burnCoin, sdkutil.DenomUakt)

	// Mock refund
	suite.BankKeeper().
		On("SendCoinsFromModuleToAccount", mock.Anything, types.ModuleName, srcAddr, sdk.NewCoins(burnCoin)).
		Return(nil).Once()

	require.NoError(t, suite.keeper.EndBlocker(suite.ctx))

	suite.assertPendingCount(0)
	suite.assertExecutedCount(0)

	// Verify canceled with minimum mint reason
	canceledCount := 0
	err = suite.keeper.IterateLedgerCanceledRecords(suite.ctx, func(_ types.LedgerRecordID, record types.LedgerCanceledRecord) (bool, error) {
		canceledCount++
		require.Equal(t, srcAddr.String(), record.Owner)
		return false, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, canceledCount)
}

// TestExecuteBurnMint_MinMint_DefaultDoesNotAffectSettle verifies that the
// default MinMint (10M uact) does not affect ACT→AKT settlements since
// there is no uakt entry in the default MinMint.
func TestExecuteBurnMint_MinMint_DefaultDoesNotAffectSettle(t *testing.T) {
	suite := setupBMETest(t)

	srcAddr := testutil.AccAddress(t)
	dstAddr := testutil.AccAddress(t)
	// Small amount: 4 uact → 1 uakt, no MinMint for uakt in defaults
	burnCoin := sdk.NewInt64Coin(sdkutil.DenomUact, 4)

	suite.requestBurnMint(srcAddr, dstAddr, burnCoin, sdkutil.DenomUakt)

	mintCoin := sdk.NewInt64Coin(sdkutil.DenomUakt, 1)

	suite.BankKeeper().
		On("BurnCoins", mock.Anything, types.ModuleName, sdk.NewCoins(burnCoin)).
		Return(nil).Once()
	suite.BankKeeper().
		On("MintCoins", mock.Anything, types.ModuleName, sdk.NewCoins(mintCoin)).
		Return(nil).Once()
	suite.BankKeeper().
		On("SendCoinsFromModuleToAccount", mock.Anything, types.ModuleName, dstAddr, sdk.NewCoins(mintCoin)).
		Return(nil).Once()

	require.NoError(t, suite.keeper.EndBlocker(suite.ctx))

	suite.assertPendingCount(0)
	suite.assertFailedCount(0)
	suite.assertExecutedCount(1)
}

// TestExecuteBurnMint_SmallAmountWithSpread_SpreadTruncatesToZero verifies
// that when the spread amount truncates to zero (small mint), the user still
// receives the full amount — spread only takes effect on larger amounts.
//
//	4 uact at $3 AKT → mintAmountDec = 1.333... → mintAmount = 1 uakt
//	spreadDec = 1.333... * 1000/10000 = 0.1333... → spread = 0 (truncated)
//	userCoin = 1 - 0 = 1 uakt (user gets full amount)
func TestExecuteBurnMint_SmallAmountWithSpread_SpreadTruncatesToZero(t *testing.T) {
	suite := setupBMETest(t)

	// Set settle spread to 1000 bps (10% — the maximum allowed)
	params, err := suite.keeper.GetParams(suite.ctx)
	require.NoError(t, err)
	params.SettleSpreadBps = 1000
	require.NoError(t, suite.keeper.SetParams(suite.ctx, params))

	srcAddr := testutil.AccAddress(t)
	dstAddr := testutil.AccAddress(t)
	burnCoin := sdk.NewInt64Coin(sdkutil.DenomUact, 4)

	suite.requestBurnMint(srcAddr, dstAddr, burnCoin, sdkutil.DenomUakt)

	// Full mint = 1 uakt, spread truncates to 0, user gets full 1 uakt
	mintCoin := sdk.NewInt64Coin(sdkutil.DenomUakt, 1)

	suite.BankKeeper().
		On("BurnCoins", mock.Anything, types.ModuleName, sdk.NewCoins(burnCoin)).
		Return(nil).Once()
	suite.BankKeeper().
		On("MintCoins", mock.Anything, types.ModuleName, sdk.NewCoins(mintCoin)).
		Return(nil).Once()
	suite.BankKeeper().
		On("SendCoinsFromModuleToAccount", mock.Anything, types.ModuleName, dstAddr, sdk.NewCoins(mintCoin)).
		Return(nil).Once()

	require.NoError(t, suite.keeper.EndBlocker(suite.ctx))

	suite.assertPendingCount(0)
	suite.assertFailedCount(0)
	suite.assertExecutedCount(1)
}
