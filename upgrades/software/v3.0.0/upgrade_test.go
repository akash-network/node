package v3_0_0

import (
	"testing"

	"cosmossdk.io/log"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"
	"github.com/stretchr/testify/require"

	mv1 "pkg.akt.dev/go/node/market/v1"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"

	utypes "pkg.akt.dev/node/v3/upgrades/types"
	verificationtypes "pkg.akt.dev/node/v3/x/verification/types"
)

func TestStoreLoaderAddsVerificationStore(t *testing.T) {
	storeUpgrades := (&upgrade{}).StoreLoader()

	require.NotNil(t, storeUpgrades)
	require.ElementsMatch(t, []string{verificationtypes.StoreKey}, storeUpgrades.Added)
	require.Empty(t, storeUpgrades.Deleted)
}

func TestUpgradeRegistersVerificationStoreAndProviderMigration(t *testing.T) {
	upgrades := utypes.GetUpgradesList()
	require.Contains(t, upgrades, UpgradeName)

	up, err := upgrades[UpgradeName](log.NewNopLogger(), nil)
	require.NoError(t, err)

	storeUpgrades := up.StoreLoader()
	require.NotNil(t, storeUpgrades)
	require.Contains(t, storeUpgrades.Added, verificationtypes.StoreKey)

	var foundProviderMigration bool
	utypes.ModuleMigrations(ptypes.ModuleName, nil, func(_ string, version uint64, _ sdkmodule.MigrationHandler) {
		if version == 3 {
			foundProviderMigration = true
		}
	})
	require.True(t, foundProviderMigration)

	var foundMarketMigration bool
	utypes.ModuleMigrations(mv1.ModuleName, nil, func(_ string, version uint64, _ sdkmodule.MigrationHandler) {
		if version == 8 {
			foundMarketMigration = true
		}
	})
	require.True(t, foundMarketMigration)
}
