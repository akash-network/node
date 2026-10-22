package keeper

import (
	"bytes"
	"encoding/binary"
	"fmt"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

// refundQueuePrefix indexes refundable transfers: voided with no hold. Keys
// order by challenge end, then ID, so the head is the oldest matured refund.
// Only the head may be paid, which stops a small refund from starving a
// cap-sized one at the start of a day. Like open/, the index is derived:
// InitGenesis rebuilds it and export omits it.
var refundQueuePrefix = []byte("refundq/")

func refundQueueKey(challengeEnd int64, id uint64) []byte {
	key := binary.BigEndian.AppendUint64(bytes.Clone(refundQueuePrefix), uint64(challengeEnd)) //nolint:gosec // positive for a void
	return binary.BigEndian.AppendUint64(key, id)
}

func queued(s types.Settlement) bool {
	return s.Status == types.Status_STATUS_VOIDED && s.Hold == types.Hold_HOLD_NONE
}

// putSettlement stores a settlement and keeps the refund queue in step.
// before is the settlement as loaded, after the one to store.
func (k Keeper) putSettlement(ctx sdk.Context, id uint64, before, after types.Settlement) {
	store := ctx.KVStore(k.key)
	if queued(before) {
		store.Delete(refundQueueKey(before.ChallengeEnd, id))
	}
	if queued(after) {
		store.Set(refundQueueKey(after.ChallengeEnd, id), openValue)
	}
	k.setSettlement(ctx, id, after)
}

// refundQueueHead returns the transfer that must be refunded next.
func (k Keeper) refundQueueHead(ctx sdk.Context) (uint64, bool, error) {
	it := storetypes.KVStorePrefixIterator(ctx.KVStore(k.key), refundQueuePrefix)
	defer func() { _ = it.Close() }()
	if !it.Valid() {
		return 0, false, nil
	}
	if len(it.Key()) != len(refundQueuePrefix)+16 {
		return 0, false, fmt.Errorf("malformed sink refund queue key")
	}
	return binary.BigEndian.Uint64(it.Key()[len(refundQueuePrefix)+8:]), true, nil
}

// largestOpenAmount is the largest transfer that is neither minted nor
// refunded, so a lowered refund cap never strands an open transfer.
func (k Keeper) largestOpenAmount(ctx sdk.Context) (uint64, error) {
	it := storetypes.KVStorePrefixIterator(ctx.KVStore(k.key), openPrefix)
	defer func() { _ = it.Close() }()
	var largest uint64
	for ; it.Valid(); it.Next() {
		if len(it.Key()) != len(openPrefix)+8 {
			return 0, fmt.Errorf("malformed open sink transfer key")
		}
		t, _, err := k.settlement(ctx, binary.BigEndian.Uint64(it.Key()[len(openPrefix):]))
		if err != nil {
			return 0, err
		}
		largest = max(largest, t.Amount)
	}
	return largest, nil
}
