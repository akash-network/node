package upgrades

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"

	utypes "github.com/akash-network/node/upgrades/types"
)

func TestUpgradesName(t *testing.T) {
	upgrades := utypes.GetUpgradesList()
	require.NotNil(t, upgrades)

	for name := range upgrades {
		// NOTE this is the only exception to the upgrade name
		// Rest MUST be compliant with SEMVER
		if name == "akash_v0.15.0_cosmos_v0.44.x" {
			continue
		}

		require.True(t, strings.HasPrefix(name, "v"), "upgrade name must start with \"v\"")

		require.True(t, semver.IsValid(name), fmt.Sprintf("upgrade name \"%s\" must be valid Semver", name))
	}
}
