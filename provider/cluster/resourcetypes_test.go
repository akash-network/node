package cluster

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	types "github.com/ovrclk/akash/types/v1beta2"
)

func TestResourcePairAvailable(t *testing.T) {
	rp := &resourcePair{
		allocatable: sdk.NewInt(100),
		allocated:   sdk.NewInt(0),
	}

	avail := rp.available()

	require.Equal(t, int64(100), avail.Int64())

	rp = &resourcePair{
		allocatable: sdk.NewInt(100),
		allocated:   sdk.NewInt(100),
	}

	avail = rp.available()

	require.Equal(t, int64(0), avail.Int64())
}

func TestResourcePairSubNLZ(t *testing.T) {
	rp := &resourcePair{
		allocatable: sdk.NewInt(100),
		allocated:   sdk.NewInt(0),
	}

	adjusted := rp.subNLZ(types.NewResourceValue(0))
	require.True(t, adjusted)

	avail := rp.available()
	require.Equal(t, int64(100), avail.Int64())

	adjusted = rp.subNLZ(types.NewResourceValue(9))
	require.True(t, adjusted)

	avail = rp.available()
	require.Equal(t, int64(91), avail.Int64())

	adjusted = rp.subNLZ(types.NewResourceValue(92))
	require.False(t, adjusted)
}
