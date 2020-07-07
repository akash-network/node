package keeper

import (
	"strings"
	"testing"

	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupKeyConversion(t *testing.T) {
	d := testutil.Deployment(t)
	g := testutil.DeploymentGroup(t, d.ID(), uint32(0))

	dataKey := groupKey(g.ID())
	openKey := groupOpenKey(g.ID())

	assert.NotEqual(t, dataKey, openKey)

	convertedKey, err := groupOpenKeyConvert(openKey)
	require.NoError(t, err)
	assert.Equal(t, dataKey, convertedKey)

	t.Run("open-group set for org key", func(t *testing.T) {
		groupSetKey := groupsOpenKey(g.ID().DeploymentID())
		assert.True(t, strings.Contains(string(openKey), string(groupSetKey)))
	})
}
