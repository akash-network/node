# ADR 001: Implementing network upgrades

## Changelog

* 2023/04/28: Initial implementation @troian

## Status

InProgress

## Upgrade types

- [Software upgrade](#software-upgrade) - upgrade network state with software upgrade proposal.
    - [Upgrade handler](#upgrade-handler) **mandatory**
    - [State migrations](#state-migrations) **optional**
- [Height patch](#implementing-height-patch) - Allows urgent patching of the network state at given height

## Software upgrade

We will refer to a [v0.24.0](../../upgrades/software/v0.24.0) as an example in this guide

Software upgrades are located within this [directory](../../upgrades/software)
Each upgrade must be contained within own directory. Name of the directory corresponds to upgrade name which is always Semver compliant.

To keep upgrades consistent, they must implement following file structure.
Each file has steps in form of comment with `StepX` prefix. Each step must be implemented unless stated in comment
1. have [upgrade.go](#upgradego) file
2. have [init.go](#initgo) file
3. have dedicated file for each module that requires [state migration](#state-migrations)
4. if any helpers needed they must be located in `helpers.go`
5. [register](#register-upgrade-handler) upgrade handler
6. [Test](#testing-software-upgrade) upgrade
7. Update [changelog](../../upgrades/CHANGELOG.md) with new upgrade info

#### upgrade.go

```go
// Package v0_24_0 # stops linter complaining about package name with underscores
// nolint: revive
package v0_24_0

import (
	"github.com/tendermint/tendermint/libs/log"

	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	apptypes "github.com/akash-network/node/app/types"
	utypes "github.com/akash-network/node/upgrades/types"
)

// Step1 (mandatory): declare upgrade name. Must be Semver compliant with v prefix
const (
	upgradeName = "v0.24.0"
)

// Step2 (mandatory): declare upgrade implementations
type upgrade struct {
	*apptypes.App
	log log.Logger
}

// Step3: ensure upgrade implement software upgrade interface
var _ utypes.IUpgrade = (*upgrade)(nil)

// Step4 (mandatory): initialize upgrade. function will be called from `init.go`
func initUpgrade(log log.Logger, app *apptypes.App) (utypes.IUpgrade, error) {
	up := &upgrade{
		App: app,
		log: log.With(fmt.Sprintf("upgrade/%s", upgradeName)),
	}
	
	
	// Step 4.1 (optional): check required modules when necessary 
	
	
	return up, nil
}

// StoreLoader add|rename|remove stores (aka modules)
// Step5 (mandatory): implement changes to stores. If no stores added|renamed|removed function must return nil
func (up *upgrade) StoreLoader() *storetypes.StoreUpgrades {
	return nil
}

// Step6 (mandatory): implement upgrade handler
func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx sdk.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		// Step6.1 (optional): perform validations and state patching if necessary
		
		// Step6.2 (mandatory): following must always be present as last line
		return up.MM.RunMigrations(ctx, up.Configurator, fromVM)
	}
}

```

#### init.go

```go
// Package v0_24_0
// nolint revive
package v0_24_0

import (
	utypes "github.com/akash-network/node/upgrades/types"
)

// Step1 (mandatory): create init function
func init() {
	// Step1.1 (mandatory): register upgrade
	utypes.RegisterUpgrade(upgradeName, initUpgrade)
	
	// Step1.2 (optional) register state migrations for each module
	// To determine migration version:
	//    Find module in [changelog](../../upgrades/CHANGELOG.md)
	//    Increment version in changelog and put new value when registering migration below
	utypes.RegisterMigration(dv1beta3.ModuleName, 2, newDeploymentMigration)
}

```

#### State migrations
```go
// Package v0_24_0
// nolint revive
package v0_24_0

// Step1 (mandatory): define migrator <module name>Migrations
type deploymentMigrations struct {
	utypes.Migrator
}

// Step2 (mandatory): ensure migration implements utypes.Migrator interface

var _ utypes.Migrator = (*deploymentMigrations)(nil)

// Step3 (mandatory): initialize migrator. function will be registered in init.go. Check example above
func newDeploymentMigration(m utypes.Migrator) utypes.Migration {
	return deploymentMigrations{Migrator: m}
}

// Step4 (mandatory): implement GetHandler stub (implementation is same for all migrations)
func (m deploymentMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates deployment from version 2 to 3.
// Step5 (mandatory): implement migration handler
func (m deploymentMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	err := utypes.MigrateValue(store, m.Codec(), dv1beta2.GroupPrefix(), migrateDeploymentGroup)

	if err != nil {
		return err
	}

	return nil
}

func migrateDeploymentGroup(fromBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler {
	var from dv1beta2.Group
	cdc.MustUnmarshal(fromBz, &from)

	to := dmigrate.GroupFromV1Beta2(from)
	return &to
}
```

#### Register upgrade handler

1. register upgrade within [upgrades/upgrades.go](../../upgrades/upgrades.go)
   ```go
   import (
   	// nolint: revive
   	_ "github.com/akash-network/node/upgrades/software/v0.24.0"
   )
   ```
2. Once imported, the upgrade will register itself, and `App` will initialize it during startup
3. To deregister obsolete upgrade simply remove respective import from [upgrades/upgrades.go](../upgrades/upgrades.go)

##### Testing software upgrade
1. cd `tests/upgrade`
2. Create test config `upgrade-<upgrade name>.json` (use `upgrade-v0.24.0.json` as reference)
3. Run test
   ```shell
   UPGRADE_TO=<upgrade name> make test
   ```
4. To reset test `make test-reset`
