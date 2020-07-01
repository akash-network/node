package keeper

import (
	"strconv"
	"testing"

	fuzz "github.com/google/gofuzz"
	"github.com/stretchr/testify/assert"

	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/deployment/types"
)

const iterations = 100

func TestDeploymentKeyValues(t *testing.T) {
	for i := 0; i < iterations; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			dep := testutil.Deployment(t)

			// Assert Key length
			key, err := deploymentStateIDKey(dep)
			assert.NoError(t, err, "error creating deployment key")
			assert.Equal(t, len(key), 30)

			// Assert two keys to search are generated
			keys, err := deploymentStatelessIDKeys(dep.ID())
			assert.NoError(t, err, "error creating deployment keys")
			assert.Len(t, keys, 2)
		})
	}
}

func TestDeploymentStateKeyValueExtents(t *testing.T) {
	for i := 0; i < iterations; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			f := fuzz.New()
			var dep types.DeploymentState
			f.Fuzz(&dep)
			key, err := deploymentStateKey(dep)
			assert.NoError(t, err, "error creating deployment key")
			assert.Equal(t, len(key), 2)
		})
	}
}
