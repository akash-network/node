// Package v1_0_0
// nolint revive
package v1_0_0

import (
	"fmt"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mv1beta "pkg.akt.dev/go/node/market/v1beta5"

	"pkg.akt.dev/go/node/migrate"

	utypes "pkg.akt.dev/node/upgrades/types"
	mkeys "pkg.akt.dev/node/x/market/keeper/keys"
)

type marketMigrations struct {
	utypes.Migrator
}

func newMarketMigration(m utypes.Migrator) utypes.Migration {
	return marketMigrations{Migrator: m}
}

func (m marketMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates market from version 6 to 7.
func (m marketMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	cdc := m.Codec()

	// bid prefixes do not change in this upgrade
	store.Delete(mkeys.BidPrefixReverse)
	biter := storetypes.KVStorePrefixIterator(store, mkeys.BidPrefix)
	defer func() {
		_ = biter.Close()
	}()

	var bidsTotal uint64
	var bidsOpen uint64
	var bidsActive uint64
	var bidsLost uint64
	var bidsClosed uint64

	for ; biter.Valid(); biter.Next() {
		nVal := migrate.BidFromV1beta4(cdc, biter.Value())

		switch nVal.State {
		case mv1beta.BidOpen:
			bidsOpen++
		case mv1beta.BidActive:
			bidsActive++
		case mv1beta.BidLost:
			bidsLost++
		case mv1beta.BidClosed:
			bidsClosed++
		default:
			panic(fmt.Sprintf("unknown order state %d", nVal.State))
		}

		bidsTotal++

		store.Delete(biter.Key())

		data, err := m.Codec().Marshal(&nVal)
		if err != nil {
			return err
		}

		state := mkeys.BidStateToPrefix(nVal.State)
		key, err := mkeys.BidKey(state, nVal.ID)
		if err != nil {
			return err
		}

		revKey, err := mkeys.BidReverseKey(state, nVal.ID)
		if err != nil {
			return err
		}

		store.Set(key, data)
		if len(revKey) > 0 {
			store.Set(revKey, data)
		}
	}

	// lease prefixes do not change in this upgrade
	store.Delete(mkeys.LeasePrefixReverse)
	liter := storetypes.KVStorePrefixIterator(store, mkeys.LeasePrefix)
	defer func() {
		_ = liter.Close()
	}()

	var leasesTotal uint64
	var leasesActive uint64
	var leasesInsufficientFunds uint64
	var leasesClosed uint64

	for ; liter.Valid(); liter.Next() {
		nVal := migrate.LeaseFromV1beta4(cdc, liter.Value())

		switch nVal.State {
		case mv1.LeaseActive:
			leasesActive++
		case mv1.LeaseInsufficientFunds:
			leasesInsufficientFunds++
		case mv1.LeaseClosed:
			leasesClosed++
		default:
			panic(fmt.Sprintf("unknown order state %d", nVal.State))
		}

		leasesTotal++
		store.Delete(liter.Key())

		data, err := m.Codec().Marshal(&nVal)
		if err != nil {
			return err
		}

		state := mkeys.LeaseStateToPrefix(nVal.State)
		key, err := mkeys.LeaseKey(state, nVal.ID)
		if err != nil {
			return err
		}

		revKey, err := mkeys.LeaseReverseKey(state, nVal.ID)
		if err != nil {
			return err
		}

		store.Set(key, data)
		if len(revKey) > 0 {
			store.Set(revKey, data)
		}
	}

	return nil
}
