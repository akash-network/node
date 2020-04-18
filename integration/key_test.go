package integration

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/tests"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestAkashKeysAddRecover(t *testing.T) {
	t.Parallel()
	f := InitFixtures(t)

	exitSuccess, _, _ := f.KeysAddRecover("empty-mnemonic", "")
	require.False(t, exitSuccess)

	exitSuccess, _, _ = f.KeysAddRecover("test-recover", "donate behave film hero magnet disagree sock talk alarm loop stone imitate apology weird desert member trouble warrior book man alien mixed remain hold")
	require.True(t, exitSuccess)
	require.Equal(t, "akash1m37wsknc9qluzkqxrec965unxlukuprgg0pxdj", f.KeyAddress("test-recover").String())

	// Cleanup testing directories
	f.Cleanup()
}

func TestAkashKeysList(t *testing.T) {
	t.Parallel()
	f := InitFixtures(t)

	f.KeysDelete(keyFoo)
	f.KeysDelete(keyBar)
	f.KeysDelete(keyBaz)
	f.KeysAdd(keyFoo)

	fooAddr := f.KeyAddress(keyFoo)

	list := f.KeysList()
	require.Len(t, list, 1)
	require.Equal(t, fooAddr.String(), list[0].Address)

	// Cleanup testing directories
	f.Cleanup()
}

func TestAkashSend(t *testing.T) {
	t.Parallel()
	f := InitFixtures(t)

	// start akashd server
	proc := f.AkashdStart()

	defer func() {
		_ = proc.Stop(false)
	}()

	// Save key addresses for later use
	fooAddr := f.KeyAddress(keyFoo)
	bazAddr := f.KeyAddress(keyBaz)

	fooAcc := f.QueryAccount(fooAddr)
	startTokens := sdk.TokensFromConsensusPower(denomStartValue)
	require.Equal(t, startTokens, fooAcc.GetCoins().AmountOf(denom))

	// Send some tokens from one account to the other
	sendTokens := sdk.TokensFromConsensusPower(10)
	f.TxSend(keyFoo, bazAddr, sdk.NewCoin(denom, sendTokens), "-y")
	tests.WaitForNextNBlocksTM(1, f.Port)

	// Ensure account balances match expected
	barAcc := f.QueryAccount(bazAddr)
	require.Equal(t, sendTokens, barAcc.GetCoins().AmountOf(denom))

	fooAcc = f.QueryAccount(fooAddr)
	require.Equal(t, startTokens.Sub(sendTokens), fooAcc.GetCoins().AmountOf(denom))

	f.Cleanup()
}
