package keeper

import (
	"errors"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	mv1 "pkg.akt.dev/go/node/market/v1"

	"pkg.akt.dev/node/v2/x/market/keeper/keys"
)

const providerLeaseStatsCompletedReason int32 = -1

func providerLeaseStatsReasonKey(reason mv1.LeaseClosedReason) int32 {
	if !reason.IsRange(mv1.LeaseClosedReasonRangeProvider) {
		return providerLeaseStatsCompletedReason
	}
	return int32(reason)
}

func (k Keeper) incrementProviderLeaseStats(ctx sdk.Context, provider string, reason mv1.LeaseClosedReason) error {
	key := collections.Join(provider, providerLeaseStatsReasonKey(reason))
	count, err := k.leaseStats.Get(ctx, key)
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return err
	}

	return k.leaseStats.Set(ctx, key, count+1)
}

// GetProviderLeaseStats returns aggregate completed leases and provider-fault
// close counts keyed by close reason. Chain SDK query/proto support for this
// local keeper data does not exist yet.
func (k Keeper) GetProviderLeaseStats(ctx sdk.Context, provider sdk.Address) (uint64, map[mv1.LeaseClosedReason]uint64, bool) {
	failures := make(map[mv1.LeaseClosedReason]uint64)
	var completed uint64
	found := false

	prefix := collections.NewPrefixedPairRange[string, int32](provider.String())
	err := k.leaseStats.Walk(ctx, prefix, func(key keys.ProviderLeaseStatsKey, count uint64) (bool, error) {
		found = true
		reason := key.K2()
		if reason == providerLeaseStatsCompletedReason {
			completed += count
			return false, nil
		}
		failures[mv1.LeaseClosedReason(reason)] += count
		return false, nil
	})
	if err != nil {
		return 0, nil, false
	}

	return completed, failures, found
}
