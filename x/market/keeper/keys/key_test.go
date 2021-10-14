package keys_test

import (
	_ "github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/market/keeper/keys"
	types "github.com/ovrclk/akash/x/market/types/v1beta2"

	"github.com/stretchr/testify/require"
	"testing"
)

func TestKeysAndSecondaryKeysFilter(t *testing.T) {

	filter := types.LeaseFilters{
		Owner:    "akash104fq56d9attl4m709h7mgx9lwqklnh05fhy5nu",
		DSeq:     1,
		GSeq:     2,
		OSeq:     3,
		Provider: "akash1vlaa09ytnl0hvu04wgs0d6zw5n6anjc3allk49",
		State:    types.LeaseClosed.String(),
	}

	prefix, isSecondary, err := keys.LeasePrefixFromFilter(filter)
	require.NoError(t, err)
	require.False(t, isSecondary)
	require.Equal(t, types.LeasePrefix(), prefix[0:2])

	filter.Owner = ""
	prefix, isSecondary, err = keys.LeasePrefixFromFilter(filter)
	require.NoError(t, err)
	require.True(t, isSecondary)
	require.Equal(t, types.SecondaryLeasePrefix(), prefix[0:2])
}
