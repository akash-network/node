package relayer

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// This coherent fixture skips two extra slots immediately after the required
// window end. The complete window and its signed blockhash stay unchanged.
func expiryLocatorGap(t *testing.T) *expiryFixture {
	t.Helper()
	f := newExpiryFixture(t, "proof-append")
	f.anchor = 1036
	slotAt := func(height uint64) uint64 {
		slot := expiryTestSlot(height)
		if height > 1002 {
			slot += 2
		}
		return slot
	}
	f.mutate = func(method string, params []json.RawMessage, result any) any {
		switch method {
		case "getSlot":
			return slotAt(f.anchor)
		case "getBlocks":
			var start, end uint64
			require.NoError(t, json.Unmarshal(params[0], &start))
			require.NoError(t, json.Unmarshal(params[1], &end))
			slots := []uint64{}
			for height := uint64(1); height <= f.anchor; height++ {
				if slot := slotAt(height); slot >= start && slot <= end {
					slots = append(slots, slot)
				}
			}
			return slots
		case "getBlock":
			var slot uint64
			require.NoError(t, json.Unmarshal(params[0], &slot))
			require.NotEqual(t, uint64(3006), slot, "skipped slot must not be fetched")
			height := (slot - 1000) / 2
			if slot > 3006 {
				height--
			}
			block := f.block(height)
			block["parentSlot"] = slotAt(height - 1)
			return block
		case "getSignatureStatuses", "isBlockhashValid":
			fields := result.(map[string]any)
			fields["context"] = map[string]any{"slot": slotAt(f.anchor)}
		}
		return result
	}
	return f
}

func TestExpiryLocatorSkippedRangeRecovery(t *testing.T) {
	f := expiryLocatorGap(t)
	saved, err := f.service.store.snapshotPending(f.path)
	require.NoError(t, err)
	err = f.resume(context.Background())
	require.True(t, isIntentNotIncluded(err), "%v", err)
	cert, err := f.service.store.loadExpiryCertificate(saved)
	require.NoError(t, err)
	require.NotNil(t, cert)
	empty := -1
	for i, step := range cert.Locator {
		if len(step.Slots) == 0 {
			empty = i
			require.True(t, reflect.DeepEqual(step.Block, expiryBlock{}))
		}
	}
	require.GreaterOrEqual(t, empty, 0, "regression must exercise an empty locator range")
	require.NoError(t, validateExpiryCertificate(saved, cert))
	require.Zero(t, f.observed)
	require.Zero(t, f.calls["sendTransaction"])

	for _, mutation := range []string{"block", "bounds", "null", "unfinished"} {
		t.Run(mutation, func(t *testing.T) {
			data, err := json.Marshal(cert)
			require.NoError(t, err)
			var changed nonInclusionCertificate
			require.NoError(t, json.Unmarshal(data, &changed))
			switch mutation {
			case "block":
				changed.Locator[empty].Block = cert.Blocks[0]
			case "bounds":
				changed.Locator[empty].End++
			case "null":
				changed.Locator[empty].Slots = nil
			case "unfinished":
				changed.Locator = changed.Locator[:empty+1]
			}
			require.Error(t, validateExpiryCertificate(saved, &changed))
		})
	}

	// Recreate only this synthetic interrupted unlink, then exercise cached
	// certification against a newer coherent anchor without another scan.
	require.NoError(t, os.WriteFile(f.path, saved.Original, 0o600))
	scans := f.calls["getBlocks"]
	f.mu.Lock()
	f.anchor++
	f.mu.Unlock()
	require.True(t, isIntentNotIncluded(f.resume(context.Background())))
	require.Equal(t, scans, f.calls["getBlocks"])
}

func TestExpiryLocatorEmptyRangeIsOnlySearchHint(t *testing.T) {
	for _, response := range []string{"null", "all-empty", "omitted-member"} {
		t.Run(response, func(t *testing.T) {
			f := expiryLocatorGap(t)
			original := f.mutate
			f.mutate = func(method string, params []json.RawMessage, result any) any {
				result = original(method, params, result)
				if method == "getBlocks" {
					switch response {
					case "null":
						return nil
					case "all-empty":
						return []uint64{}
					case "omitted-member":
						slots := result.([]uint64)
						filtered := []uint64{}
						for _, slot := range slots {
							if slot != expiryTestSlot(900) {
								filtered = append(filtered, slot)
							}
						}
						return filtered
					}
				}
				if response == "omitted-member" && method == "getBlock" {
					var slot uint64
					require.NoError(t, json.Unmarshal(params[0], &slot))
					if slot == expiryTestSlot(900) {
						result.(map[string]any)["signatures"] = []string{f.doc.Signature}
					}
				}
				return result
			}
			err := f.retained(context.Background())
			if response == "omitted-member" {
				require.ErrorContains(t, err, "original signature included")
			}
			require.LessOrEqual(t, f.calls["getBlocks"], 18)
		})
	}
}
