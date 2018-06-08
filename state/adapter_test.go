package state_test

import (
	"testing"

	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/stretchr/testify/require"
)

func TestAccountAdapter(t *testing.T) {
	db := state.NewMemDB()
	st := state.NewState(db)
	adapter := state.NewAccountAdapter(st)

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
