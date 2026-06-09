package v3_0_0

import (
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	types "pkg.akt.dev/go/node/provider/v1beta4"

	utypes "pkg.akt.dev/node/v3/upgrades/types"
	"pkg.akt.dev/node/v3/x/provider/keeper"
)

type providerMigrations struct {
	utypes.Migrator
}

func newProviderMigration(m utypes.Migrator) utypes.Migration {
	return providerMigrations{Migrator: m}
}

func (m providerMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates provider from version 3 to 4.
func (m providerMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())
	providerStore := prefix.NewStore(store, types.ProviderPrefix())

	var registrations []types.ProviderRegistration

	iter := providerStore.Iterator(nil, nil)
	for ; iter.Valid(); iter.Next() {
		var provider types.Provider
		m.Codec().MustUnmarshal(iter.Value(), &provider)

		owner, err := sdk.AccAddressFromBech32(provider.Owner)
		if err != nil {
			_ = iter.Close()
			return types.ErrInvalidAddress.Wrap(err.Error())
		}

		if store.Has(keeper.ProviderRegistrationKey(owner)) {
			continue
		}

		registrations = append(registrations, types.ProviderRegistration{
			Owner:        provider.Owner,
			RegisteredAt: ctx.BlockTime(),
		})
	}
	if err := iter.Close(); err != nil {
		return err
	}

	for _, registration := range registrations {
		owner, err := sdk.AccAddressFromBech32(registration.Owner)
		if err != nil {
			return types.ErrInvalidAddress.Wrap(err.Error())
		}

		store.Set(keeper.ProviderRegistrationKey(owner), m.Codec().MustMarshal(&registration))
	}

	return nil
}
