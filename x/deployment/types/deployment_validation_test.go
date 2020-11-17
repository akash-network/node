package types_test


import (
	"testing"

	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/require"
	"github.com/ovrclk/akash/x/deployment/types"
)

func TestZeroValueGroupSpec(t *testing.T) {
	did := testutil.DeploymentID(t)

	dgroup := testutil.DeploymentGroup(t, did, uint32(6))
	gspec := dgroup.GroupSpec

	t.Run("assert nominal test success", func(t *testing.T) {
		err := gspec.ValidateBasic()
		require.NoError(t, err)
	})

	gspec.OrderBidDuration = int64(0)
	t.Run("assert error for zero value bid duration", func(t *testing.T) {
		err := gspec.ValidateBasic()
		require.Error(t, err)
	})
}

func TestZeroValueGroupSpecs(t *testing.T) {
	did := testutil.DeploymentID(t)
	dgroups := testutil.DeploymentGroups(t, did, uint32(6))
	gspecs := make([]types.GroupSpec, 0)
	for _, d := range dgroups {
		gspecs = append(gspecs, d.GroupSpec)
	}

	t.Run("assert nominal test success", func(t *testing.T) {
		err := types.ValidateDeploymentGroups(gspecs)
		require.NoError(t, err)
	})

	gspecZeroed := make([]types.GroupSpec, len(gspecs))
	for _, g := range gspecs {
		g.OrderBidDuration = int64(0)
		gspecZeroed = append(gspecZeroed, g)
	}
	t.Run("assert error for zero value bid duration", func(t *testing.T) {
		err := types.ValidateDeploymentGroups(gspecZeroed)
		require.Error(t, err)
	})
}
