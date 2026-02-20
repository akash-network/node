package app

import (
	"cosmossdk.io/x/evidence"
	feegrantmodule "cosmossdk.io/x/feegrant/module"
	"cosmossdk.io/x/upgrade"
	"github.com/CosmWasm/wasmd/x/wasm"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	addresscodec "github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authsims "github.com/cosmos/cosmos-sdk/x/auth/simulation"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	authzmodule "github.com/cosmos/cosmos-sdk/x/authz/module"
	"github.com/cosmos/cosmos-sdk/x/bank"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/consensus"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	"github.com/cosmos/cosmos-sdk/x/gov"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/cosmos/cosmos-sdk/x/mint"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/cosmos/cosmos-sdk/x/params"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/cosmos/ibc-go/v10/modules/apps/transfer"
	ibc "github.com/cosmos/ibc-go/v10/modules/core"
	ibctm "github.com/cosmos/ibc-go/v10/modules/light-clients/07-tendermint"

	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/x/audit"
	"pkg.akt.dev/node/v2/x/bme"
	"pkg.akt.dev/node/v2/x/cert"
	"pkg.akt.dev/node/v2/x/deployment"
	"pkg.akt.dev/node/v2/x/epochs"
	"pkg.akt.dev/node/v2/x/escrow"
	"pkg.akt.dev/node/v2/x/market"
	"pkg.akt.dev/node/v2/x/oracle"
	"pkg.akt.dev/node/v2/x/provider"
	awasm "pkg.akt.dev/node/v2/x/wasm"
)

func appModules(
	app *AkashApp,
	encodingConfig sdkutil.EncodingConfig,
) []module.AppModule {
	cdc := encodingConfig.Codec

	return []module.AppModule{
		genutil.NewAppModule(
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Staking,
			app.BaseApp,
			encodingConfig.TxConfig,
		),
		auth.NewAppModule(
			cdc,
			app.Keepers.Cosmos.Acct,
			nil,
			app.GetSubspace(authtypes.ModuleName),
		),
		vesting.NewAppModule(
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
		),
		bank.NewAppModule(
			cdc,
			app.Keepers.Cosmos.Bank,
			app.Keepers.Cosmos.Acct,
			app.GetSubspace(banktypes.ModuleName),
		),
		gov.NewAppModule(
			cdc,
			app.Keepers.Cosmos.Gov,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.GetSubspace(govtypes.ModuleName),
		),
		mint.NewAppModule(
			cdc,
			app.Keepers.Cosmos.Mint,
			app.Keepers.Cosmos.Acct,
			nil, // todo akash-network/support#4
			app.GetSubspace(minttypes.ModuleName),
		),
		slashing.NewAppModule(
			cdc,
			app.Keepers.Cosmos.Slashing,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.Keepers.Cosmos.Staking,
			app.GetSubspace(slashingtypes.ModuleName),
			encodingConfig.InterfaceRegistry,
		),
		distr.NewAppModule(
			cdc,
			app.Keepers.Cosmos.Distr,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.Keepers.Cosmos.Staking,
			app.GetSubspace(distrtypes.ModuleName),
		),
		staking.NewAppModule(
			cdc,
			app.Keepers.Cosmos.Staking,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.GetSubspace(stakingtypes.ModuleName),
		),
		upgrade.NewAppModule(
			app.Keepers.Cosmos.Upgrade,
			addresscodec.NewBech32Codec(sdkutil.Bech32PrefixAccAddr),
		),
		evidence.NewAppModule(
			*app.Keepers.Cosmos.Evidence,
		),
		authzmodule.NewAppModule(
			cdc, app.Keepers.Cosmos.Authz,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.interfaceRegistry,
		),
		feegrantmodule.NewAppModule(
			cdc,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.Keepers.Cosmos.FeeGrant,
			app.interfaceRegistry,
		),
		ibc.NewAppModule(
			app.Keepers.Cosmos.IBC,
		),
		transfer.NewAppModule(
			app.Keepers.Cosmos.Transfer,
		),
		ibctm.NewAppModule(
			app.Keepers.Modules.TMLight,
		),
		params.NewAppModule( //nolint: staticcheck
			app.Keepers.Cosmos.Params,
		),
		consensus.NewAppModule(
			cdc,
			*app.Keepers.Cosmos.ConsensusParams,
		),
		escrow.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Escrow,
			app.Keepers.Cosmos.Authz,
			app.Keepers.Cosmos.Bank,
		),
		deployment.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Deployment,
			app.Keepers.Akash.Market,
			app.Keepers.Akash.Escrow,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.Keepers.Cosmos.Authz,
		),
		market.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Market,
			app.Keepers.Akash.Escrow,
			app.Keepers.Akash.Audit,
			app.Keepers.Akash.Deployment,
			app.Keepers.Akash.Provider,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Authz,
			app.Keepers.Cosmos.Bank,
		),
		provider.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Provider,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.Keepers.Akash.Market,
		),
		audit.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Audit,
		),
		cert.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Cert,
		),
		awasm.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Wasm,
		),
		epochs.NewAppModule(
			app.Keepers.Akash.Epochs,
		),
		oracle.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Oracle,
		),
		bme.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Bme,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
		),
		wasm.NewAppModule(
			app.cdc,
			app.Keepers.Cosmos.Wasm,
			app.Keepers.Cosmos.Staking,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.MsgServiceRouter(),
			app.GetSubspace(wasmtypes.ModuleName),
		),
	}
}

func appSimModules(
	app *AkashApp,
	encodingConfig sdkutil.EncodingConfig,
) []module.AppModuleSimulation {
	return []module.AppModuleSimulation{
		auth.NewAppModule(
			app.cdc,
			app.Keepers.Cosmos.Acct,
			authsims.RandomGenesisAccounts,
			app.GetSubspace(authtypes.ModuleName),
		),
		authzmodule.NewAppModule(
			app.cdc,
			app.Keepers.Cosmos.Authz,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.interfaceRegistry,
		),
		bank.NewAppModule(
			app.cdc,
			app.Keepers.Cosmos.Bank,
			app.Keepers.Cosmos.Acct,
			app.GetSubspace(banktypes.ModuleName),
		),
		feegrantmodule.NewAppModule(
			app.cdc,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.Keepers.Cosmos.FeeGrant,
			app.interfaceRegistry,
		),
		authzmodule.NewAppModule(
			app.cdc,
			app.Keepers.Cosmos.Authz,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.interfaceRegistry,
		),
		gov.NewAppModule(
			app.cdc,
			app.Keepers.Cosmos.Gov,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.GetSubspace(govtypes.ModuleName),
		),
		mint.NewAppModule(
			app.cdc,
			app.Keepers.Cosmos.Mint,
			app.Keepers.Cosmos.Acct,
			nil, // todo akash-network/support#4
			app.GetSubspace(minttypes.ModuleName),
		),
		staking.NewAppModule(
			app.cdc,
			app.Keepers.Cosmos.Staking,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.GetSubspace(stakingtypes.ModuleName),
		),
		distr.NewAppModule(
			app.cdc,
			app.Keepers.Cosmos.Distr,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.Keepers.Cosmos.Staking,
			app.GetSubspace(distrtypes.ModuleName),
		),
		slashing.NewAppModule(
			app.cdc,
			app.Keepers.Cosmos.Slashing,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.Keepers.Cosmos.Staking,
			app.GetSubspace(slashingtypes.ModuleName),
			encodingConfig.InterfaceRegistry,
		),
		params.NewAppModule( //nolint: staticcheck
			app.Keepers.Cosmos.Params,
		),
		evidence.NewAppModule(
			*app.Keepers.Cosmos.Evidence,
		),
		ibc.NewAppModule(
			app.Keepers.Cosmos.IBC,
		),
		transfer.NewAppModule(
			app.Keepers.Cosmos.Transfer,
		),
		// akash sim modules
		deployment.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Deployment,
			app.Keepers.Akash.Market,
			app.Keepers.Akash.Escrow,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.Keepers.Cosmos.Authz,
		),
		market.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Market,
			app.Keepers.Akash.Escrow,
			app.Keepers.Akash.Audit,
			app.Keepers.Akash.Deployment,
			app.Keepers.Akash.Provider,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Authz,
			app.Keepers.Cosmos.Bank,
		),
		provider.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Provider,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.Keepers.Akash.Market,
		),
		cert.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Cert,
		),
		epochs.NewAppModule(
			app.Keepers.Akash.Epochs,
		),
		oracle.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Oracle,
		),
		bme.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Bme,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
		),
		awasm.NewAppModule(
			app.cdc,
			app.Keepers.Akash.Wasm,
		),
		wasm.NewAppModule(
			app.cdc,
			app.Keepers.Cosmos.Wasm,
			app.Keepers.Cosmos.Staking,
			app.Keepers.Cosmos.Acct,
			app.Keepers.Cosmos.Bank,
			app.MsgServiceRouter(),
			app.GetSubspace(wasmtypes.ModuleName),
		),
	}
}
