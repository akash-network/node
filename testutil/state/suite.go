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
	akeeper akeeper.Keeper
	ekeeper ekeeper.Keeper
	mkeeper mkeeper.Keeper
	dkeeper dkeeper.Keeper
	pkeeper pkeeper.Keeper
	bkeeper *emocks.BankKeeper
}

// SetupTestSuite provides toolkit for accessing stores and keepers
// for complex data interactions.
func SetupTestSuite(t testing.TB) *TestSuite {
	suite := &TestSuite{
		t: t,
	}

	bkeeper := &emocks.BankKeeper{}
	bkeeper.
		On("SendCoinsFromAccountToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
	bkeeper.
		On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	app := app.Setup(false)
	suite.app = app
	suite.ctx = app.BaseApp.NewContext(false, tmproto.Header{})

	suite.bkeeper = bkeeper

	suite.akeeper = akeeper.NewKeeper(atypes.ModuleCdc, app.GetKey(atypes.StoreKey))
	suite.ekeeper = ekeeper.NewKeeper(etypes.ModuleCdc, app.GetKey(etypes.StoreKey), suite.bkeeper)
	suite.mkeeper = mkeeper.NewKeeper(mtypes.ModuleCdc, app.GetKey(mtypes.StoreKey), app.GetSubspace(mtypes.ModuleName), suite.ekeeper)
	suite.dkeeper = dkeeper.NewKeeper(dtypes.ModuleCdc, app.GetKey(dtypes.StoreKey), app.GetSubspace(dtypes.ModuleName))
	suite.pkeeper = pkeeper.NewKeeper(ptypes.ModuleCdc, app.GetKey(ptypes.StoreKey))

	suite.ekeeper.AddOnAccountClosedHook(suite.dkeeper.OnEscrowAccountClosed)
	suite.ekeeper.AddOnAccountClosedHook(suite.mkeeper.OnEscrowAccountClosed)
	suite.ekeeper.AddOnPaymentClosedHook(suite.dkeeper.OnEscrowPaymentClosed)
	suite.ekeeper.AddOnPaymentClosedHook(suite.mkeeper.OnEscrowPaymentClosed)

	return suite
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
func (ts *TestSuite) AuditKeeper() akeeper.Keeper {
	return ts.akeeper
}

// EscrowKeeper key store
func (ts *TestSuite) EscrowKeeper() ekeeper.Keeper {
	return ts.ekeeper
}

// MarketKeeper key store
func (ts *TestSuite) MarketKeeper() mkeeper.Keeper {
	return ts.mkeeper
}

// DeploymentKeeper key store
func (ts *TestSuite) DeploymentKeeper() dkeeper.Keeper {
	return ts.dkeeper
}

// ProviderKeeper key store
func (ts *TestSuite) ProviderKeeper() pkeeper.Keeper {
	return ts.pkeeper
}

// BankKeeper key store
func (ts *TestSuite) BankKeeper() *emocks.BankKeeper {
	return ts.bkeeper
}
