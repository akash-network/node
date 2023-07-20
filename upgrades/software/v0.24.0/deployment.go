// Package v0_24_0
// nolint revive
package v0_24_0

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	dv1beta2 "github.com/akash-network/akash-api/go/node/deployment/v1beta2"
	dmigrate "github.com/akash-network/akash-api/go/node/deployment/v1beta3/migrate"

	utypes "github.com/akash-network/node/upgrades/types"
)

type deploymentMigrations struct {
	utypes.Migrator
}

func newDeploymentMigration(m utypes.Migrator) utypes.Migration {
	return deploymentMigrations{Migrator: m}
}

func (m deploymentMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates deployment from version 2 to 3.
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
