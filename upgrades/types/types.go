package types

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"
	"github.com/tendermint/tendermint/libs/log"

	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"

	apptypes "github.com/akash-network/node/app/types"
)

var (
	upgrades      = map[string]UpgradeInitFn{}
	heightPatches = map[int64]IHeightPatch{}
	migrations    = map[string]moduleMigrations{}

	// actual consensus versions is set when migrations are
	// registered.
	// currentConsensusVersions = map[string]uint64{}
)

type UpgradeInitFn func(log.Logger, *apptypes.App) (IUpgrade, error)
type NewMigrationFn func(Migrator) Migration

type moduleMigrations map[uint64]NewMigrationFn

// IUpgrade defines an interface to run a SoftwareUpgradeProposal
type IUpgrade interface {
	// StoreLoader add|rename|remove stores (aka modules)
	// function may return nil if there is changes to stores
	StoreLoader() *storetypes.StoreUpgrades
	UpgradeHandler() upgradetypes.UpgradeHandler
}

// IHeightPatch defines an interface for a non-software upgrade proposal Height Patch at a given height to implement.
// There is one time code that can be added for the start of the Patch, in `Begin`.
// Any other change in the code should be height-gated, if the goal is to have old and new binaries
// to be compatible prior to the upgrade height.
type IHeightPatch interface {
	Name() string
	Begin(sdk.Context, *apptypes.AppKeepers)
}

type Migrator interface {
	StoreKey() sdk.StoreKey
	Codec() codec.BinaryCodec
}

type migrator struct {
	cdc  codec.BinaryCodec
	skey sdk.StoreKey
}

var _ Migrator = (*migrator)(nil)

func NewMigrator(cdc codec.BinaryCodec, skey sdk.StoreKey) Migrator {
	return &migrator{
		cdc:  cdc,
		skey: skey,
	}
}

func (m *migrator) Codec() codec.BinaryCodec {
	return m.cdc
}

func (m *migrator) StoreKey() sdk.StoreKey {
	return m.skey
}

type Migration interface {
	GetHandler() sdkmodule.MigrationHandler
}

func RegisterUpgrade(name string, fn UpgradeInitFn) {
	if _, exists := upgrades[name]; exists {
		panic(fmt.Sprintf("upgrade \"%s\" already registered", name))
	}

	upgrades[name] = fn
}

func RegisterHeightPatch(height int64, patch IHeightPatch) {
	if _, exists := heightPatches[height]; exists {
		panic(fmt.Sprintf("patch \"%s\" for height %d already registered", patch.Name(), height))
	}

	heightPatches[height] = patch
}

func GetUpgradesList() map[string]UpgradeInitFn {
	return upgrades
}

func GetHeightPatchesList() map[int64]IHeightPatch {
	return heightPatches
}

func RegisterMigration(module string, version uint64, initFn NewMigrationFn) {
	if _, exists := migrations[module]; !exists {
		migrations[module] = make(moduleMigrations)
	}

	if _, exists := migrations[module][version]; exists {
		panic(fmt.Sprintf("migration version (%d) has already been registered for module (%s)", version, module))
	}

	migrations[module][version] = initFn
}

func IterateMigrations(fn func(module string, version uint64, initfn NewMigrationFn)) {
	for module, migrations := range migrations {
		for version, handler := range migrations {
			fn(module, version, handler)
		}
	}
}
