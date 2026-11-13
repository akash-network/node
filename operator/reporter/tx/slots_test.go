package tx

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/stretchr/testify/require"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

// scaleTiming multiplies every duration of the nonce scheme by factor for the rest of the test.
func scaleTiming(t *testing.T, factor float64) {
	t.Helper()

	saved := timing
	scale := func(d time.Duration) time.Duration { return time.Duration(float64(d) * factor) }
	timing = timingValues{
		window:     scale(saved.window),
		spacing:    scale(saved.spacing),
		cutoffBase: scale(saved.cutoffBase),
		flushLead:  scale(saved.flushLead),
	}

	t.Cleanup(func() { timing = saved })
}

func vectorID(t *testing.T) sinkid.ID {
	t.Helper()

	owner, err := bech32.ConvertAndEncode("akash", bytes.Repeat([]byte{0x01}, 20))
	require.NoError(t, err)

	id, err := sinkid.Parse(fmt.Sprintf("%s/42/7", owner))
	require.NoError(t, err)

	return id
}

func TestSlotVectors(t *testing.T) {
	id := vectorID(t)
	vote := policy.Key{ID: id.String(), Kind: policy.MsgVote, Outcome: sinkv1.Outcome(1), Round: 1, PolicyVersion: 2}

	require.Equal(t, 30, SlotOf(id, vote))

	vote.Outcome = sinkv1.Outcome(2)
	require.Equal(t, 73, SlotOf(id, vote))
	require.Equal(t, 217, SlotOf(id, policy.Key{ID: id.String(), Kind: policy.MsgVeto}))
	require.Equal(t, 613, SlotOf(id, policy.Key{ID: id.String(), Kind: policy.MsgRefund}))

	w := WindowOf(time.Unix(1_800_000_005, 0))
	require.Equal(t, int64(15_000_000), w)

	at := func(sec, ms int) time.Time {
		return time.Date(2027, 1, 15, 8, 1, sec, ms*int(time.Millisecond), time.UTC)
	}

	for slot, want := range map[int]time.Time{
		0:    at(59, 999),
		127:  at(59, 872),
		128:  at(59, 871),
		1151: at(58, 848),
	} {
		got := SlotTimeout(w, slot)
		require.Equal(t, want, got, "slot %d", slot)
		require.Equal(t, time.UTC, got.Location(), "slot %d", slot)
	}

	require.Equal(t, time.UTC, WindowStart(w).Location())
}

func TestSlotRanges(t *testing.T) {
	id := vectorID(t)

	for round := uint64(0); round < 512; round++ {
		vote := SlotOf(id, policy.Key{ID: id.String(), Kind: policy.MsgVote, Outcome: sinkv1.Outcome(1), Round: round, PolicyVersion: 1})
		require.GreaterOrEqual(t, vote, 0)
		require.Less(t, vote, VoteSlots)
	}

	for height := int64(1); height < 512; height++ {
		other, err := sinkid.Parse(fmt.Sprintf("%s/%d/1", id.Owner, height))
		require.NoError(t, err)

		for _, kind := range []policy.MsgKind{policy.MsgVeto, policy.MsgRefund} {
			slot := SlotOf(other, policy.Key{ID: other.String(), Kind: kind})
			require.GreaterOrEqual(t, slot, VoteSlots)
			require.Less(t, slot, VoteSlots+SingleSlots)
		}
	}
}

func TestSlotTimeoutsDistinct(t *testing.T) {
	const w = int64(15_000_000)

	end := WindowStart(w + 1)
	earliest := end.Add(-1152 * time.Millisecond)
	seen := make(map[time.Time]int, VoteSlots+SingleSlots)

	for s := 0; s < VoteSlots+SingleSlots; s++ {
		timeout := SlotTimeout(w, s)
		require.False(t, timeout.Before(earliest), "slot %d", s)
		require.True(t, timeout.Before(end), "slot %d", s)

		prev, dup := seen[timeout]
		require.False(t, dup, "slots %d and %d share a timeout", prev, s)
		seen[timeout] = s
	}

	require.Len(t, seen, 1152)
}

func TestSlotsUseBlockTime(t *testing.T) {
	base := time.Date(2027, 1, 15, 8, 0, 0, 0, time.UTC)
	tokyo := time.FixedZone("UTC+9", 9*60*60)

	for _, bt := range []time.Time{
		base,
		base.Add(time.Nanosecond),
		base.Add(2*time.Minute - time.Nanosecond),
		base.Add(time.Minute + 1234567),
		base.In(tokyo),
		time.Unix(0, 0),
		time.Unix(-1, 0),
		time.Unix(1_800_000_005, 0),
	} {
		w := WindowOf(bt)
		require.Equal(t, w, WindowOf(bt.UTC()), bt)
		require.Equal(t, w, WindowOf(bt), bt)
		require.False(t, bt.Before(WindowStart(w)), bt)
		require.True(t, bt.Before(WindowStart(w+1)), bt)

		for _, s := range []int{0, 127, 128, 1151} {
			require.Less(t, SlotTimeout(w, s).Sub(bt), 2*time.Minute, "%s slot %d", bt, s)
		}
	}

	require.Equal(t, int64(-1), WindowOf(time.Unix(-1, 0)))
}

func TestSendCutoff(t *testing.T) {
	require.Equal(t, 20*time.Second, SendCutoff(0))
	require.Equal(t, 30*time.Second, SendCutoff(10*time.Second))
}

func TestScaleTiming(t *testing.T) {
	t.Run("scaled", func(t *testing.T) {
		scaleTiming(t, 0.5)
		require.Equal(t, time.Minute, timing.window)
		require.Equal(t, 10*time.Second, SendCutoff(0))
		require.Equal(t, WindowStart(1).Add(-500*time.Microsecond), SlotTimeout(0, 0))
	})

	require.Equal(t, 2*time.Minute, timing.window)
	require.Equal(t, time.Millisecond, timing.spacing)
	require.Equal(t, 20*time.Second, timing.cutoffBase)
	require.Equal(t, 10*time.Second, timing.flushLead)
}
