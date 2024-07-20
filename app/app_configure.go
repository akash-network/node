package app

import (
	"github.com/cosmos/cosmos-sdk/types/module"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	capabilitytypes "github.com/cosmos/cosmos-sdk/x/capability/types"
	consensustypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	evidencetypes "github.com/cosmos/cosmos-sdk/x/evidence/types"
	"github.com/cosmos/cosmos-sdk/x/feegrant"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	// auctiontypes "github.com/skip-mev/block-sdk/x/auction/types"

	transfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	ibchost "github.com/cosmos/ibc-go/v7/modules/core/exported"

	audittypes "pkg.akt.dev/go/node/audit/v1"
	certtypes "pkg.akt.dev/go/node/cert/v1"
	deploymenttypes "pkg.akt.dev/go/node/deployment/v1"
	escrowtypes "pkg.akt.dev/go/node/escrow/v1"
	inflationtypes "pkg.akt.dev/go/node/inflation/v1beta3"
	markettypes "pkg.akt.dev/go/node/market/v1beta5"
	providertypes "pkg.akt.dev/go/node/provider/v1beta4"
	taketypes "pkg.akt.dev/go/node/take/v1"

	"pkg.akt.dev/akashd/x/audit"
	"pkg.akt.dev/akashd/x/cert"
	"pkg.akt.dev/akashd/x/deployment"
	"pkg.akt.dev/akashd/x/escrow"
	agov "pkg.akt.dev/akashd/x/gov"
	"pkg.akt.dev/akashd/x/inflation"
	"pkg.akt.dev/akashd/x/market"
	"pkg.akt.dev/akashd/x/provider"
	astaking "pkg.akt.dev/akashd/x/staking"
	"pkg.akt.dev/akashd/x/take"
)

func akashModuleBasics() []module.AppModuleBasic {
	return []module.AppModuleBasic{
		take.AppModuleBasic{},
		escrow.AppModuleBasic{},
		deployment.AppModuleBasic{},
		market.AppModuleBasic{},
		provider.AppModuleBasic{},
		audit.AppModuleBasic{},
		cert.AppModuleBasic{},
		// inflation.AppModuleBasic{},
		astaking.AppModuleBasic{},
		agov.AppModuleBasic{},
	}
}

// akashBeginBlockModules returns all end block modules.
func (app *AkashApp) akashBeginBlockModules() []string {
	return []string{
		upgradetypes.ModuleName,
		capabilitytypes.ModuleName,
		banktypes.ModuleName,
		paramstypes.ModuleName,
		deploymenttypes.ModuleName,
		govtypes.ModuleName,
		agov.ModuleName,
		providertypes.ModuleName,
		certtypes.ModuleName,
		markettypes.ModuleName,
		audittypes.ModuleName,
		genutiltypes.ModuleName,
		vestingtypes.ModuleName,
		crisistypes.ModuleName,
		inflationtypes.ModuleName,
		authtypes.ModuleName,
		authz.ModuleName,
		taketypes.ModuleName,
		escrowtypes.ModuleName,
		minttypes.ModuleName,
		distrtypes.ModuleName,
		slashingtypes.ModuleName,
		evidencetypes.ModuleName,
		stakingtypes.ModuleName,
		astaking.ModuleName,
		transfertypes.ModuleName,
		ibchost.ModuleName,
		feegrant.ModuleName,
	}
}

// akashEndBlockModules returns all end block modules.
func (app *AkashApp) akashEndBlockModules() []string {
	return []string{
		crisistypes.ModuleName,
		govtypes.ModuleName,
		agov.ModuleName,
		stakingtypes.ModuleName,
		astaking.ModuleName,
		upgradetypes.ModuleName,
		capabilitytypes.ModuleName,
		banktypes.ModuleName,
		paramstypes.ModuleName,
		deploymenttypes.ModuleName,
		providertypes.ModuleName,
		certtypes.ModuleName,
		markettypes.ModuleName,
		audittypes.ModuleName,
		genutiltypes.ModuleName,
		vestingtypes.ModuleName,
		inflationtypes.ModuleName,
		authtypes.ModuleName,
		authz.ModuleName,
		taketypes.ModuleName,
		escrowtypes.ModuleName,
		minttypes.ModuleName,
		distrtypes.ModuleName,
		slashingtypes.ModuleName,
		evidencetypes.ModuleName,
		transfertypes.ModuleName,
		ibchost.ModuleName,
		feegrant.ModuleName,
	}
}

// OrderInitGenesis returns module names in order for init genesis calls.
// NOTE: The genutils module must occur after staking so that pools are
// properly initialized with tokens from genesis accounts.
// NOTE: Capability module must occur first so that it can initialize any capabilities
// so that other modules that want to create or claim capabilities afterwards in InitChain
// can do so safely.
func OrderInitGenesis(_ []string) []string {
	return []string{
		capabilitytypes.ModuleName,
		authtypes.ModuleName,
		authz.ModuleName,
		banktypes.ModuleName,
		distrtypes.ModuleName,
		stakingtypes.ModuleName,
		slashingtypes.ModuleName,
		govtypes.ModuleName,
		vestingtypes.ModuleName,
		paramstypes.ModuleName,
		audittypes.ModuleName,
		upgradetypes.ModuleName,
		minttypes.ModuleName,
		crisistypes.ModuleName,
		ibchost.ModuleName,
		evidencetypes.ModuleName,
		transfertypes.ModuleName,
		consensustypes.ModuleName,
		feegrant.ModuleName,
		cert.ModuleName,
		taketypes.ModuleName,
		escrow.ModuleName,
		deployment.ModuleName,
		provider.ModuleName,
		market.ModuleName,
		inflation.ModuleName,
		astaking.ModuleName,
		agov.ModuleName,
		genutiltypes.ModuleName,
		// auctiontypes.ModuleName,
	}
}
