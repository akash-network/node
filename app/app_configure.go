package app

import (
	evidencetypes "cosmossdk.io/x/evidence/types"
	"cosmossdk.io/x/feegrant"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	consensustypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	transfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	ibchost "github.com/cosmos/ibc-go/v10/modules/core/exported"

	audittypes "pkg.akt.dev/go/node/audit/v1"

	"pkg.akt.dev/node/v2/x/audit"
	"pkg.akt.dev/node/v2/x/cert"
	"pkg.akt.dev/node/v2/x/deployment"
	"pkg.akt.dev/node/v2/x/epochs"
	"pkg.akt.dev/node/v2/x/escrow"
	"pkg.akt.dev/node/v2/x/market"
	"pkg.akt.dev/node/v2/x/oracle"
	"pkg.akt.dev/node/v2/x/provider"
	"pkg.akt.dev/node/v2/x/take"
	awasm "pkg.akt.dev/node/v2/x/wasm"
)

func akashModuleBasics() []module.AppModuleBasic {
	return []module.AppModuleBasic{
		epochs.AppModuleBasic{},
		escrow.AppModuleBasic{},
		deployment.AppModuleBasic{},
		market.AppModuleBasic{},
		provider.AppModuleBasic{},
		audit.AppModuleBasic{},
		cert.AppModuleBasic{},
		oracle.AppModuleBasic{},
		take.AppModuleBasic{},
		awasm.AppModuleBasic{},
	}
}

// OrderInitGenesis returns module names in order for init genesis calls.
// NOTE: The genutils module must occur after staking so that pools are
// properly initialized with tokens from genesis accounts.
// NOTE: Capability module must occur first so that it can initialize any capabilities
// so that other modules that want to create or claim capabilities afterwards in InitChain
// can do so safely.
func orderInitGenesis(_ []string) []string {
	return []string{
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
		take.ModuleName,
		escrow.ModuleName,
		deployment.ModuleName,
		provider.ModuleName,
		market.ModuleName,
		genutiltypes.ModuleName,
		oracle.ModuleName,
		epochs.ModuleName,
		awasm.ModuleName,
		wasmtypes.ModuleName,
	}
}
