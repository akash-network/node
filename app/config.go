package app

import (
	"github.com/cosmos/cosmos-sdk/std"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	authzmodule "github.com/cosmos/cosmos-sdk/x/authz/module"
	"github.com/cosmos/cosmos-sdk/x/bank"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/capability"
	capabilitytypes "github.com/cosmos/cosmos-sdk/x/capability/types"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/cosmos-sdk/x/evidence"
	evidencetypes "github.com/cosmos/cosmos-sdk/x/evidence/types"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	feegrantmodule "github.com/cosmos/cosmos-sdk/x/feegrant/module"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	"github.com/cosmos/cosmos-sdk/x/gov"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/cosmos/cosmos-sdk/x/mint"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/cosmos/cosmos-sdk/x/params"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/cosmos/cosmos-sdk/x/upgrade"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	"github.com/cosmos/ibc-go/v7/modules/apps/transfer"
	ibctransfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	ibc "github.com/cosmos/ibc-go/v7/modules/core"
	ibcexported "github.com/cosmos/ibc-go/v7/modules/core/exported"

	appparams "github.com/akash-network/node/app/params"
)

var (
	mbasics = module.NewBasicManager(
		append([]module.AppModuleBasic{
			auth.AppModuleBasic{},
			genutil.AppModuleBasic{},
			bank.AppModule{},
			capability.AppModuleBasic{},
			staking.AppModuleBasic{},
			mint.AppModuleBasic{},
			distr.AppModuleBasic{},
			gov.NewAppModuleBasic(getGovProposalHandlers()),

			params.AppModuleBasic{},
			crisis.AppModuleBasic{},
			slashing.AppModuleBasic{},
			feegrantmodule.AppModuleBasic{},
			authzmodule.AppModuleBasic{},
			ibc.AppModuleBasic{},
			upgrade.AppModuleBasic{},
			evidence.AppModuleBasic{},
			transfer.AppModuleBasic{},
			vesting.AppModuleBasic{},
			// chain parameters
			feegrantmodule.AppModuleBasic{},
		},
			// akash
			akashModuleBasics()...,
		)...,
	)
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
	mbasics.RegisterLegacyAminoCodec(encodingConfig.Amino)
	mbasics.RegisterInterfaces(encodingConfig.InterfaceRegistry)

	return encodingConfig
}

func kvStoreKeys() map[string]*storetypes.KVStoreKey {
	return sdk.NewKVStoreKeys(
		append([]string{
			authtypes.StoreKey,
			feegrant.StoreKey,
			authzkeeper.StoreKey,
			banktypes.StoreKey,
			stakingtypes.StoreKey,
			minttypes.StoreKey,
			distrtypes.StoreKey,
			slashingtypes.StoreKey,
			govtypes.StoreKey,
			paramstypes.StoreKey,
			ibcexported.StoreKey,
			upgradetypes.StoreKey,
			evidencetypes.StoreKey,
			ibctransfertypes.StoreKey,
			capabilitytypes.StoreKey,
		},
			akashKVStoreKeys()...,
		)...,
	)
}

func transientStoreKeys() map[string]*storetypes.TransientStoreKey {
	return sdk.NewTransientStoreKeys(paramstypes.TStoreKey)
}

func memStoreKeys() map[string]*storetypes.MemoryStoreKey {
	return sdk.NewMemoryStoreKeys(capabilitytypes.MemStoreKey)

}
