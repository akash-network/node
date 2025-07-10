// Package v1_0_0
// nolint revive
package v1_0_0

import (
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"
	types "pkg.akt.dev/go/node/audit/v1"

	"pkg.akt.dev/go/node/migrate"

	utypes "pkg.akt.dev/node/upgrades/types"
	akeeper "pkg.akt.dev/node/x/audit/keeper"
)

type auditMigrations struct {
	utypes.Migrator
}

func newAuditMigration(m utypes.Migrator) utypes.Migration {
	return auditMigrations{Migrator: m}
}

func (m auditMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates audit store from version 2 to 3.
func (m auditMigrations) handler(ctx sdk.Context) (err error) {
	cdc := m.Codec()

	store := ctx.KVStore(m.StoreKey())
	oStore := prefix.NewStore(store, migrate.AuditedAttributesV1beta3Prefix())

	iter := oStore.Iterator(nil, nil)
	defer func() {
		err = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		val := migrate.AuditedProviderFromV1beta3(cdc, iter.Value())

		owner := sdk.MustAccAddressFromBech32(val.Owner)
		auditor := sdk.MustAccAddressFromBech32(val.Auditor)

		key := akeeper.ProviderKey(types.ProviderID{Owner: owner, Auditor: auditor})

		bz := cdc.MustMarshal(&types.AuditedAttributesStore{Attributes: val.Attributes})

		oStore.Delete(iter.Key())
		store.Set(key, bz)
	}

	return nil
}
