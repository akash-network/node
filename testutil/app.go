package testutil

import (
	"testing"

	tmtypes "github.com/tendermint/abci/types"

	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/require"
)

func NewApp(t *testing.T, gen *types.Genesis) tmtypes.Application {
	commit, cache := NewState(t, gen)
	app, err := app.Create(commit, cache, Logger())
	require.NoError(t, err)
	return app
}
