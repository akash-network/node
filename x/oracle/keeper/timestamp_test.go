package keeper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeTimestamp_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		time time.Time
	}{
		{
			name: "zero subseconds",
			time: time.Date(2025, 3, 15, 10, 30, 45, 0, time.UTC),
		},
		{
			name: "full nanosecond precision",
			time: time.Date(2026, 12, 31, 23, 59, 59, 123_456_789, time.UTC),
		},
		{
			name: "unix epoch",
			time: time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "milliseconds only",
			time: time.Date(2024, 6, 15, 12, 0, 0, 500_000_000, time.UTC),
		},
		{
			name: "microseconds only",
			time: time.Date(2024, 6, 15, 12, 0, 0, 123_000, time.UTC),
		},
		{
			name: "nanoseconds only",
			time: time.Date(2024, 6, 15, 12, 0, 0, 456, time.UTC),
		},
		{
			name: "non-UTC converted",
			time: time.Date(2025, 1, 1, 5, 30, 0, 0, time.FixedZone("IST", 5*3600+1800)),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := EncodeTimestamp(tc.time)
			require.Len(t, buf, TimestampEncodedSize)

			decoded, err := DecodeTimestamp(buf)
			require.NoError(t, err)

			// EncodeTimestamp converts to UTC, so compare in UTC
			assert.True(t, tc.time.UTC().Equal(decoded), "want %v, got %v", tc.time.UTC(), decoded)
		})
	}
}

func TestEncodeTimestamp_Format(t *testing.T) {
	ts := time.Date(2025, 3, 5, 8, 9, 7, 12_034_056, time.UTC)
	buf := EncodeTimestamp(ts)
	assert.Equal(t, "2025.03.05.08.09.07.012.034.056", string(buf))
}

func TestEncodeTimestamp_LexicographicOrder(t *testing.T) {
	times := []time.Time{
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 1, 0, 0, 0, 1, time.UTC),
		time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	for i := 1; i < len(times); i++ {
		a := string(EncodeTimestamp(times[i-1]))
		b := string(EncodeTimestamp(times[i]))
		assert.Less(t, a, b, "expected %q < %q", a, b)
	}
}

func TestDecodeTimestamp_BufferTooShort(t *testing.T) {
	_, err := DecodeTimestamp([]byte("short"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}
