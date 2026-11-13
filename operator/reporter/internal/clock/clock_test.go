package clock_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/internal/clock"
)

func TestFakeClock(t *testing.T) {
	start := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	fake := clock.NewFake(start)

	require.Equal(t, start, fake.Now())

	ch := fake.After(5 * time.Second)

	fake.Advance(4 * time.Second)
	require.Equal(t, start.Add(4*time.Second), fake.Now())
	select {
	case <-ch:
		t.Fatal("timer fired before its deadline")
	default:
	}

	fake.Advance(time.Second)
	select {
	case fired := <-ch:
		require.Equal(t, start.Add(5*time.Second), fired)
	default:
		t.Fatal("timer did not fire after Advance(5s)")
	}
}

func TestFakeClockNonPositiveDelayFiresImmediately(t *testing.T) {
	fake := clock.NewFake(time.Unix(100, 0))

	select {
	case <-fake.After(0):
	default:
		t.Fatal("After(0) did not fire immediately")
	}
}

func TestRealClock(t *testing.T) {
	c := clock.Real()

	before := time.Now()
	require.False(t, c.Now().Before(before))

	select {
	case <-c.After(time.Millisecond):
	case <-time.After(5 * time.Second):
		t.Fatal("real After did not fire")
	}
}
