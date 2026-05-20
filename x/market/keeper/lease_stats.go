package keeper

import (
	"errors"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	mv1 "pkg.akt.dev/go/node/market/v1"

	"pkg.akt.dev/node/v2/x/market/keeper/keys"
)

const providerLeaseStatsCompletedReason int32 = -1

func NewProviderLeaseStatsMap(sb *collections.SchemaBuilder) collections.Map[keys.ProviderLeaseStatsKey, uint64] {
	return collections.NewMap(sb, collections.NewPrefix(keys.ProviderLeaseStatsPrefix), "provider_lease_stats", keys.ProviderLeaseStatsKeyCodec, collections.Uint64Value)
}

func providerLeaseStatsReasonKey(reason mv1.LeaseClosedReason) int32 {
	if !reason.IsRange(mv1.LeaseClosedReasonRangeProvider) {
		return providerLeaseStatsCompletedReason
	}
	return int32(reason)
}

func (k Keeper) incrementProviderLeaseStats(ctx sdk.Context, provider string, reason mv1.LeaseClosedReason) error {
	return incrementProviderLeaseStats(ctx, k.leaseStats, provider, reason)
}

func incrementProviderLeaseStats(ctx sdk.Context, leaseStats collections.Map[keys.ProviderLeaseStatsKey, uint64], provider string, reason mv1.LeaseClosedReason) error {
	key := collections.Join(provider, providerLeaseStatsReasonKey(reason))
	count, err := leaseStats.Get(ctx, key)
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return err
	}

	return leaseStats.Set(ctx, key, count+1)
}

func clearProviderLeaseStats(ctx sdk.Context, leaseStats collections.Map[keys.ProviderLeaseStatsKey, uint64]) error {
	var statKeys []keys.ProviderLeaseStatsKey
	if err := leaseStats.Walk(ctx, nil, func(key keys.ProviderLeaseStatsKey, _ uint64) (bool, error) {
		statKeys = append(statKeys, key)
		return false, nil
	}); err != nil {
		return err
	}

	for _, key := range statKeys {
		if err := leaseStats.Remove(ctx, key); err != nil {
			return err
		}
	}

	return nil
}

func isProviderLeaseStatsTerminalState(state mv1.Lease_State) bool {
	return state == mv1.LeaseClosed || state == mv1.LeaseInsufficientFunds
}

func BackfillProviderLeaseStats(
	ctx sdk.Context,
	leases *collections.IndexedMap[keys.LeasePrimaryKey, mv1.Lease, LeaseIndexes],
	leaseStats collections.Map[keys.ProviderLeaseStatsKey, uint64],
) error {
	if err := clearProviderLeaseStats(ctx, leaseStats); err != nil {
		return err
	}

	return leases.Walk(ctx, nil, func(_ keys.LeasePrimaryKey, lease mv1.Lease) (bool, error) {
		if !isProviderLeaseStatsTerminalState(lease.State) {
			return false, nil
		}

		return false, incrementProviderLeaseStats(ctx, leaseStats, lease.ID.Provider, lease.Reason)
	})
}

func (k Keeper) BackfillProviderLeaseStats(ctx sdk.Context) error {
	return BackfillProviderLeaseStats(ctx, k.leases, k.leaseStats)
}

// GetProviderLeaseStats returns aggregate completed leases and provider-fault
// close counts keyed by close reason.
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
