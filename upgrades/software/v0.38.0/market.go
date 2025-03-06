// Package v0_38_0
// nolint revive
package v0_38_0

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	mtypesbeta "github.com/akash-network/akash-api/go/node/market/v1beta4"

	utypes "github.com/akash-network/node/upgrades/types"
	"github.com/akash-network/node/x/market/keeper/keys/v1beta4"
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

// handler migrates market from version 5 to 6.
func (m marketMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())
	oiter := sdk.KVStorePrefixIterator(store, mtypesbeta.OrderPrefix())
	defer func() {
		_ = oiter.Close()
	}()

	var ordersTotal uint64
	var ordersOpen uint64
	var ordersActive uint64
	var ordersClosed uint64
	for ; oiter.Valid(); oiter.Next() {
		var val mtypesbeta.Order
		m.Codec().MustUnmarshal(oiter.Value(), &val)

		var state []byte
		switch val.State {
		case mtypesbeta.OrderOpen:
			state = v1beta4.OrderStateOpenPrefix
			ordersOpen++
		case mtypesbeta.OrderActive:
			state = v1beta4.OrderStateActivePrefix
			ordersActive++
		case mtypesbeta.OrderClosed:
			state = v1beta4.OrderStateClosedPrefix
			ordersClosed++
		default:
			panic(fmt.Sprintf("unknown order state %d", val.State))
		}

		ordersTotal++
		store.Delete(v1beta4.OrderKeyLegacy(val.OrderID))

		key, err := v1beta4.OrderKey(state, val.OrderID)
		if err != nil {
			return err
		}

		data, err := m.Codec().Marshal(&val)
		if err != nil {
			return err
		}

		store.Set(key, data)
	}

	biter := sdk.KVStorePrefixIterator(store, mtypesbeta.BidPrefix())
	defer func() {
		_ = biter.Close()
	}()

	var bidsTotal uint64
	var bidsOpen uint64
	var bidsActive uint64
	var bidsLost uint64
	var bidsClosed uint64

	for ; biter.Valid(); biter.Next() {
		var val mtypesbeta.Bid
		m.Codec().MustUnmarshal(biter.Value(), &val)

		switch val.State {
		case mtypesbeta.BidOpen:
			bidsOpen++
		case mtypesbeta.BidActive:
			bidsActive++
		case mtypesbeta.BidLost:
			bidsLost++
		case mtypesbeta.BidClosed:
			bidsClosed++
		default:
			panic(fmt.Sprintf("unknown order state %d", val.State))
		}

		bidsTotal++
		store.Delete(v1beta4.BidKeyLegacy(val.BidID))

		data, err := m.Codec().Marshal(&val)
		if err != nil {
			return err
		}

		state := v1beta4.BidStateToPrefix(val.State)
		key, err := v1beta4.BidKey(state, val.BidID)
		if err != nil {
			return err
		}

		revKey, err := v1beta4.BidStateReverseKey(val.State, val.BidID)
		if err != nil {
			return err
		}

		store.Set(key, data)
		if len(revKey) > 0 {
			store.Set(revKey, data)
		}
	}

	liter := sdk.KVStorePrefixIterator(store, mtypesbeta.LeasePrefix())
	defer func() {
		_ = liter.Close()
	}()

	var leasesTotal uint64
	var leasesActive uint64
	var leasesInsufficientFunds uint64
	var leasesClosed uint64

	for ; liter.Valid(); liter.Next() {
		var val mtypesbeta.Lease
		m.Codec().MustUnmarshal(liter.Value(), &val)

		switch val.State {
		case mtypesbeta.LeaseActive:
			leasesActive++
		case mtypesbeta.LeaseInsufficientFunds:
			leasesInsufficientFunds++
		case mtypesbeta.LeaseClosed:
			leasesClosed++
		default:
			panic(fmt.Sprintf("unknown order state %d", val.State))
		}

		leasesTotal++
		store.Delete(v1beta4.LeaseKeyLegacy(val.LeaseID))

		data, err := m.Codec().Marshal(&val)
		if err != nil {
			return err
		}

		state := v1beta4.LeaseStateToPrefix(val.State)
		key, err := v1beta4.LeaseKey(state, val.LeaseID)
		if err != nil {
			return err
		}

		revKey, err := v1beta4.LeaseStateReverseKey(val.State, val.LeaseID)
		if err != nil {
			return err
		}

		store.Set(key, data)
		if len(revKey) > 0 {
			store.Set(revKey, data)
		}
	}
	ctx.Logger().Info(fmt.Sprintf("[upgrade %s]: updated x/market store keys:"+
		"\n\torders total:              %d"+
		"\n\torders open:               %d"+
		"\n\torders active:             %d"+
		"\n\torders closed:             %d"+
		"\n\tbids total:                %d"+
		"\n\tbids open:                 %d"+
		"\n\tbids active:               %d"+
		"\n\tbids lost:                 %d"+
		"\n\tbids closed:               %d"+
		"\n\tleases total:              %d"+
		"\n\tleases active:             %d"+
		"\n\tleases insufficient funds: %d"+
		"\n\tleases closed:             %d",
		UpgradeName,
		ordersTotal, ordersOpen, ordersActive, ordersClosed,
		bidsTotal, bidsOpen, bidsActive, bidsLost, bidsClosed,
		leasesTotal, leasesActive, leasesInsufficientFunds, leasesClosed))

	return nil
}
