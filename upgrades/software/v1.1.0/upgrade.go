// Package v1_1_0
// nolint revive
package v1_1_0

import (
	"context"
	"fmt"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
	escrowid "pkg.akt.dev/go/node/escrow/id/v1"
	idv1 "pkg.akt.dev/go/node/escrow/id/v1"
	emodule "pkg.akt.dev/go/node/escrow/module"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"

	apptypes "pkg.akt.dev/node/app/types"
	utypes "pkg.akt.dev/node/upgrades/types"
	ekeeper "pkg.akt.dev/node/x/escrow/keeper"
	"pkg.akt.dev/node/x/market"
	mhooks "pkg.akt.dev/node/x/market/hooks"
	"pkg.akt.dev/node/x/market/keeper/keys"
)

const (
	UpgradeName = "v1.1.0"
)

type upgrade struct {
	*apptypes.App
	log log.Logger
}

var _ utypes.IUpgrade = (*upgrade)(nil)

func initUpgrade(log log.Logger, app *apptypes.App) (utypes.IUpgrade, error) {
	up := &upgrade{
		App: app,
		log: log.With("module", fmt.Sprintf("upgrade/%s", UpgradeName)),
	}

	return up, nil
}

func (up *upgrade) StoreLoader() *storetypes.StoreUpgrades {
	return &storetypes.StoreUpgrades{}
}

func (up *upgrade) UpgradeHandler() upgradetypes.UpgradeHandler {
	return func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		toVM, err := up.MM.RunMigrations(ctx, up.Configurator, fromVM)
		if err != nil {
			return nil, err
		}

		sctx := sdk.UnwrapSDKContext(ctx)
		err = up.closeOverdrawnEscrowAccounts(sctx)
		if err != nil {
			return nil, err
		}

		up.log.Info(fmt.Sprintf("all migrations have been completed"))

		return toVM, err
	}
}

func (up *upgrade) closeOverdrawnEscrowAccounts(ctx sdk.Context) error {
	store := ctx.KVStore(up.GetKey(emodule.StoreKey))
	searchPrefix := ekeeper.BuildSearchPrefix(ekeeper.AccountPrefix, etypes.StateOpen.String(), "")

	searchStore := prefix.NewStore(store, searchPrefix)

	iter := searchStore.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	cdc := up.GetCodec()

	totalAccounts := 0
	totalPayments := 0

	for ; iter.Valid(); iter.Next() {
		id, _ := ekeeper.ParseAccountKey(append(searchPrefix, iter.Key()...))
		val := etypes.Account{
			ID: id,
		}

		cdc.MustUnmarshal(iter.Value(), &val.State)

		if val.State.Funds[0].Denom != "ibc/170C677610AC31DF0904FFE09CD3B5C657492170E7E52372E48756B71E56F2F1" {
			continue
		}

		aPrevState := val.State.State

		heightDelta := ctx.BlockHeight() + val.State.SettledAt

		totalAvailableDeposits := sdkmath.LegacyZeroDec()

		for _, deposit := range val.State.Deposits {
			totalAvailableDeposits.AddMut(deposit.Balance.Amount)
		}

		payments := up.accountPayments(cdc, store, id, []etypes.State{etypes.StateOpen, etypes.StateOverdrawn})

		totalBlockRate := sdkmath.LegacyZeroDec()

		for _, pmnt := range payments {
			totalBlockRate.AddMut(pmnt.State.Rate.Amount)

			if pmnt.State.State == etypes.StateOverdrawn {
				val.State.State = etypes.StateOverdrawn
			}
		}

		owed := sdkmath.LegacyZeroDec()
		owed.AddMut(totalBlockRate)
		owed.MulInt64Mut(heightDelta)

		overdraft := totalAvailableDeposits.LTE(owed) || val.State.State == etypes.StateOverdrawn

		totalAccounts++

		val.State.Deposits = nil
		val.State.Funds[0].Amount = val.State.Funds[0].Amount.Sub(owed)

		key := ekeeper.BuildAccountsKey(aPrevState, &val.ID)
		store.Delete(key)

		if !overdraft {
			val.State.State = etypes.StateClosed
		}

		// find associated deployment/groups/lease/bid and close it
		hooks := mhooks.New(up.Keepers.Akash.Deployment, up.Keepers.Akash.Market)

		err := up.OnEscrowAccountClosed(ctx, val)
		if err != nil {
			return err
		}

		key = ekeeper.BuildAccountsKey(val.State.State, &val.ID)
		store.Set(key, cdc.MustMarshal(&val.State))

		for i := range payments {
			totalPayments++
			key = ekeeper.BuildPaymentsKey(payments[i].State.State, &payments[i].ID)
			store.Delete(key)

			payments[i].State.State = etypes.StateClosed
			if overdraft {
				payments[i].State.State = etypes.StateOverdrawn
			}

			payments[i].State.Balance.Amount.Set(sdkmath.LegacyZeroDec())
			payments[i].State.Unsettled.Amount.Set(payments[i].State.Rate.Amount.MulInt64Mut(heightDelta))

			key = ekeeper.BuildPaymentsKey(payments[i].State.State, &payments[i].ID)
			err = hooks.OnEscrowPaymentClosed(ctx, payments[i])
			if err != nil {
				return err
			}

			store.Set(key, cdc.MustMarshal(&payments[i].State))
		}
	}

	biter := searchStore.Iterator(nil, nil)
	defer func() {
		_ = biter.Close()
	}()

	for ; biter.Valid(); biter.Next() {
		eid, _ := ekeeper.ParseAccountKey(append(searchPrefix, biter.Key()...))
		val := etypes.Account{
			ID: eid,
		}

		if eid.Scope != idv1.ScopeDeployment {
			continue
		}

		cdc.MustUnmarshal(biter.Value(), &val.State)
		aPrevState := val.State.State

		did, err := dv1.DeploymentIDFromEscrowID(val.ID)
		if err != nil {
			return err
		}

		deployment, found := up.Keepers.Akash.Deployment.GetDeployment(ctx, did)
		if !found {
			return nil
		}

		if deployment.State == dv1.DeploymentClosed {
			totalAccounts++

			val.State.Deposits = nil
			val.State.State = etypes.StateClosed
			val.State.Funds[0].Amount.Set(sdkmath.LegacyZeroDec())

			key := ekeeper.BuildAccountsKey(aPrevState, &val.ID)
			store.Delete(key)

			key = ekeeper.BuildAccountsKey(val.State.State, &val.ID)
			store.Set(key, cdc.MustMarshal(&val.State))

			payments := up.accountPayments(cdc, store, eid, []etypes.State{etypes.StateOpen, etypes.StateOverdrawn})

			for i := range payments {
				totalPayments++
				key = ekeeper.BuildPaymentsKey(payments[i].State.State, &payments[i].ID)
				store.Delete(key)

				payments[i].State.State = etypes.StateClosed
				payments[i].State.Balance.Amount.Set(sdkmath.LegacyZeroDec())

				key = ekeeper.BuildPaymentsKey(payments[i].State.State, &payments[i].ID)
				store.Set(key, cdc.MustMarshal(&payments[i].State))
			}
		}
	}

	up.log.Info(fmt.Sprintf("cleaned up overdrawn:\n"+
		"\taccounts: %d\n"+
		"\tpayments: %d", totalAccounts, totalPayments))

	return nil
}

func (up *upgrade) accountPayments(cdc codec.Codec, store storetypes.KVStore, id escrowid.Account, states []etypes.State) []etypes.Payment {
	var payments []etypes.Payment

	iters := make([]storetypes.Iterator, 0, len(states))
	defer func() {
		for _, iter := range iters {
			_ = iter.Close()
		}
	}()

	for _, state := range states {
		pprefix := ekeeper.BuildPaymentsKey(state, &id)
		iter := storetypes.KVStorePrefixIterator(store, pprefix)
		iters = append(iters, iter)

		for ; iter.Valid(); iter.Next() {
			id, _ := ekeeper.ParsePaymentKey(iter.Key())
			val := etypes.Payment{
				ID: id,
			}
			cdc.MustUnmarshal(iter.Value(), &val.State)
			payments = append(payments, val)
		}
	}
	return payments
}

func (up *upgrade) OnEscrowAccountClosed(ctx sdk.Context, obj etypes.Account) error {
	id, err := dv1.DeploymentIDFromEscrowID(obj.ID)
	if err != nil {
		return err
	}

	deployment, found := up.Keepers.Akash.Deployment.GetDeployment(ctx, id)
	if !found {
		return nil
	}

	if deployment.State != dv1.DeploymentActive {
		return nil
	}
	err = up.Keepers.Akash.Deployment.CloseDeployment(ctx, deployment)
	if err != nil {
		return err
	}

	gstate := dtypes.GroupClosed
	if obj.State.State == etypes.StateOverdrawn {
		gstate = dtypes.GroupInsufficientFunds
	}

	for _, group := range up.Keepers.Akash.Deployment.GetGroups(ctx, deployment.ID) {
		if group.ValidateClosable() == nil {
			err = up.Keepers.Akash.Deployment.OnCloseGroup(ctx, group, gstate)
			if err != nil {
				return err
			}
			err = up.OnGroupClosed(ctx, group.ID)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (up *upgrade) OnGroupClosed(ctx sdk.Context, id dv1.GroupID) error {
	processClose := func(ctx sdk.Context, bid mtypes.Bid) error {
		err := up.Keepers.Akash.Market.OnBidClosed(ctx, bid)
		if err != nil {
			return err
		}

		if lease, ok := up.Keepers.Akash.Market.GetLease(ctx, bid.ID.LeaseID()); ok {
			// OnGroupClosed is callable by x/deployment only so only reason is owner
			err = up.Keepers.Akash.Market.OnLeaseClosed(ctx, lease, mv1.LeaseClosed, mv1.LeaseClosedReasonOwner)
			if err != nil {
				return err
			}
		}

		return nil
	}

	var err error
	up.Keepers.Akash.Market.WithOrdersForGroup(ctx, id, mtypes.OrderActive, func(order mtypes.Order) bool {
		err = up.Keepers.Akash.Market.OnOrderClosed(ctx, order)
		if err != nil {
			return true
		}

		up.Keepers.Akash.Market.WithBidsForOrder(ctx, order.ID, mtypes.BidOpen, func(bid mtypes.Bid) bool {
			err = processClose(ctx, bid)
			return err != nil
		})

		if err != nil {
			return true
		}

		up.Keepers.Akash.Market.WithBidsForOrder(ctx, order.ID, mtypes.BidActive, func(bid mtypes.Bid) bool {
			err = processClose(ctx, bid)
			return err != nil
		})

		return err != nil
	})

	if err != nil {
		return err
	}

	return nil
}

func (up *upgrade) OnEscrowPaymentClosed(ctx sdk.Context, obj etypes.Payment) error {
	id, err := mv1.LeaseIDFromPaymentID(obj.ID)
	if err != nil {
		// Escrow payments can belong to different scopes (e.g., bid-scoped, deployment-scoped).
		// This upgrade hook only processes lease payments (deployment-scoped).
		// Silently ignore non-lease payment closures.
		return nil
	}

	bid, ok := up.Keepers.Akash.Market.GetBid(ctx, id.BidID())
	if !ok {
		return nil
	}

	if bid.State != mtypes.BidActive {
		return nil
	}

	order, ok := up.Keepers.Akash.Market.GetOrder(ctx, id.OrderID())
	if !ok {
		return mv1.ErrOrderNotFound
	}

	lease, ok := up.Keepers.Akash.Market.GetLease(ctx, id)
	if !ok {
		return mv1.ErrLeaseNotFound
	}

	err = up.Keepers.Akash.Market.OnOrderClosed(ctx, order)
	if err != nil {
		return err
	}
	err = up.OnBidClosed(ctx, bid)
	if err != nil {
		return err
	}

	if obj.State.State == etypes.StateOverdrawn {
		err = up.Keepers.Akash.Market.OnLeaseClosed(ctx, lease, mv1.LeaseInsufficientFunds, mv1.LeaseClosedReasonInsufficientFunds)
		if err != nil {
			return err
		}
	} else {
		err = up.Keepers.Akash.Market.OnLeaseClosed(ctx, lease, mv1.LeaseClosed, mv1.LeaseClosedReasonUnspecified)
		if err != nil {
			return err
		}
	}

	return nil
}

// OnBidClosed updates bid state to closed
func (up *upgrade) OnBidClosed(ctx sdk.Context, bid mtypes.Bid) error {
	switch bid.State {
	case mtypes.BidClosed, mtypes.BidLost:
		return nil
	}

	currState := bid.State
	bid.State = mtypes.BidClosed
	up.updateBid(ctx, bid, currState)

	err := ctx.EventManager().EmitTypedEvent(
		&mv1.EventBidClosed{
			ID: bid.ID,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

func (up *upgrade) updateBid(ctx sdk.Context, bid mtypes.Bid, currState mtypes.Bid_State) {
	store := ctx.KVStore(up.GetKey(market.StoreKey))

	switch currState {
	case mtypes.BidOpen:
	case mtypes.BidActive:
	default:
		panic(fmt.Sprintf("unexpected current state of the bid: %d", currState))
	}

	key := keys.MustBidKey(keys.BidStateToPrefix(currState), bid.ID)
	revKey := keys.MustBidStateRevereKey(currState, bid.ID)
	store.Delete(key)
	if revKey != nil {
		store.Delete(revKey)
	}

	switch bid.State {
	case mtypes.BidActive:
	case mtypes.BidLost:
	case mtypes.BidClosed:
	default:
		panic(fmt.Sprintf("unexpected new state of the bid: %d", bid.State))
	}

	data := up.App.Cdc.MustMarshal(&bid)

	key = keys.MustBidKey(keys.BidStateToPrefix(bid.State), bid.ID)
	revKey = keys.MustBidStateRevereKey(bid.State, bid.ID)

	store.Set(key, data)
	if len(revKey) > 0 {
		store.Set(revKey, data)
	}
}
