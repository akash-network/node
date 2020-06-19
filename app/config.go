package app

import (
	bam "github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
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
	// AKASH-REMOVAL
	// "github.com/ovrclk/akash/x/deployment"
	// "github.com/ovrclk/akash/x/market"
	// "github.com/ovrclk/akash/x/provider"
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

		// AKASH-REMOVAL
		// deployment.AppModuleBasic{},
		// market.AppModuleBasic{},
		// provider.AppModuleBasic{},
	)
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
		// AKASH-REMOVAL
		// deployment.StoreKey,
		// market.StoreKey,
		// provider.StoreKey,
	)
}

func transientStoreKeys() map[string]*sdk.TransientStoreKey {
	return sdk.NewTransientStoreKeys(params.TStoreKey)
}
