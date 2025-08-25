// Package v1_0_0
// nolint revive
package v1_0_0

import (
	"fmt"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"
	etypes "pkg.akt.dev/go/node/escrow/v1"
	"pkg.akt.dev/go/node/migrate"

	utypes "pkg.akt.dev/node/upgrades/types"
	ekeeper "pkg.akt.dev/node/x/escrow/keeper"
)

type escrowMigrations struct {
	utypes.Migrator
}

func newEscrowMigration(m utypes.Migrator) utypes.Migration {
	return escrowMigrations{Migrator: m}
}

func (m escrowMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates escrow store from version 2 to 3.
func (m escrowMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	oStore := prefix.NewStore(store, migrate.AccountV1beta3Prefix())

	iter := oStore.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	cdc := m.Codec()

	var accountsTotal uint64
	var accountsActive uint64
	var accountsClosed uint64
	var accountsOverdrawn uint64

	for ; iter.Valid(); iter.Next() {
		nVal := migrate.AccountFromV1beta3(cdc, iter.Value())
		bz := cdc.MustMarshal(&nVal)

		switch nVal.State {
		case etypes.StateOpen:
			accountsActive++
		case etypes.StateClosed:
			accountsClosed++
		case etypes.StateOverdrawn:
			accountsOverdrawn++
		}

		accountsTotal++

		key := ekeeper.LegacyAccountKey(nVal.ID)
		oStore.Delete(iter.Key())

		key = ekeeper.BuildAccountsKey(nVal.State, &nVal.ID)
		store.Set(key, bz)
	}

	oStore = prefix.NewStore(store, migrate.PaymentV1beta3Prefix())

	iter = oStore.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	var paymentsTotal uint64
	var paymentsActive uint64
	var paymentsClosed uint64
	var paymentsOverdrawn uint64

	for ; iter.Valid(); iter.Next() {
		nVal := migrate.FractionalPaymentFromV1beta3(cdc, iter.Value())
		bz := cdc.MustMarshal(&nVal)

		switch nVal.State {
		case etypes.StateOpen:
			paymentsActive++
		case etypes.StateClosed:
			paymentsClosed++
		case etypes.StateOverdrawn:
			paymentsOverdrawn++
		}

		paymentsTotal++

		key := ekeeper.LegacyPaymentKey(nVal.AccountID, nVal.PaymentID)
		oStore.Delete(iter.Key())

		key = ekeeper.BuildPaymentsKey(nVal.State, &nVal.AccountID, nVal.PaymentID)
		store.Set(key, bz)
	}

	ctx.Logger().Info(fmt.Sprintf("[upgrade %s]: updated x/escrow store keys:"+
		"\n\taccounts total:              %d"+
		"\n\taccounts open:               %d"+
		"\n\taccounts closed:             %d"+
		"\n\taccounts overdrawn:          %d"+
		"\n\tpayments total:              %d"+
		"\n\tpayments open:               %d"+
		"\n\tpayments closed:             %d"+
		"\n\tpayments overdrawn:          %d",
		UpgradeName,
		accountsTotal, accountsActive, accountsClosed, accountsOverdrawn,
		paymentsTotal, paymentsActive, paymentsClosed, paymentsOverdrawn))

	return nil
}
