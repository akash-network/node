package kube

import (
	"testing"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
)

func TestLidNsSanity(t *testing.T) {
	log := testutil.Logger(t)
	leaseID := testutil.LeaseID(t)

	ns := lidNS(leaseID)

	assert.NotEmpty(t, ns)

	// namespaces must be no more than 63 characters.
	assert.Less(t, len(ns), int(64))

	g := &manifest.Group{}
	mb := newManifestBuilder(log, settings{}, ns, leaseID, g)

	m, err := mb.create()
	assert.NoError(t, err)
	assert.Equal(t, m.Spec.LeaseID, leaseID)

	assert.Equal(t, ns, m.Name)
}

func TestNetworkPolicies(t *testing.T) {
	leaseID := testutil.LeaseID(t)

	g := &manifest.Group{}
	np := newNetPolBuilder(settings{}, leaseID, g)
	netPolicies, err := np.create()
	assert.NoError(t, err)
	assert.Len(t, netPolicies, 4)

	pol0 := netPolicies[0]
	assert.Equal(t, pol0.Name, "ingress-deny-all")

	// Change the DSeq ID
	np.lid.DSeq = uint64(100)
	k := akashNetworkNamespace
	ns := lidNS(np.lid)
	updatedNetPol, err := np.update(netPolicies[0])
	assert.NoError(t, err)
	updatedNS := updatedNetPol.Labels[k]
	assert.Equal(t, ns, updatedNS)
}
