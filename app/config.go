package app

import (
	simparams "github.com/cosmos/cosmos-sdk/simapp/params"
	"github.com/cosmos/cosmos-sdk/std"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	"github.com/cosmos/cosmos-sdk/x/bank"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/capability"
	capabilitytypes "github.com/cosmos/cosmos-sdk/x/capability/types"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	distrclient "github.com/cosmos/cosmos-sdk/x/distribution/client"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/cosmos-sdk/x/evidence"
	evidencetypes "github.com/cosmos/cosmos-sdk/x/evidence/types"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	"github.com/cosmos/cosmos-sdk/x/gov"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	transfer "github.com/cosmos/cosmos-sdk/x/ibc/applications/transfer"
	ibctransfertypes "github.com/cosmos/cosmos-sdk/x/ibc/applications/transfer/types"
	ibc "github.com/cosmos/cosmos-sdk/x/ibc/core"
	ibchost "github.com/cosmos/cosmos-sdk/x/ibc/core/24-host"
	"github.com/cosmos/cosmos-sdk/x/mint"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/cosmos/cosmos-sdk/x/params"
	paramsclient "github.com/cosmos/cosmos-sdk/x/params/client"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/cosmos/cosmos-sdk/x/upgrade"
	upgradeclient "github.com/cosmos/cosmos-sdk/x/upgrade/client"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	appparams "github.com/ovrclk/akash/app/params"
)

var (
	mbasics = module.NewBasicManager(
		append([]module.AppModuleBasic{
			// accounts, fees.
			auth.AppModuleBasic{},
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
				paramsclient.ProposalHandler, distrclient.ProposalHandler,
				upgradeclient.ProposalHandler, upgradeclient.CancelProposalHandler,
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
func MakeEncodingConfig() simparams.EncodingConfig {
	encodingConfig := appparams.MakeEncodingConfig()
	std.RegisterLegacyAminoCodec(encodingConfig.Amino)
	std.RegisterInterfaces(encodingConfig.InterfaceRegistry)
	mbasics.RegisterLegacyAminoCodec(encodingConfig.Amino)
	mbasics.RegisterInterfaces(encodingConfig.InterfaceRegistry)
	return encodingConfig
}

func kvStoreKeys() map[string]*sdk.KVStoreKey {
	return sdk.NewKVStoreKeys(
		append([]string{
			authtypes.StoreKey,
			banktypes.StoreKey,
			stakingtypes.StoreKey,
			minttypes.StoreKey,
			distrtypes.StoreKey,
			slashingtypes.StoreKey,
			govtypes.StoreKey,
			paramstypes.StoreKey,
			ibchost.StoreKey,
			upgradetypes.StoreKey,
			evidencetypes.StoreKey,
			ibctransfertypes.StoreKey,
			capabilitytypes.StoreKey,
		},
			akashKVStoreKeys()...,
		)...,
	)
}

func transientStoreKeys() map[string]*sdk.TransientStoreKey {
	return sdk.NewTransientStoreKeys(paramstypes.TStoreKey)
}

func memStoreKeys() map[string]*sdk.MemoryStoreKey {
	return sdk.NewMemoryStoreKeys(capabilitytypes.MemStoreKey)

}
