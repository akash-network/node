package v2_2_0

import (
	"fmt"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	escrowid "pkg.akt.dev/go/node/escrow/id/v1"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"

	ekeeper "pkg.akt.dev/node/v2/x/escrow/keeper"
	mkeeper "pkg.akt.dev/node/v2/x/market/keeper"
)

// EscrowRepairResult reports containment, not reimbursement. Missing leases do
// not establish that a payment should be closed and require separate review.
type EscrowRepairResult struct {
	ClosedPayments uint64
	MissingLeases  uint64
}

// RepairEscrowPayments closes open payments whose leases are already terminal.
// It changes only the payment state and index: balances, debt, prior withdrawals,
// account settlement heights and bank balances remain untouched. Held balances
// may include both legitimate earnings and invalid accrual, so paying or refunding
// them requires historical reconciliation rather than another settlement.
func RepairEscrowPayments(ctx sdk.Context, escrow ekeeper.Keeper, market mkeeper.IKeeper) (EscrowRepairResult, error) {
	cached, commit := ctx.CacheContext()
	store := cached.KVStore(escrow.StoreKey())
	prefix := ekeeper.BuildPaymentsKey(etypes.StateOpen, nil)
	iter := storetypes.KVStorePrefixIterator(store, prefix)

	type change struct {
		oldKey []byte
		newKey []byte
		value  []byte
	}
	var changes []change
	result := EscrowRepairResult{}

	// Finish discovery, validation and encoding before closing the iterator and
	// changing any keys. In particular, never overwrite a second state for an ID.
	err := func() error {
		for ; iter.Valid(); iter.Next() {
			if len(iter.Key()) <= len(prefix)+1 {
				return fmt.Errorf("malformed open escrow payment key %X", iter.Key())
			}
			id, _, err := ekeeper.ParsePaymentKey(iter.Key())
			if err != nil {
				return fmt.Errorf("parse open escrow payment: %w", err)
			}
			if id.AID.Scope != escrowid.ScopeDeployment {
				continue
			}
			leaseID, err := mv1.LeaseIDFromPaymentID(id)
			if err != nil {
				return fmt.Errorf("parse lease for payment %s: %w", id.Key(), err)
			}
			var state etypes.PaymentState
			if err = escrow.Codec().Unmarshal(iter.Value(), &state); err != nil {
				return fmt.Errorf("decode payment %s: %w", id.Key(), err)
			}
			if state.State != etypes.StateOpen {
				return fmt.Errorf("payment %s has state %s under its open key", id.Key(), state.State)
			}
			closedKey := ekeeper.BuildPaymentsKey(etypes.StateClosed, &id)
			if store.Has(closedKey) || store.Has(ekeeper.BuildPaymentsKey(etypes.StateOverdrawn, &id)) {
				return fmt.Errorf("payment %s exists in multiple state indexes", id.Key())
			}
			lease, found := market.GetLease(cached, leaseID)
			if !found {
				result.MissingLeases++
				continue
			}
			if lease.State != mv1.LeaseClosed && lease.State != mv1.LeaseInsufficientFunds {
				continue
			}

			state.State = etypes.StateClosed
			value, err := escrow.Codec().Marshal(&state)
			if err != nil {
				return fmt.Errorf("encode repaired payment %s: %w", id.Key(), err)
			}
			changes = append(changes, change{
				oldKey: append([]byte(nil), iter.Key()...),
				newKey: closedKey,
				value:  value,
			})
		}
		return nil
	}()
	closeErr := iter.Close()
	if err != nil {
		return EscrowRepairResult{}, err
	}
	if closeErr != nil {
		return EscrowRepairResult{}, closeErr
	}

	for _, entry := range changes {
		// Normal PaymentClose would accrue rent through the upgrade height.
		// SavePaymentRaw alone would leave the old open key available to settlement.
		store.Delete(entry.oldKey)
		store.Set(entry.newKey, entry.value)
	}
	result.ClosedPayments = uint64(len(changes))
	commit()
	return result, nil
}
