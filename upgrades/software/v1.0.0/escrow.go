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
		case etypes.AccountOpen:
			accountsActive++
		case etypes.AccountClosed:
			accountsClosed++
		case etypes.AccountOverdrawn:
			accountsOverdrawn++
		}

		accountsTotal++

		key := ekeeper.AccountKey(nVal.ID)

		oStore.Delete(iter.Key())
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
		case etypes.PaymentOpen:
			paymentsActive++
		case etypes.PaymentClosed:
			paymentsClosed++
		case etypes.PaymentOverdrawn:
			paymentsOverdrawn++
		}

		paymentsTotal++

		key := ekeeper.PaymentKey(nVal.AccountID, nVal.PaymentID)

		oStore.Delete(iter.Key())
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
