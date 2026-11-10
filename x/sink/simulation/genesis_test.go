package simulation_test

import (
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/cosmos/cosmos-sdk/types/module"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/x/sink/simulation"
)

func TestRandomizedGenesis(t *testing.T) {
	cdc := sdkutil.MakeEncodingConfig().Codec
	for _, count := range []int{0, 1, 3, 8} {
		var previous []byte
		for seed := range int64(10) {
			var first []byte
			for run := range 2 {
				r := rand.New(rand.NewSource(seed)) //nolint:gosec
				state := &module.SimulationState{
					Cdc:      cdc,
					Rand:     r,
					GenState: make(map[string]json.RawMessage),
					Accounts: simtypes.RandomAccounts(r, count),
				}

				simulation.RandomizedGenState(state)
				var genesis types.GenesisState

				cdc.MustUnmarshalJSON(state.GenState[types.ModuleName], &genesis)

				err := genesis.ValidateBasic()
				require.NoError(t, err)

				require.Equal(t, count > 0, genesis.Params.Enabled)
				require.Equal(t, types.DefaultParams().ReporterChallengePeriod, genesis.Params.ReporterChallengePeriod)

				if run == 0 {
					first = state.GenState[types.ModuleName]
				} else {
					require.Equal(t, first, []byte(state.GenState[types.ModuleName]))
				}
			}

			if seed > 0 {
				require.NotEqual(t, previous, first, "different seeds should exercise different valid parameters")
			}

			previous = first
		}
	}
}
