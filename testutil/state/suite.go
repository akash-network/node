package state

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"cosmossdk.io/collections"
	"cosmossdk.io/store"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"

	atypes "pkg.akt.dev/go/node/audit/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1"
	emodule "pkg.akt.dev/go/node/escrow/module"
	mv1 "pkg.akt.dev/go/node/market/v1"
	oracletypes "pkg.akt.dev/go/node/oracle/v1"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	ttypes "pkg.akt.dev/go/node/take/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/app"
	emocks "pkg.akt.dev/node/v2/testutil/cosmos/mocks"
	oracletestutil "pkg.akt.dev/node/v2/testutil/oracle"
	akeeper "pkg.akt.dev/node/v2/x/audit/keeper"
	dkeeper "pkg.akt.dev/node/v2/x/deployment/keeper"
	ekeeper "pkg.akt.dev/node/v2/x/escrow/keeper"
	mhooks "pkg.akt.dev/node/v2/x/market/hooks"
	mkeeper "pkg.akt.dev/node/v2/x/market/keeper"
	oraclekeeper "pkg.akt.dev/node/v2/x/oracle/keeper"
	pkeeper "pkg.akt.dev/node/v2/x/provider/keeper"
	tkeeper "pkg.akt.dev/node/v2/x/take/keeper"
)

// TestSuite encapsulates a functional Akash nodes data stores for
// ephemeral testing.
type TestSuite struct {
	t           testing.TB
	ms          store.CommitMultiStore
	ctx         sdk.Context
	app         *app.AkashApp
	keepers     Keepers
	priceFeeder *oracletestutil.PriceFeeder
}

type Keepers struct {
	Account    *emocks.AccountKeeper
	Audit      akeeper.IKeeper
	Authz      *emocks.AuthzKeeper
	Bank       *emocks.BankKeeper
	Deployment dkeeper.IKeeper
	Escrow     ekeeper.Keeper
	Market     mkeeper.IKeeper
	Oracle     oraclekeeper.Keeper
	Provider   pkeeper.IKeeper
	Take       tkeeper.IKeeper
}

// SetupTestSuite provides toolkit for accessing stores and keepers
// for complex data interactions.
func SetupTestSuite(t testing.TB) *TestSuite {
	return SetupTestSuiteWithKeepers(t, Keepers{})
}

func SetupTestSuiteWithKeepers(t testing.TB, keepers Keepers) *TestSuite {
	dir, err := os.MkdirTemp("", "akashd-test-home")
	if err != nil {
		panic(fmt.Sprintf("failed creating temporary directory: %v", err))
	}

	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	if keepers.Bank == nil {
		bkeeper := &emocks.BankKeeper{}
		// do not set bank mock during suite setup, each test must set them manually
		// to make sure escrow balance values are tracked correctly
		bkeeper.
			On("SpendableCoin", mock.Anything, mock.Anything, mock.MatchedBy(func(denom string) bool {
				matched := denom == sdkutil.DenomUakt || denom == sdkutil.DenomUact
				return matched
			})).
			Return(func(_ context.Context, _ sdk.AccAddress, denom string) sdk.Coin {
				if denom == sdkutil.DenomUakt {
					return sdk.NewInt64Coin(sdkutil.DenomUakt, 10000000)
				}
				return sdk.NewInt64Coin("uact", 1800000)
			})

		// Mock GetSupply for BME collateral ratio checks
		bkeeper.
			On("GetSupply", mock.Anything, mock.MatchedBy(func(denom string) bool {
				return denom == sdkutil.DenomUakt || denom == sdkutil.DenomUact
			})).
			Return(func(ctx context.Context, denom string) sdk.Coin {
				if denom == sdkutil.DenomUakt {
					return sdk.NewInt64Coin(sdkutil.DenomUakt, 1000000000000) // 1T uakt total supply
				}
				// For CR calculation: CR = (BME_uakt_balance * swap_rate) / total_uact_supply
				// Target CR > 100% for tests: (600B * 3.0) / 1.8T = 1800B / 1800B = 1.0 = 100%
				return sdk.NewInt64Coin(sdkutil.DenomUact, 1800000000000) // 1.8T uact total supply
			})

		// Mock GetBalance for BME module account balance checks
		bkeeper.
			On("GetBalance", mock.Anything, mock.Anything, mock.MatchedBy(func(denom string) bool {
				return denom == sdkutil.DenomUakt || denom == sdkutil.DenomUact
			})).
			Return(func(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin {
				if denom == sdkutil.DenomUakt {
					// BME module should have enough uakt to maintain healthy CR
					return sdk.NewInt64Coin(sdkutil.DenomUakt, 600000000000) // 600B uakt in BME module
				}
				return sdk.NewInt64Coin(sdkutil.DenomUact, 100000000000) // 100B uact in BME module
			})

		keepers.Bank = bkeeper
	}

	if keepers.Authz == nil {
		keeper := &emocks.AuthzKeeper{}

		keepers.Authz = keeper
	}

	if keepers.Account == nil {
		akeeper := &emocks.AccountKeeper{}

		// Mock GetModuleAddress to return deterministic addresses for module accounts
		akeeper.
			On("GetModuleAddress", mock.Anything).
			Return(func(moduleName string) sdk.AccAddress {
				// Generate deterministic module addresses based on module name
				return authtypes.NewModuleAddress(moduleName)
			})

		keepers.Account = akeeper
	}

	app := app.Setup(
		app.WithCheckTx(false),
		app.WithHome(dir),
		app.WithGenesis(app.GenesisStateWithValSet),
	)

	ctx := app.NewContext(false)
	cdc := app.AppCodec()

	vals, err := app.Keepers.Cosmos.Staking.GetAllValidators(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Manually set validator signing info, otherwise we panic
	for _, val := range vals {
		consAddr, _ := val.GetConsAddr()
		signingInfo := slashingtypes.NewValidatorSigningInfo(
			consAddr,
			0,
			ctx.BlockHeight(),
			time.Unix(0, 0),
			false,
			0,
		)
		err = app.Keepers.Cosmos.Slashing.SetValidatorSigningInfo(ctx, consAddr, signingInfo)
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	if keepers.Audit == nil {
		keepers.Audit = akeeper.NewKeeper(cdc, app.GetKey(atypes.StoreKey))
	}

	if keepers.Oracle == nil {
		keepers.Oracle = oraclekeeper.NewKeeper(cdc, app.GetKey(oracletypes.StoreKey), authtypes.NewModuleAddress(govtypes.ModuleName).String())
	}

	if keepers.Take == nil {
		keepers.Take = tkeeper.NewKeeper(cdc, app.GetKey(ttypes.StoreKey), authtypes.NewModuleAddress(govtypes.ModuleName).String())
	}
	if keepers.Escrow == nil {
		storeService := runtime.NewKVStoreService(app.GetKey(distrtypes.StoreKey))
		sb := collections.NewSchemaBuilder(storeService)
		feepool := collections.NewItem(sb, distrtypes.FeePoolKey, "fee_pool", codec.CollValue[distrtypes.FeePool](cdc))
		keepers.Escrow = ekeeper.NewKeeper(cdc, app.GetKey(emodule.StoreKey), keepers.Bank, keepers.Take, keepers.Authz, feepool)
	}
	if keepers.Market == nil {
		keepers.Market = mkeeper.NewKeeper(cdc, app.GetKey(mv1.StoreKey), keepers.Escrow, authtypes.NewModuleAddress(govtypes.ModuleName).String())

	}
	if keepers.Deployment == nil {
		keepers.Deployment = dkeeper.NewKeeper(cdc, app.GetKey(dtypes.StoreKey), keepers.Escrow, authtypes.NewModuleAddress(govtypes.ModuleName).String())
	}
	if keepers.Provider == nil {
		keepers.Provider = pkeeper.NewKeeper(cdc, app.GetKey(ptypes.StoreKey))
	}

	hook := mhooks.New(keepers.Deployment, keepers.Market)

	keepers.Escrow.AddOnAccountClosedHook(hook.OnEscrowAccountClosed)
	keepers.Escrow.AddOnPaymentClosedHook(hook.OnEscrowPaymentClosed)

	// Initialize price feeder for oracle testing
	priceFeeder, err := oracletestutil.SetupPriceFeeder(ctx, keepers.Oracle)
	if err != nil {
		t.Fatal("failed to setup price feeder:", err)
	}

	// Feed initial prices (AKT/USD = $3.00)
	if err := priceFeeder.FeedPrices(ctx); err != nil {
		t.Fatal("failed to feed initial prices:", err)
	}

	return &TestSuite{
		t:           t,
		app:         app,
		ctx:         ctx,
		keepers:     keepers,
		priceFeeder: priceFeeder,
	}
}

func (ts *TestSuite) PrepareMocks(fn func(ts *TestSuite)) {
	fn(ts)
}

func (ts *TestSuite) App() *app.AkashApp {
	return ts.app
}

// SetBlockHeight provides arbitrarily setting the chain's block height.
func (ts *TestSuite) SetBlockHeight(height int64) {
	ts.ctx = ts.ctx.WithBlockHeight(height)
}

// Store provides access to the underlying KVStore
func (ts *TestSuite) Store() store.CommitMultiStore {
	return ts.ms
}

// Context of the current mempool
func (ts *TestSuite) Context() sdk.Context {
	return ts.ctx
}

// AuditKeeper key store
func (ts *TestSuite) AuditKeeper() akeeper.IKeeper {
	return ts.keepers.Audit
}

// EscrowKeeper key store
func (ts *TestSuite) EscrowKeeper() ekeeper.Keeper {
	return ts.keepers.Escrow
}

// MarketKeeper key store
func (ts *TestSuite) MarketKeeper() mkeeper.IKeeper {
	return ts.keepers.Market
}

// DeploymentKeeper key store
func (ts *TestSuite) DeploymentKeeper() dkeeper.IKeeper {
	return ts.keepers.Deployment
}

// ProviderKeeper key store
func (ts *TestSuite) ProviderKeeper() pkeeper.IKeeper {
	return ts.keepers.Provider
}

// BankKeeper key store
func (ts *TestSuite) BankKeeper() *emocks.BankKeeper {
	return ts.keepers.Bank
}

// AuthzKeeper key store
func (ts *TestSuite) AuthzKeeper() *emocks.AuthzKeeper {
	return ts.keepers.Authz
}

// OracleKeeper key store
func (ts *TestSuite) OracleKeeper() oraclekeeper.Keeper {
	return ts.keepers.Oracle
}

// PriceFeeder returns the oracle price feeder for testing
func (ts *TestSuite) PriceFeeder() *oracletestutil.PriceFeeder {
	return ts.priceFeeder
}
