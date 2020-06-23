// +build integration,!mainnet

package integration

import (
	"fmt"
	"testing"

	"github.com/cosmos/cosmos-sdk/tests"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestProvider(t *testing.T) {
	t.Parallel()
	f := InitFixtures(t)

	// start akashd server
	proc := f.AkashdStart()

	defer func() {
		_ = proc.Stop(false)
	}()

	// Save key addresses for later use
	fooAddr := f.KeyAddress(keyFoo)

	fooAcc := f.QueryAccount(fooAddr)
	startTokens := sdk.TokensFromConsensusPower(denomStartValue)
	require.Equal(t, startTokens, fooAcc.GetCoins().AmountOf(denom))

	// Create provider
	f.TxCreateProvider(fmt.Sprintf("--from=%s", keyFoo), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// test query providers
	providers := f.QueryProviders()
	require.Len(t, providers, 1, "Creating provider failed")
	require.Equal(t, fooAddr.String(), providers[0].Owner.String())

	// test query provider
	createdProvider := providers[0]
	provider := f.QueryProvider(createdProvider.Owner.String())
	require.Equal(t, createdProvider, provider)

	f.Cleanup()
}
