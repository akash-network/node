package manifest

import (
	"testing"

	"github.com/ovrclk/akash/testutil"
	_ "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifest(t *testing.T) {
	runServer(t)
	sendManifest(t)
}

func runServer(t *testing.T) {
	go func() {
		run("3001", "debug")
	}()
}

func sendManifest(t *testing.T) {
	mani := &Manifest{}
	path := "../_docs/manifest.yaml"
	err := mani.Parse(path)
	require.NoError(t, err)

	_, kmgr := testutil.NewNamedKey(t)
	signer := testutil.Signer(t, kmgr)

	state := testutil.NewState(t, nil)
	pacc, _ := testutil.CreateAccount(t, state)
	provider := testutil.Provider(pacc.Address, uint64(1))

	lease := []byte("leaseaddress")

	err = mani.Send(signer, provider.Address, lease, "http://localhost:3001/manifest")
	require.NoError(t, err)
}
