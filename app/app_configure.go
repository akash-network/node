package app

import (
	"github.com/cosmos/cosmos-sdk/types/module"

	"pkg.akt.dev/node/v2/x/audit"
	"pkg.akt.dev/node/v2/x/bme"
	"pkg.akt.dev/node/v2/x/cert"
	"pkg.akt.dev/node/v2/x/deployment"
	"pkg.akt.dev/node/v2/x/epochs"
	"pkg.akt.dev/node/v2/x/escrow"
	"pkg.akt.dev/node/v2/x/market"
	"pkg.akt.dev/node/v2/x/oracle"
	"pkg.akt.dev/node/v2/x/provider"
	"pkg.akt.dev/node/v2/x/sink"
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
		awasm.AppModuleBasic{},
		bme.AppModuleBasic{},
		sink.AppModuleBasic{},
	}
}
