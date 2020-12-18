package util_test

import (
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/cluster/util"
	atypes "github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestShouldBeIngress(t *testing.T) {
	// Should not create ingress for something on port 81
	require.False(t, util.ShouldBeIngress(manifest.ServiceExpose{
		Global: true,
		Proto:  manifest.TCP,
		Port:   81,
	}))

	// Should create ingress for something on port 80
	require.True(t, util.ShouldBeIngress(manifest.ServiceExpose{
		Global: true,
		Proto:  manifest.TCP,
		Port:   80,
	}))

	// Should not create ingress for something on port 80 that is not Global
	require.False(t, util.ShouldBeIngress(manifest.ServiceExpose{
		Global: false,
		Proto:  manifest.TCP,
		Port:   80,
	}))

	// Should not create ingress for something on port 80 that is UDP
	require.False(t, util.ShouldBeIngress(manifest.ServiceExpose{
		Global: true,
		Proto:  manifest.UDP,
		Port:   80,
	}))
}

func TestComputeCommittedResources(t *testing.T) {

	rv := atypes.NewResourceValue(100)
	// Negative factor returns original value
	require.Equal(t, uint64(100), util.ComputeCommittedResources(-1.0, rv).Val.Uint64())

	// Zero factor returns original value
	require.Equal(t, uint64(100), util.ComputeCommittedResources(0.0, rv).Val.Uint64())

	// Factor of one returns the original value
	require.Equal(t, uint64(100), util.ComputeCommittedResources(1.0, rv).Val.Uint64())

	require.Equal(t, uint64(50), util.ComputeCommittedResources(2.0, rv).Val.Uint64())

	require.Equal(t, uint64(33), util.ComputeCommittedResources(3.0, rv).Val.Uint64())

	// Even for huge overcommit values, zero is not returned
	require.Equal(t, uint64(1), util.ComputeCommittedResources(10000.0, rv).Val.Uint64())
}
