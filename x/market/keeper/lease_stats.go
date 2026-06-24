package keeper

import (
	"errors"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	mv1 "pkg.akt.dev/go/node/market/v1"

	"pkg.akt.dev/node/v3/x/market/keeper/keys"
)

const (
	providerLeaseStatsBackfillBucket        int64 = 0
	providerLeaseStatsLegacyCompletedReason int32 = -1
)

var providerLeaseStatsLegacyKeyCodec = collections.PairKeyCodec(collections.StringKey, collections.Int32Key)

func NewProviderLeaseStatsMap(sb *collections.SchemaBuilder) collections.Map[keys.ProviderLeaseStatsKey, uint64] {
	return collections.NewMap(sb, collections.NewPrefix(keys.ProviderLeaseStatsPrefix), "provider_lease_stats", keys.ProviderLeaseStatsKeyCodec, collections.Uint64Value)
}

func providerLeaseStatsReasonKey(reason mv1.LeaseClosedReason) (int32, bool) {
	if reason == mv1.LeaseClosedReasonInvalid {
		return 0, false
	}
	if reason.IsRange(mv1.LeaseClosedReasonRangeOwner) ||
		reason.IsRange(mv1.LeaseClosedReasonRangeProvider) ||
		reason.IsRange(mv1.LeaseClosedReasonRangeNetwork) {
		return int32(reason), true
	}
	return 0, false
}

func (k Keeper) incrementProviderLeaseStats(ctx sdk.Context, provider string, reason mv1.LeaseClosedReason) error {
	return incrementProviderLeaseStats(ctx, k.leaseStats, provider, providerLeaseStatsBucket(ctx.BlockTime()), reason)
}

func providerLeaseStatsBucket(blockTime time.Time) int64 {
	if blockTime.IsZero() {
		return providerLeaseStatsBackfillBucket
	}
	return blockTime.UTC().Unix()
}

func incrementProviderLeaseStats(ctx sdk.Context, leaseStats collections.Map[keys.ProviderLeaseStatsKey, uint64], provider string, bucket int64, reason mv1.LeaseClosedReason) error {
	reasonKey, ok := providerLeaseStatsReasonKey(reason)
	if !ok {
		return nil
	}

	key := collections.Join3(provider, bucket, reasonKey)
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

		return false, incrementProviderLeaseStats(ctx, leaseStats, lease.ID.Provider, providerLeaseStatsBackfillBucket, lease.Reason)
	})
}

func (k Keeper) BackfillProviderLeaseStats(ctx sdk.Context) error {
	return BackfillProviderLeaseStats(ctx, k.leases, k.leaseStats)
}

type providerLeaseStatsResult struct {
	completed         uint64
	completedByReason map[mv1.LeaseClosedReason]uint64
	failures          map[mv1.LeaseClosedReason]uint64
	found             bool
}

type providerLeaseStatsDecodedKey struct {
	provider        string
	bucket          int64
	reason          mv1.LeaseClosedReason
	legacy          bool
	legacyCompleted bool
}

// GetProviderLeaseStats returns completed leases and provider-fault close
// counts keyed by close reason. Since is inclusive; zero includes all retained
// buckets.
func (k Keeper) GetProviderLeaseStats(ctx sdk.Context, provider sdk.Address, since time.Time) (uint64, map[mv1.LeaseClosedReason]uint64, bool) {
	stats := k.getProviderLeaseStats(ctx, provider, since)
	return stats.completed, stats.failures, stats.found
}

func (k Keeper) getProviderLeaseStats(ctx sdk.Context, provider sdk.Address, since time.Time) providerLeaseStatsResult {
	stats := providerLeaseStatsResult{
		completedByReason: make(map[mv1.LeaseClosedReason]uint64),
		failures:          make(map[mv1.LeaseClosedReason]uint64),
	}
	sinceBucket := providerLeaseStatsBucket(since)
	providerString := provider.String()

	store := prefix.NewStore(ctx.KVStore(k.skey), keys.ProviderLeaseStatsPrefix)
	iter := store.Iterator(nil, nil)
	for ; iter.Valid(); iter.Next() {
		key, ok := decodeProviderLeaseStatsKey(iter.Key())
		if !ok || key.provider != providerString {
			continue
		}

		if !since.IsZero() {
			if key.legacy || key.bucket < sinceBucket {
				continue
			}
		}

		count, err := collections.Uint64Value.Decode(iter.Value())
		if err != nil {
			_ = iter.Close()
			return providerLeaseStatsResult{
				completedByReason: make(map[mv1.LeaseClosedReason]uint64),
				failures:          make(map[mv1.LeaseClosedReason]uint64),
			}
		}

		stats.found = true
		if key.legacyCompleted {
			stats.completed += count
		} else if key.reason.IsRange(mv1.LeaseClosedReasonRangeProvider) {
			stats.failures[key.reason] += count
		} else {
			stats.completed += count
			stats.completedByReason[key.reason] += count
		}
	}
	if err := iter.Close(); err != nil {
		return providerLeaseStatsResult{
			completedByReason: make(map[mv1.LeaseClosedReason]uint64),
			failures:          make(map[mv1.LeaseClosedReason]uint64),
		}
	}

	return stats
}

func decodeProviderLeaseStatsKey(raw []byte) (providerLeaseStatsDecodedKey, bool) {
	read, key, err := keys.ProviderLeaseStatsKeyCodec.Decode(raw)
	if err == nil && read == len(raw) {
		return providerLeaseStatsDecodedKey{
			provider: key.K1(),
			bucket:   key.K2(),
			reason:   mv1.LeaseClosedReason(key.K3()),
		}, true
	}

	read, legacyKey, err := providerLeaseStatsLegacyKeyCodec.Decode(raw)
	if err != nil || read != len(raw) {
		return providerLeaseStatsDecodedKey{}, false
	}

	reason := legacyKey.K2()
	return providerLeaseStatsDecodedKey{
		provider:        legacyKey.K1(),
		bucket:          providerLeaseStatsBackfillBucket,
		reason:          mv1.LeaseClosedReason(reason),
		legacy:          true,
		legacyCompleted: reason == providerLeaseStatsLegacyCompletedReason,
	}, true
}
