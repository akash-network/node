package v2beta1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovrclk/akash/testutil"
)

func Test_Manifest_encoding(t *testing.T) {
	for _, spec := range testutil.ManifestGenerators {

		// ensure decode(encode(obj)) == obj

		var (
			lid  = testutil.LeaseID(t)
			mgrp = spec.Generator.Group(t)
		)

		kmani, err := NewManifest("foo", lid, &mgrp)
		require.NoError(t, err, spec.Name)

		deployment, err := kmani.Deployment()
		require.NoError(t, err, spec.Name)

		assert.Equal(t, lid, deployment.LeaseID(), spec.Name)
		assert.Equal(t, mgrp, deployment.ManifestGroup(), spec.Name)
	}
}
