package app

import (
	simparams "github.com/cosmos/cosmos-sdk/simapp/params"
	"github.com/cosmos/cosmos-sdk/std"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
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
	"github.com/cosmos/cosmos-sdk/x/ibc"
	ibctransfertypes "github.com/cosmos/cosmos-sdk/x/ibc-transfer/types"
	ibchost "github.com/cosmos/cosmos-sdk/x/ibc/24-host"
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
			genutil.AppModuleBasic{},

			// accounts, fees.
			auth.AppModuleBasic{},

			// tokens, token balance.
			bank.AppModuleBasic{},

			capability.AppModuleBasic{},

			// inflation
			mint.AppModuleBasic{},

			staking.AppModuleBasic{},

			slashing.AppModuleBasic{},

			distr.AppModuleBasic{},

			gov.NewAppModuleBasic(
				paramsclient.ProposalHandler, distrclient.ProposalHandler,
				upgradeclient.ProposalHandler, upgradeclient.CancelProposalHandler,
			),

			params.AppModuleBasic{},
			ibc.AppModuleBasic{},
			upgrade.AppModuleBasic{},
			evidence.AppModuleBasic{},
			crisis.AppModuleBasic{},
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
	std.RegisterCodec(encodingConfig.Amino)
	std.RegisterInterfaces(encodingConfig.InterfaceRegistry)
	mbasics.RegisterCodec(encodingConfig.Amino)
	mbasics.RegisterInterfaces(encodingConfig.InterfaceRegistry)
	return encodingConfig
}

func kvStoreKeys() map[string]*sdk.KVStoreKey {
	return sdk.NewKVStoreKeys(
		append([]string{
			authtypes.StoreKey,
			banktypes.StoreKey,
			paramstypes.StoreKey,
			slashingtypes.StoreKey,
			distrtypes.StoreKey,
			stakingtypes.StoreKey,
			minttypes.StoreKey,
			govtypes.StoreKey,
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
