package pubsub

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	defaultDelayThreadStart = time.Millisecond * 6
)

// AfterThreadStart waits for the duration of delay thread start
func AfterThreadStart(t *testing.T) <-chan time.Time {
	return time.After(delayThreadStart(t))
}

// SleepForThreadStart pass go routine for the duration of delay thread start
func SleepForThreadStart(t *testing.T) {
	time.Sleep(delayThreadStart(t))
}

func delayThreadStart(t *testing.T) time.Duration {
	if val := os.Getenv("TEST_DELAY_THREAD_START"); val != "" {
		d, err := time.ParseDuration(val)
		require.NoError(t, err)

		return d
	}

	return defaultDelayThreadStart
}
