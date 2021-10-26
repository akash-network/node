package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	v015 "github.com/ovrclk/akash/x/market/legacy/v015"
)

// Migrator is a struct for handling in-place store migrations.
type Migrator struct {
	keeper Keeper
}

// NewMigrator returns a new Migrator.
func NewMigrator(k IKeeper) Migrator {
	return Migrator{keeper: k.(Keeper)}
}

// Migrate1to2 migrates from version 1 to 2.
func (m Migrator) Migrate1to2(ctx sdk.Context) error {
	return v015.MigrateStore(ctx, m.keeper.skey, m.keeper.cdc)
}
