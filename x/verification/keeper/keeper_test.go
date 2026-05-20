package keeper

import (
	"testing"

	storetypes "cosmossdk.io/store/types"
	"github.com/stretchr/testify/require"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	moduletypes "pkg.akt.dev/node/v2/x/verification/types"
)

func TestKeeperShell(t *testing.T) {
	key := storetypes.NewKVStoreKey(moduletypes.StoreKey)
	k := NewKeeper(nil, key)

	require.Equal(t, key, k.StoreKey())
	require.Nil(t, k.Codec())
	require.NotNil(t, k.NewQuerier())

	got, err := k.Settle(SettlementInput{
		Path:             SettlementPathAttestationExpired,
		FaultAttribution: vtypes.FaultAttributionNoFault,
	})
	require.NoError(t, err)
	require.Equal(t, SettlementResult{
		FeeStatus:     vtypes.FeeStatusReleasedToAuditor,
		DepositStatus: vtypes.DepositStatusReturnedToAuditor,
	}, got)
}
