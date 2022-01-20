package operatorcommon

import (
	"bytes"
	"encoding/json"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
	"time"
)

func TestIgnoreList(t *testing.T) {
	start := time.Now()
	il := NewIgnoreList(IgnoreListConfig{
		FailureLimit: 1,
		EntryLimit:   100,
		AgeLimit:     time.Hour,
	})

	cnt := testutil.RandRangeInt(101, 1000)
	for i := 0; i != cnt; i++ {
		lid := testutil.LeaseID(t)
		require.False(t, il.IsFlagged(lid))
		il.AddError(lid, io.EOF)
		require.True(t, il.IsFlagged(lid))
	}

	require.Greater(t, il.Size(), 100)

	lid := testutil.LeaseID(t)
	require.False(t, il.IsFlagged(lid))

	require.True(t, il.Prune())

	require.Equal(t, 100, il.Size())

	pd := newPreparedResult()
	require.NoError(t, il.Prepare(pd))

	data := pd.get()
	require.Greater(t, data.preparedAt.UnixNano(), start.UnixNano())

	dec := json.NewDecoder(bytes.NewReader(data.data))
	var output map[string]interface{}
	require.NoError(t, dec.Decode(&output))
	require.Len(t, output, 100)
}
