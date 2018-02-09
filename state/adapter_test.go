package state_test

import (
	"testing"

	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/stretchr/testify/require"
)

func TestAccountAdapter(t *testing.T) {
	db := state.NewMemDB()
	adapter := state.NewAccountAdapter(db)

	address := base.Bytes("foo")

	acct := &types.Account{
		Address: address,
		Balance: 200,
	}

	require.NoError(t, adapter.Save(acct))

	acct_, err := adapter.Get(address)
	require.NoError(t, err)

	require.Equal(t, acct, acct_)
}
