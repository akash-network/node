package state

import (
	"fmt"
	"os"
	"testing"
	"time"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/stretchr/testify/mock"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"

	atypes "pkg.akt.dev/go/node/audit/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1"
	etypes "pkg.akt.dev/go/node/escrow/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	ttypes "pkg.akt.dev/go/node/take/v1"

	"pkg.akt.dev/node/app"

	emocks "pkg.akt.dev/node/testutil/cosmos/mocks"
	akeeper "pkg.akt.dev/node/x/audit/keeper"
	dkeeper "pkg.akt.dev/node/x/deployment/keeper"
	ekeeper "pkg.akt.dev/node/x/escrow/keeper"
	mhooks "pkg.akt.dev/node/x/market/hooks"
	mkeeper "pkg.akt.dev/node/x/market/keeper"
	pkeeper "pkg.akt.dev/node/x/provider/keeper"
	tkeeper "pkg.akt.dev/node/x/take/keeper"
)

// TestSuite encapsulates a functional Akash nodes data stores for
// ephemeral testing.
type TestSuite struct {
	t       testing.TB
	ms      sdk.CommitMultiStore
	ctx     sdk.Context
	app     *app.AkashApp
	keepers Keepers
}

type Keepers struct {
	Take       tkeeper.IKeeper
	Escrow     ekeeper.Keeper
	Audit      akeeper.IKeeper
	Market     mkeeper.IKeeper
	Deployment dkeeper.IKeeper
	Provider   pkeeper.IKeeper
	Bank       *emocks.BankKeeper
	Distr      *emocks.DistrKeeper
	Authz      *emocks.AuthzKeeper
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

	// cdc := codec.NewProtoCodec(cdctypes.NewInterfaceRegistry())

	if keepers.Bank == nil {
		bkeeper := &emocks.BankKeeper{}
		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		keepers.Bank = bkeeper
	}

	if keepers.Distr == nil {
		dkeeper := &emocks.DistrKeeper{}
		dkeeper.
			On("GetFeePool", mock.Anything).
			Return(distrtypes.FeePool{})
		dkeeper.On("SetFeePool", mock.Anything, mock.Anything).
			Return()

		keepers.Distr = dkeeper
	}

	if keepers.Authz == nil {
		keeper := &emocks.AuthzKeeper{}

		keepers.Authz = keeper
	}

	app := app.Setup(
		app.WithCheckTx(false),
		app.WithHome(dir),
		app.WithGenesis(app.GenesisStateWithValSet),
	)

	ctx := app.BaseApp.NewContext(false, tmproto.Header{})

	cdc := app.AppCodec()

	// Manually set validator signing info, otherwise we panic
	for _, val := range app.Keepers.Cosmos.Staking.GetAllValidators(ctx) {
		consAddr, _ := val.GetConsAddr()
		signingInfo := slashingtypes.NewValidatorSigningInfo(
			consAddr,
			0,
			ctx.BlockHeight(),
			time.Unix(0, 0),
			false,
			0,
		)
		app.Keepers.Cosmos.Slashing.SetValidatorSigningInfo(ctx, consAddr, signingInfo)
	}

	if keepers.Audit == nil {
		keepers.Audit = akeeper.NewKeeper(cdc, app.GetKey(atypes.StoreKey))
	}

	if keepers.Take == nil {
		keepers.Take = tkeeper.NewKeeper(cdc, app.GetKey(ttypes.StoreKey), authtypes.NewModuleAddress(govtypes.ModuleName).String())
	}

	if keepers.Escrow == nil {
		keepers.Escrow = ekeeper.NewKeeper(cdc, app.GetKey(etypes.StoreKey), keepers.Bank, keepers.Take, keepers.Distr, keepers.Authz)
	}
	if keepers.Market == nil {
		keepers.Market = mkeeper.NewKeeper(cdc, app.GetKey(mtypes.StoreKey), keepers.Escrow, authtypes.NewModuleAddress(govtypes.ModuleName).String())
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

	return &TestSuite{
		t:       t,
		app:     app,
		ctx:     ctx,
		keepers: keepers,
	}
}

func (ts *TestSuite) App() *app.AkashApp {
	return ts.app
}

// SetBlockHeight provides arbitrarily setting the chain's block height.
func (ts *TestSuite) SetBlockHeight(height int64) {
	ts.ctx = ts.ctx.WithBlockHeight(height)
}

// Store provides access to the underlying KVStore
func (ts *TestSuite) Store() sdk.CommitMultiStore {
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
