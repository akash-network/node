package testutil

import (
	"testing"

	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/types"
	"github.com/stretchr/testify/require"
)

func NewState(t *testing.T, gen *types.Genesis) state.State {
	db := state.NewMemDB()
	state, err := state.LoadState(db, gen)
	require.NoError(t, err)
	return state
}
