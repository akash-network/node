package keys_test

//import (
//	"testing"
//
//	"github.com/stretchr/testify/require"
//
//	_ "pkg.akt.dev/node/testutil"
//
//	"pkg.akt.dev/go/node/market/v1"
//	"pkg.akt.dev/go/node/market/v1beta5"
//
//	"pkg.akt.dev/node/x/market/keeper/keys"
//)
//
//func TestKeysAndSecondaryKeysFilter(t *testing.T) {
//	filter := v1.LeaseFilters{
//		Owner:    "akash104fq56d9attl4m709h7mgx9lwqklnh05fhy5nu",
//		DSeq:     1,
//		GSeq:     2,
//		OSeq:     3,
//		Provider: "akash1vlaa09ytnl0hvu04wgs0d6zw5n6anjc3allk49",
//		State:    v1.LeaseClosed.String(),
//	}
//
//	prefix, isSecondary, err := keys.LeasePrefixFromFilter(filter)
//	require.NoError(t, err)
//	require.False(t, isSecondary)
//	require.Equal(t, v1beta5.LeasePrefix(), prefix[0:2])
//
//	filter.Owner = ""
//	prefix, isSecondary, err = keys.LeasePrefixFromFilter(filter)
//	require.NoError(t, err)
//	require.False(t, isSecondary)
//	require.Equal(t, v1beta5.LeasePrefix(), prefix[0:2])
//}
