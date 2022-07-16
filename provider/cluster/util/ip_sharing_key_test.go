package util_test

import (
	"github.com/ovrclk/akash/provider/cluster/util"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPassesThroughNames(t *testing.T) {
	leaseID := testutil.LeaseID(t)

	sharingKey := util.MakeIPSharingKey(leaseID, "foobar")
	require.Contains(t, sharingKey, "foobar")
}

func TestFiltersUnderscore(t *testing.T) {
	leaseID := testutil.LeaseID(t)

	sharingKey := util.MakeIPSharingKey(leaseID, "me_you")
	require.NotContains(t, sharingKey, "me_you")
}

func TestFiltersUpperccase(t *testing.T) {
	leaseID := testutil.LeaseID(t)

	sharingKey := util.MakeIPSharingKey(leaseID, "meYOU")
	require.NotContains(t, sharingKey, "meYOU")

	require.Equal(t, sharingKey, leaseID.GetOwner()+"-ip-ps9pn7rkocct7m9ivtovuktb")
}
