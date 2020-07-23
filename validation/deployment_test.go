package validation_test

import (
	"testing"

	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/validation"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/stretchr/testify/require"
)

func TestZeroValueGroupSpec(t *testing.T) {
	did := testutil.DeploymentID(t)
	dgroup := testutil.DeploymentGroup(t, did, uint32(6))
	gspec := dgroup.GroupSpec

	t.Run("assert nominal test success", func(t *testing.T) {
		err := validation.ValidateDeploymentGroup(gspec)
		require.NoError(t, err)
	})

	gspec.OrderBidDuration = int64(0)
	t.Run("assert error for zero value bid duration", func(t *testing.T) {
		err := validation.ValidateDeploymentGroup(gspec)
		require.Error(t, err)
	})
}

func TestZeroValueGroupSpecs(t *testing.T) {
	did := testutil.DeploymentID(t)
	dgroups := testutil.DeploymentGroups(t, did, uint32(6))
	gspecs := make([]dtypes.GroupSpec, 0)
	for _, d := range dgroups {
		gspecs = append(gspecs, d.GroupSpec)
	}

	t.Run("assert nominal test success", func(t *testing.T) {
		err := validation.ValidateDeploymentGroups(gspecs)
		require.NoError(t, err)
	})

	gspecZeroed := make([]dtypes.GroupSpec, len(gspecs))
	for _, g := range gspecs {
		g.OrderBidDuration = int64(0)
		gspecZeroed = append(gspecZeroed, g)
	}
	t.Run("assert error for zero value bid duration", func(t *testing.T) {
		err := validation.ValidateDeploymentGroups(gspecZeroed)
		require.Error(t, err)
	})
}
