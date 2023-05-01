// Package v0_15_0
// nolint revive
package v0_15_0

import (
	dv1beta1 "github.com/akash-network/akash-api/go/node/deployment/v1beta1"
	dmigrate "github.com/akash-network/akash-api/go/node/deployment/v1beta2/migrate"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

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

// handler migrates deployment from version 1 to 2.
func (m deploymentMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	migratePrefixBech32AddrBytes(store, dv1beta1.DeploymentPrefix())
	migratePrefixBech32AddrBytes(store, dv1beta1.GroupPrefix())

	err := utypes.MigrateValue(store, m.Codec(), dv1beta1.GroupPrefix(), migrateDeploymentGroup)
	if err != nil {
		return err
	}

	return nil
}

func migrateDeploymentGroup(fromBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler {
	var from dv1beta1.Group
	cdc.MustUnmarshal(fromBz, &from)

	to := dmigrate.GroupFromV1Beta1(from)
	return &to
}
