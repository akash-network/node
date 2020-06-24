// +build mainnet

package app

import (
	bam "github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	"github.com/cosmos/cosmos-sdk/x/evidence"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	"github.com/cosmos/cosmos-sdk/x/gov"
	"github.com/cosmos/cosmos-sdk/x/mint"
	"github.com/cosmos/cosmos-sdk/x/params"
	paramsclient "github.com/cosmos/cosmos-sdk/x/params/client"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	"github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/cosmos/cosmos-sdk/x/supply"
	"github.com/cosmos/cosmos-sdk/x/upgrade"
	upgradeclient "github.com/cosmos/cosmos-sdk/x/upgrade/client"
)

var (
	mbasics = module.NewBasicManager(
		genutil.AppModuleBasic{},

		// accounts, fees.
		auth.AppModuleBasic{},

		// tokens, token balance.
		bank.AppModuleBasic{},

		// total supply of the chain
		supply.AppModuleBasic{},

		// inflation
		mint.AppModuleBasic{},

		staking.AppModuleBasic{},

		slashing.AppModuleBasic{},

		distr.AppModuleBasic{},

		gov.NewAppModuleBasic(
			paramsclient.ProposalHandler, distr.ProposalHandler, upgradeclient.ProposalHandler,
		),

		params.AppModuleBasic{},
		upgrade.AppModuleBasic{},
		evidence.AppModuleBasic{},
		crisis.AppModuleBasic{},
	)
)

func kvStoreKeys() map[string]*sdk.KVStoreKey {
	return sdk.NewKVStoreKeys(
		bam.MainStoreKey,
		auth.StoreKey,
		params.StoreKey,
		slashing.StoreKey,
		distr.StoreKey,
		supply.StoreKey,
		staking.StoreKey,
		mint.StoreKey,
		gov.StoreKey,
		upgrade.StoreKey,
		evidence.StoreKey,
	)
}

func (app *AkashApp) setAkashKeepers() {
}

func (app *AkashApp) setModuleManager() {
	app.mm = module.NewManager(
		genutil.NewAppModule(app.keeper.acct, app.keeper.staking, app.BaseApp.DeliverTx),
		auth.NewAppModule(app.keeper.acct),
		bank.NewAppModule(app.keeper.bank, app.keeper.acct),

		supply.NewAppModule(app.keeper.supply, app.keeper.acct),
		distr.NewAppModule(app.keeper.distr, app.keeper.acct, app.keeper.supply, app.keeper.staking),

		mint.NewAppModule(app.keeper.mint),
		slashing.NewAppModule(app.keeper.slashing, app.keeper.acct, app.keeper.staking),

		staking.NewAppModule(app.keeper.staking, app.keeper.acct, app.keeper.supply),

		gov.NewAppModule(app.keeper.gov, app.keeper.acct, app.keeper.supply),
		upgrade.NewAppModule(app.keeper.upgrade),
		evidence.NewAppModule(app.keeper.evidence),
		crisis.NewAppModule(&app.keeper.crisis),
	)

	app.mm.SetOrderBeginBlockers(upgrade.ModuleName, mint.ModuleName, distr.ModuleName, slashing.ModuleName, evidence.ModuleName)
	app.mm.SetOrderEndBlockers(crisis.ModuleName, gov.ModuleName, staking.ModuleName)

	// NOTE: The genutils module must occur after staking so that pools are
	//       properly initialized with tokens from genesis accounts.
	app.mm.SetOrderInitGenesis(
		auth.ModuleName,
		distr.ModuleName,
		staking.ModuleName,
		bank.ModuleName,
		slashing.ModuleName,
		gov.ModuleName,
		mint.ModuleName,
		supply.ModuleName,
		crisis.ModuleName,
		genutil.ModuleName,
		evidence.ModuleName,
	)
}

func (app *AkashApp) setSimulationManager() {
	app.sm = module.NewSimulationManager(
		auth.NewAppModule(app.keeper.acct),
		bank.NewAppModule(app.keeper.bank, app.keeper.acct),
		supply.NewAppModule(app.keeper.supply, app.keeper.acct),
		mint.NewAppModule(app.keeper.mint),
		staking.NewAppModule(app.keeper.staking, app.keeper.acct, app.keeper.supply),
		distr.NewAppModule(app.keeper.distr, app.keeper.acct, app.keeper.supply, app.keeper.staking),
		slashing.NewAppModule(app.keeper.slashing, app.keeper.acct, app.keeper.staking),
		params.NewAppModule(), // NOTE: only used for simulation to generate randomized param change proposals
	)
}
