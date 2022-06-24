package operatorcommon

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPreparedData(t *testing.T) {
	start := time.Now()
	pr := newPreparedResult()
	require.Len(t, pr.get().data, 0)
	require.Greater(t, pr.get().preparedAt.UnixNano(), int64(0))

	require.False(t, pr.needsPrepare)
	pr.Flag()
	require.True(t, pr.needsPrepare)

	testData := []byte{0x33, 0x44, 0xff}

	pr.Set(testData)

	require.Equal(t, pr.get().data, testData)
	require.Greater(t, pr.get().preparedAt.UnixNano(), start.UnixNano())
}

func TestPrepraedDataTruncates(t *testing.T) {
	pr := newPreparedResult()

	const l = 10000000
	data := make([]byte, l) // only length matters
	pr.Set(data)

	require.Less(t, len(pr.get().data), l)
}
