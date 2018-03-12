package testutil

import (
	"testing"

	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/types"
	"github.com/stretchr/testify/require"
	crypto "github.com/tendermint/go-crypto"
)

func NewState(t *testing.T, gen *types.Genesis) state.State {
	db := state.NewMemDB()
	state, err := state.LoadState(db, gen)
	require.NoError(t, err)
	return state
}

func CreateAccount(t *testing.T, state state.State) (*types.Account, crypto.PrivKey) {
	key := PrivateKey(t)
	account := &types.Account{
		Address: key.PubKey().Address(),
	}
	require.NoError(t, state.Account().Save(account))
	return account, key
}
