package util_test

import (
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/cluster/util"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestShouldExpose(t *testing.T) {
	// Should not create ingress for something on port 81
	require.False(t, util.ShouldExpose(&manifest.ServiceExpose{
		Global: true,
		Proto:  manifest.TCP,
		Port:   81,
	}))

	// Should create ingress for something on port 80
	require.True(t, util.ShouldExpose(&manifest.ServiceExpose{
		Global: true,
		Proto:  manifest.TCP,
		Port:   80,
	}))

	// Should not create ingress for something on port 80 that is not Global
	require.False(t, util.ShouldExpose(&manifest.ServiceExpose{
		Global: false,
		Proto:  manifest.TCP,
		Port:   80,
	}))

	// Should not create ingress for something on port 80 that is UDP
	require.False(t, util.ShouldExpose(&manifest.ServiceExpose{
		Global: true,
		Proto:  manifest.UDP,
		Port:   80,
	}))
}
