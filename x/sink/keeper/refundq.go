package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

// The refund queue indexes refundable transfers: voided with no hold. Keys
// order by challenge end, then ID, so the head is the oldest matured refund.
// Only the head may be paid, which stops a small refund from starving a
// cap-sized one at the start of a day. Like the open index, the queue is
// derived: InitGenesis rebuilds it and export omits it.
func refundKey(s types.Settlement, id uint64) collections.Pair[uint64, uint64] {
	return collections.Join(uint64(s.ChallengeEnd), id) //nolint:gosec // positive for a queued void
}

func queued(s types.Settlement) bool {
	return s.Status == types.Status_STATUS_VOIDED && s.Hold == types.Hold_HOLD_NONE
}

// putSettlement stores a settlement and keeps the refund queue in step.
// before is the settlement as loaded, after the one to store.
func (k Keeper) putSettlement(ctx context.Context, id uint64, before, after types.Settlement) error {
	if queued(before) {
		if err := k.refunds.Remove(ctx, refundKey(before, id)); err != nil {
			return err
		}
	}
	if queued(after) {
		if err := k.refunds.Set(ctx, refundKey(after, id)); err != nil {
			return err
		}
	}
	return k.settlements.Set(ctx, id, after)
}

// refundQueueHead returns the transfer that must be refunded next.
func (k Keeper) refundQueueHead(ctx context.Context) (uint64, bool, error) {
	it, err := k.refunds.Iterate(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = it.Close() }()
	if !it.Valid() {
		return 0, false, nil
	}
	key, err := it.Key()
	if err != nil {
		return 0, false, fmt.Errorf("malformed sink refund queue key: %w", err)
	}
	return key.K2(), true, nil
}

// largestOpenAmount is the largest transfer that is neither minted nor
// refunded, so a lowered refund cap never strands an open transfer.
func (k Keeper) largestOpenAmount(ctx sdk.Context) (uint64, error) {
	it, err := k.open.Iterate(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = it.Close() }()
	var largest uint64
	for ; it.Valid(); it.Next() {
		id, err := it.Key()
		if err != nil {
			return 0, fmt.Errorf("malformed open sink transfer key: %w", err)
		}
		t, _, err := k.settlement(ctx, id)
		if err != nil {
			return 0, err
		}
		largest = max(largest, t.Amount)
	}
	return largest, nil
}
