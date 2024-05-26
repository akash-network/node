package app

import (
	"github.com/cosmos/cosmos-sdk/std"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	authzmodule "github.com/cosmos/cosmos-sdk/x/authz/module"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/capability"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	"github.com/cosmos/cosmos-sdk/x/evidence"
	feegrantmodule "github.com/cosmos/cosmos-sdk/x/feegrant/module"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	"github.com/cosmos/cosmos-sdk/x/gov"
	govclient "github.com/cosmos/cosmos-sdk/x/gov/client"
	"github.com/cosmos/cosmos-sdk/x/mint"
	"github.com/cosmos/cosmos-sdk/x/params"
	paramsclient "github.com/cosmos/cosmos-sdk/x/params/client"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	"github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/cosmos/cosmos-sdk/x/upgrade"
	upgradeclient "github.com/cosmos/cosmos-sdk/x/upgrade/client"
	"github.com/cosmos/ibc-go/v7/modules/apps/transfer"
	ibc "github.com/cosmos/ibc-go/v7/modules/core"
	ibcclient "github.com/cosmos/ibc-go/v7/modules/core/02-client/client"

	appparams "pkg.akt.dev/akashd/app/params"
)

var mbasics = module.NewBasicManager(
	append([]module.AppModuleBasic{
		// accounts, fees.
		auth.AppModuleBasic{},
		// authorizations
		authzmodule.AppModuleBasic{},
		// genesis utilities
		genutil.AppModuleBasic{},
		// tokens, token balance.
		bank.AppModuleBasic{},
		capability.AppModuleBasic{},
		// validator staking
		staking.AppModuleBasic{},
		// inflation
		mint.AppModuleBasic{},
		// distribution of fess and inflation
		distr.AppModuleBasic{},
		// governance functionality (voting)
		gov.NewAppModuleBasic(
			[]govclient.ProposalHandler{
				paramsclient.ProposalHandler,
				// distrclient.ProposalHandler,
				upgradeclient.LegacyProposalHandler,
				upgradeclient.LegacyCancelProposalHandler,
				ibcclient.UpdateClientProposalHandler,
				ibcclient.UpgradeProposalHandler,
			},
		),
		// chain parameters
		params.AppModuleBasic{},
		crisis.AppModuleBasic{},
		slashing.AppModuleBasic{},
		ibc.AppModuleBasic{},
		upgrade.AppModuleBasic{},
		evidence.AppModuleBasic{},
		transfer.AppModuleBasic{},
		vesting.AppModuleBasic{},
		feegrantmodule.AppModuleBasic{},
	},
		// akash
		akashModuleBasics()...,
	)...,
)

// ModuleBasics returns all app modules basics
func ModuleBasics() module.BasicManager {
	return mbasics
}

// MakeEncodingConfig creates an EncodingConfig for testing
func MakeEncodingConfig() appparams.EncodingConfig {
	encodingConfig := appparams.MakeEncodingConfig()

	std.RegisterLegacyAminoCodec(encodingConfig.Amino)
	std.RegisterInterfaces(encodingConfig.InterfaceRegistry)

	ModuleBasics().RegisterLegacyAminoCodec(encodingConfig.Amino)
	ModuleBasics().RegisterInterfaces(encodingConfig.InterfaceRegistry)

	return encodingConfig
}

// func kvStoreKeys() map[string]*storetypes.KVStoreKey {
// 	return sdk.NewKVStoreKeys(
// 		append([]string{
// 			authtypes.StoreKey,
// 			feegrant.StoreKey,
// 			authzkeeper.StoreKey,
// 			banktypes.StoreKey,
// 			stakingtypes.StoreKey,
// 			minttypes.StoreKey,
// 			distrtypes.StoreKey,
// 			slashingtypes.StoreKey,
// 			govtypes.StoreKey,
// 			paramstypes.StoreKey,
// 			ibchost.StoreKey,
// 			upgradetypes.StoreKey,
// 			evidencetypes.StoreKey,
// 			ibctransfertypes.StoreKey,
// 			capabilitytypes.StoreKey,
// 		},
// 			akashKVStoreKeys()...,
// 		)...,
// 	)
// }
//
// func transientStoreKeys() map[string]*storetypes.TransientStoreKey {
// 	return sdk.NewTransientStoreKeys(paramstypes.TStoreKey)
// }
//
// func memStoreKeys() map[string]*storetypes.MemoryStoreKey {
// 	return sdk.NewMemoryStoreKeys(capabilitytypes.MemStoreKey)
// }
