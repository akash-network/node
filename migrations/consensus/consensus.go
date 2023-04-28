package consensus

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"
)

type Migrator interface {
	StoreKey() sdk.StoreKey
	Codec() codec.BinaryCodec
}

type Migration interface {
	GetHandler() sdkmodule.MigrationHandler
}

type NewMigrationFn func(Migrator) Migration

type moduleMigrations map[uint64]NewMigrationFn

var (
	migrations = map[string]moduleMigrations{}

	currentConsensusVersions = map[string]uint64{
		"audit":      1, // 2
		"cert":       1, // 2
		"deployment": 1, // 3
		"escrow":     1, // 2
		"market":     1, // 2
		"provider":   1, // 2
		// modules below don't have migrations yet as there are in genesis state
		// so set consensus version to 1
		"inflation": 1, // 1
		"agov":      1,
		"astaking":  1,
	}

	// currentConsensusVersions = map[string]uint64{
	// 	"audit":      migrations["audit"].getLatest() + 1,      // 2
	// 	"cert":       migrations["cert"].getLatest() + 1,       // 2
	// 	"deployment": migrations["deployment"].getLatest() + 1, // 3
	// 	"escrow":     migrations["escrow"].getLatest() + 1,     // 2
	// 	"market":     migrations["market"].getLatest() + 1,     // 2
	// 	"provider":   migrations["provider"].getLatest() + 1,   // 2
	//
	// 	// modules below don't have migrations yet as there are in genesis state
	// 	// so set consensus version to 1
	// 	"inflation": 1, // 1
	// 	"agov":      1,
	// 	"astaking":  1,
	// }
)

func RegisterMigration(module string, version uint64, initFn NewMigrationFn) {
	if _, exists := migrations[module]; !exists {
		migrations[module] = make(moduleMigrations)
	}

	if _, exists := migrations[module][version]; exists {
		panic(fmt.Sprintf("migration version (%d) has already been registered for module (%s)", version, module))
	}

	migrations[module][version] = initFn
	if val := currentConsensusVersions[module]; val <= version+1 {
		currentConsensusVersions[module] = version + 1
	}
}

func ModuleVersion(module string) uint64 {
	ver, exists := currentConsensusVersions[module]
	if !exists {
		panic(fmt.Sprintf("requested consensus version for non existing module (%s)", module))
	}

	return ver
}

func ModuleMigrations(module string, migrator Migrator, fn func(string, uint64, sdkmodule.MigrationHandler)) {
	moduleMigrations, exists := migrations[module]
	if !exists {
		return
	}

	for version, initFn := range moduleMigrations {
		migration := initFn(migrator)
		fn(module, version, migration.GetHandler())
	}
}
