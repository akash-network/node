package app

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	"github.com/cosmos/cosmos-sdk/x/evidence"
	"github.com/cosmos/cosmos-sdk/x/gov"
	"github.com/cosmos/cosmos-sdk/x/mint"
	"github.com/cosmos/cosmos-sdk/x/params"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	"github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/cosmos/cosmos-sdk/x/supply"
	"github.com/cosmos/cosmos-sdk/x/upgrade"
	"github.com/ovrclk/akash/x/deployment"
	"github.com/ovrclk/akash/x/market"
	"github.com/ovrclk/akash/x/provider"
)

// ModuleBasics returns all app modules basics
func ModuleBasics() module.BasicManager {
	return mbasics
}

// MakeCodec returns registered codecs
func MakeCodec() *codec.Codec {
	var cdc = codec.New()

	mbasics.RegisterCodec(cdc)

	sdk.RegisterCodec(cdc)
	vesting.RegisterCodec(cdc)
	codec.RegisterCrypto(cdc)
	codec.RegisterEvidences(cdc)

	return cdc.Seal()
}

func transientStoreKeys() map[string]*sdk.TransientStoreKey {
	return sdk.NewTransientStoreKeys(params.TStoreKey)
}

func (app *AkashApp) setSDKKeepers(skipUpgradeHeights map[int64]bool) {
	app.keeper.params = params.NewKeeper(
		app.cdc,
		app.keys[params.StoreKey],
		app.tkeys[params.TStoreKey],
	)

	app.keeper.acct = auth.NewAccountKeeper(
		app.cdc,
		app.keys[auth.StoreKey],
		app.keeper.params.Subspace(auth.DefaultParamspace),
		auth.ProtoBaseAccount,
	)

	app.keeper.bank = bank.NewBaseKeeper(
		app.keeper.acct,
		app.keeper.params.Subspace(bank.DefaultParamspace),
		macAddrs(),
	)

	app.keeper.supply = supply.NewKeeper(
		app.cdc,
		app.keys[supply.StoreKey],
		app.keeper.acct,
		app.keeper.bank,
		macPerms(),
	)

	skeeper := staking.NewKeeper(
		app.cdc,
		app.keys[staking.StoreKey],
		app.keeper.supply,
		app.keeper.params.Subspace(staking.DefaultParamspace),
	)

	app.keeper.distr = distr.NewKeeper(
		app.cdc,
		app.keys[distr.StoreKey],
		app.keeper.params.Subspace(distr.DefaultParamspace),
		&skeeper,
		app.keeper.supply,
		auth.FeeCollectorName,
		macAddrs(),
	)

	app.keeper.slashing = slashing.NewKeeper(
		app.cdc,
		app.keys[slashing.StoreKey],
		&skeeper,
		app.keeper.params.Subspace(slashing.DefaultParamspace),
	)

	app.keeper.staking = *skeeper.SetHooks(
		staking.NewMultiStakingHooks(
			app.keeper.distr.Hooks(),
			app.keeper.slashing.Hooks(),
		),
	)

	app.keeper.mint = mint.NewKeeper(
		app.cdc,
		app.keys[mint.StoreKey],
		app.keeper.params.Subspace(mint.DefaultParamspace),
		&skeeper,
		app.keeper.supply,
		auth.FeeCollectorName,
	)

	app.keeper.upgrade = upgrade.NewKeeper(skipUpgradeHeights, app.keys[upgrade.StoreKey], app.cdc)

	app.keeper.crisis = crisis.NewKeeper(
		app.keeper.params.Subspace(crisis.DefaultParamspace),
		app.invCheckPeriod,
		app.keeper.supply,
		auth.FeeCollectorName,
	)

	// create evidence keeper with evidence router
	evidenceKeeper := evidence.NewKeeper(
		app.cdc, app.keys[evidence.StoreKey],
		app.keeper.params.Subspace(evidence.DefaultParamspace),
		&app.keeper.staking,
		app.keeper.slashing,
	)
	evidenceRouter := evidence.NewRouter()

	// TODO: register evidence routes
	evidenceKeeper.SetRouter(evidenceRouter)

	app.keeper.evidence = *evidenceKeeper

	// register the proposal types
	govRouter := gov.NewRouter()
	govRouter.AddRoute(gov.RouterKey, gov.ProposalHandler).
		AddRoute(params.RouterKey, params.NewParamChangeProposalHandler(app.keeper.params)).
		AddRoute(distr.RouterKey, distr.NewCommunityPoolSpendProposalHandler(app.keeper.distr)).
		AddRoute(upgrade.RouterKey, upgrade.NewSoftwareUpgradeProposalHandler(app.keeper.upgrade))

	app.keeper.gov = gov.NewKeeper(
		app.cdc,
		app.keys[gov.StoreKey],
		app.keeper.params.Subspace(gov.DefaultParamspace).WithKeyTable(gov.ParamKeyTable()),
		app.keeper.supply,
		&skeeper,
		govRouter,
	)
}

func (app *AkashApp) setAkashKeepers() {
	app.keeper.deployment = deployment.NewKeeper(
		app.cdc,
		app.keys[deployment.StoreKey],
	)

	app.keeper.market = market.NewKeeper(
		app.cdc,
		app.keys[market.StoreKey],
	)

	app.keeper.provider = provider.NewKeeper(
		app.cdc,
		app.keys[provider.StoreKey],
	)
}
