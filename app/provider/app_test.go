package provider_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	app_ "github.com/ovrclk/akash/app/provider"
	pstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/abci/types"
)

func TestProviderApp(t *testing.T) {
	state := testutil.NewState(t, nil)
	app, err := app_.NewApp(state, testutil.Logger())
	require.NoError(t, err)

	account, key := testutil.CreateAccount(t, state)
	nonce := uint64(1)

	provider := testutil.CreateProvider(t, app, account, &key, nonce)

	{
		assert.True(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", pstate.ProviderPath, hex.EncodeToString(provider.Address))}))
		assert.False(t, app.AcceptQuery(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", "/foo/", hex.EncodeToString(provider.Address))}))
	}

	{
		resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", pstate.ProviderPath, hex.EncodeToString(provider.Address))})
		assert.Empty(t, resp.Log)
		require.True(t, resp.IsOK())

		queriedprovider := new(types.Provider)
		require.NoError(t, queriedprovider.Unmarshal(resp.Value))
		assert.NotEmpty(t, resp.Value)

		assert.Equal(t, provider.Address, queriedprovider.Address)
		assert.Equal(t, provider.Attributes[0].Name, queriedprovider.Attributes[0].Name)
		assert.Equal(t, provider.Attributes[0].Value, queriedprovider.Attributes[0].Value)
	}
}
