package kube

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"

	types "github.com/ovrclk/akash/types/v1beta2"
)

func TestResourcePairAvailable(t *testing.T) {
	rp := &resourcePair{
		allocatable: *resource.NewQuantity(100, resource.DecimalSI),
		allocated:   *resource.NewQuantity(0, resource.DecimalSI),
	}

	avail := rp.available()

	require.Equal(t, int64(100), avail.Value())

	rp = &resourcePair{
		allocatable: *resource.NewQuantity(100, resource.DecimalSI),
		allocated:   *resource.NewQuantity(100, resource.DecimalSI),
	}

	avail = rp.available()

	require.Equal(t, int64(0), avail.Value())
}

func TestResourcePairSubNLZ(t *testing.T) {
	rp := &resourcePair{
		allocatable: *resource.NewQuantity(100, resource.DecimalSI),
		allocated:   *resource.NewQuantity(0, resource.DecimalSI),
	}

	adjusted := rp.subNLZ(types.NewResourceValue(0))
	require.True(t, adjusted)

	avail := rp.available()
	require.Equal(t, int64(100), avail.Value())

	adjusted = rp.subNLZ(types.NewResourceValue(9))
	require.True(t, adjusted)

	avail = rp.available()
	require.Equal(t, int64(91), avail.Value())

	adjusted = rp.subNLZ(types.NewResourceValue(92))
	require.False(t, adjusted)
}

func TestResourcePairSubMilliNLZ(t *testing.T) {
	rp := &resourcePair{
		allocatable: *resource.NewQuantity(10000, resource.DecimalSI),
		allocated:   *resource.NewQuantity(0, resource.DecimalSI),
	}

	adjusted := rp.subMilliNLZ(types.NewResourceValue(0))
	require.True(t, adjusted)

	avail := rp.available()
	require.Equal(t, int64(10000), avail.Value())

	adjusted = rp.subNLZ(types.NewResourceValue(9))
	require.True(t, adjusted)

	avail = rp.available()
	require.Equal(t, int64(9991), avail.Value())
}
