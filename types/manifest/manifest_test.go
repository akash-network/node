package manifest

import (
	"testing"

	"github.com/ovrclk/akash/testutil"
	_ "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifest(t *testing.T) {
	mani := &Manifest{}
	path := "../../_docs/manifest.yaml"
	err := mani.Parse(path)
	require.NoError(t, err)

	testutil.NewNamedKey(t)
	signer := testutil.Signer(t)

	state := testutil.NewState(t, nil)
	pacc, _ := testutil.CreateAccount(t, state)
	provider := testutil.Provider(pacc.Address, uint64(1))

	mani.Send(signer, provider.Address, "localhost:3001")
	require.True(t, false)
}
