package app

import (
	simparams "github.com/cosmos/cosmos-sdk/simapp/params"
	"github.com/cosmos/cosmos-sdk/std"
	"github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	"github.com/cosmos/cosmos-sdk/x/authz"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	authzmodule "github.com/cosmos/cosmos-sdk/x/authz/module"
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
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	feegrantmodule "github.com/cosmos/cosmos-sdk/x/feegrant/module"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	"github.com/cosmos/cosmos-sdk/x/gov"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
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
	"github.com/cosmos/ibc-go/v4/modules/apps/transfer"
	ibctransfertypes "github.com/cosmos/ibc-go/v4/modules/apps/transfer/types"
	ibc "github.com/cosmos/ibc-go/v4/modules/core"
	ibcclient "github.com/cosmos/ibc-go/v4/modules/core/02-client/client"
	ibchost "github.com/cosmos/ibc-go/v4/modules/core/24-host"

	appparams "github.com/akash-network/node/app/params"
	"github.com/akash-network/node/x/audit"
	"github.com/akash-network/node/x/cert"
	"github.com/akash-network/node/x/deployment"
	"github.com/akash-network/node/x/escrow"
	agov "github.com/akash-network/node/x/gov"
	"github.com/akash-network/node/x/inflation"
	"github.com/akash-network/node/x/market"
	"github.com/akash-network/node/x/provider"
	astaking "github.com/akash-network/node/x/staking"
	"github.com/akash-network/node/x/take"
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
			paramsclient.ProposalHandler, distrclient.ProposalHandler,
			upgradeclient.ProposalHandler, upgradeclient.CancelProposalHandler,
			ibcclient.UpdateClientProposalHandler, ibcclient.UpgradeProposalHandler,
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
func MakeEncodingConfig() simparams.EncodingConfig {
	encodingConfig := appparams.MakeEncodingConfig()

	std.RegisterLegacyAminoCodec(encodingConfig.Amino)
	std.RegisterInterfaces(encodingConfig.InterfaceRegistry)
	mbasics.RegisterLegacyAminoCodec(encodingConfig.Amino)
	mbasics.RegisterInterfaces(encodingConfig.InterfaceRegistry)

	return encodingConfig
}

func (m ModulesStoreKeys) Keys() map[string]*sdk.KVStoreKey {
	res := make(map[string]*sdk.KVStoreKey)

	for _, key := range m {
		res[key.Name()] = key
	}

	return res
}

func modulesStoreKeys() ModulesStoreKeys {
	return ModulesStoreKeys{
		authtypes.ModuleName:        types.NewKVStoreKey(authtypes.StoreKey),
		feegrant.ModuleName:         types.NewKVStoreKey(feegrant.StoreKey),
		authz.ModuleName:            types.NewKVStoreKey(authzkeeper.StoreKey),
		banktypes.ModuleName:        types.NewKVStoreKey(banktypes.StoreKey),
		stakingtypes.ModuleName:     types.NewKVStoreKey(stakingtypes.StoreKey),
		minttypes.ModuleName:        types.NewKVStoreKey(minttypes.StoreKey),
		distrtypes.ModuleName:       types.NewKVStoreKey(distrtypes.StoreKey),
		slashingtypes.ModuleName:    types.NewKVStoreKey(slashingtypes.StoreKey),
		govtypes.ModuleName:         types.NewKVStoreKey(govtypes.StoreKey),
		paramstypes.ModuleName:      types.NewKVStoreKey(paramstypes.StoreKey),
		ibchost.ModuleName:          types.NewKVStoreKey(ibchost.StoreKey),
		upgradetypes.ModuleName:     types.NewKVStoreKey(upgradetypes.StoreKey),
		evidencetypes.ModuleName:    types.NewKVStoreKey(evidencetypes.StoreKey),
		ibctransfertypes.ModuleName: types.NewKVStoreKey(ibctransfertypes.StoreKey),
		capabilitytypes.ModuleName:  types.NewKVStoreKey(capabilitytypes.StoreKey),
		// akash modules
		take.ModuleName:       types.NewKVStoreKey(take.StoreKey),
		escrow.ModuleName:     types.NewKVStoreKey(escrow.StoreKey),
		deployment.ModuleName: types.NewKVStoreKey(deployment.StoreKey),
		market.ModuleName:     types.NewKVStoreKey(market.StoreKey),
		provider.ModuleName:   types.NewKVStoreKey(provider.StoreKey),
		audit.ModuleName:      types.NewKVStoreKey(audit.StoreKey),
		cert.ModuleName:       types.NewKVStoreKey(cert.StoreKey),
		inflation.ModuleName:  types.NewKVStoreKey(inflation.StoreKey),
		astaking.ModuleName:   types.NewKVStoreKey(astaking.StoreKey),
		agov.ModuleName:       types.NewKVStoreKey(agov.StoreKey),
	}
}

func modulesTransientKeys() ModulesTransientKeys {
	return ModulesTransientKeys{
		paramstypes.ModuleName: sdk.NewTransientStoreKey(paramstypes.TStoreKey),
	}
}

func modulesMemoryKeys() ModulesMemoryKeys {
	return ModulesMemoryKeys{
		capabilitytypes.ModuleName: types.NewMemoryStoreKey(capabilitytypes.MemStoreKey),
	}
}
