package v2_1_0

import (
	"testing"

	"github.com/stretchr/testify/require"

	verificationtypes "pkg.akt.dev/node/v2/x/verification/types"
)

func TestStoreLoaderAddsVerificationStore(t *testing.T) {
	storeUpgrades := (&upgrade{}).StoreLoader()

	require.NotNil(t, storeUpgrades)
	require.ElementsMatch(t, []string{verificationtypes.StoreKey}, storeUpgrades.Added)
	require.Empty(t, storeUpgrades.Deleted)
}
