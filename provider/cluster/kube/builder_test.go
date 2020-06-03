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
	mb := newManifestBuilder(log, ns, leaseID, g)

	m, err := mb.create()
	assert.NoError(t, err)

	assert.Equal(t, ns, m.Name)
}
