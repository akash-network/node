package state

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/mock"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"

	"github.com/ovrclk/akash/app"
	akeeper "github.com/ovrclk/akash/x/audit/keeper"
	atypes "github.com/ovrclk/akash/x/audit/types"
	dkeeper "github.com/ovrclk/akash/x/deployment/keeper"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	ekeeper "github.com/ovrclk/akash/x/escrow/keeper"
	emocks "github.com/ovrclk/akash/x/escrow/keeper/mocks"
	etypes "github.com/ovrclk/akash/x/escrow/types"
	mhooks "github.com/ovrclk/akash/x/market/hooks"
	mkeeper "github.com/ovrclk/akash/x/market/keeper"
	mtypes "github.com/ovrclk/akash/x/market/types"
	pkeeper "github.com/ovrclk/akash/x/provider/keeper"
	ptypes "github.com/ovrclk/akash/x/provider/types"
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
	Escrow     ekeeper.Keeper
	Audit      akeeper.IKeeper
	Market     mkeeper.IKeeper
	Deployment dkeeper.IKeeper
	Provider   pkeeper.IKeeper
	Bank       *emocks.BankKeeper
}

// SetupTestSuite provides toolkit for accessing stores and keepers
// for complex data interactions.
func SetupTestSuite(t testing.TB) *TestSuite {
	return SetupTestSuiteWithKeepers(t, Keepers{})
}

func SetupTestSuiteWithKeepers(t testing.TB, keepers Keepers) *TestSuite {
	if keepers.Bank == nil {
		bkeeper := &emocks.BankKeeper{}
		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		keepers.Bank = bkeeper
	}

	app := app.Setup(false)

	if keepers.Audit == nil {
		keepers.Audit = akeeper.NewKeeper(atypes.ModuleCdc, app.GetKey(atypes.StoreKey))
	}
	if keepers.Escrow == nil {
		keepers.Escrow = ekeeper.NewKeeper(etypes.ModuleCdc, app.GetKey(etypes.StoreKey), keepers.Bank)
	}
	if keepers.Market == nil {
		keepers.Market = mkeeper.NewKeeper(mtypes.ModuleCdc, app.GetKey(mtypes.StoreKey), app.GetSubspace(mtypes.ModuleName), keepers.Escrow)
	}
	if keepers.Deployment == nil {
		keepers.Deployment = dkeeper.NewKeeper(dtypes.ModuleCdc, app.GetKey(dtypes.StoreKey), app.GetSubspace(dtypes.ModuleName), keepers.Escrow)
	}
	if keepers.Provider == nil {
		keepers.Provider = pkeeper.NewKeeper(ptypes.ModuleCdc, app.GetKey(ptypes.StoreKey))
	}

	hook := mhooks.New(keepers.Deployment, keepers.Market)

	keepers.Escrow.AddOnAccountClosedHook(hook.OnEscrowAccountClosed)
	keepers.Escrow.AddOnPaymentClosedHook(hook.OnEscrowPaymentClosed)

	return &TestSuite{
		t:       t,
		app:     app,
		ctx:     app.BaseApp.NewContext(false, tmproto.Header{}),
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
