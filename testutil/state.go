package testutil

import (
	"testing"

	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/require"
	crypto "github.com/tendermint/tendermint/crypto"
)

// NewState used only for testing
func NewState(t *testing.T, gen *types.Genesis) (state.CommitState, state.CacheState) {
	db := state.NewMemDB()
	commitState, cacheState, err := state.LoadState(db, gen)
	require.NoError(t, err)
	// prime commit state so root is not nil
	cacheState.Set([]byte("test"), []byte("Test"))
	err = cacheState.Write()
	require.NoError(t, err)
	return commitState, cacheState
}

func CreateAccount(t *testing.T, state state.State) (*types.Account, crypto.PrivKey) {
	key := PrivateKey(t)
	account := &types.Account{
		Address: key.PubKey().Address().Bytes(),
		Balance: 1000000000,
	}
	require.NoError(t, state.Account().Save(account))
	return account, key
}
