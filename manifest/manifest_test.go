package manifest_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/ovrclk/akash/manifest"
	pmanifest "github.com/ovrclk/akash/provider/manifest"
	pmock "github.com/ovrclk/akash/provider/manifest/mocks"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestManifest(t *testing.T) {

	sdl, err := sdl.ReadFile("../_docs/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	_, kmgr := testutil.NewNamedKey(t)
	signer := testutil.Signer(t, kmgr)

	provider := &types.Provider{
		HostURI: "http://localhost:3001/manifest",
	}

	deployment := testutil.DeploymentAddress(t)

	handler := new(pmock.Handler)
	handler.On("HandleManifest", mock.Anything).Return(nil).Once()

	withServer(t, func() {
		err = manifest.Send(mani, signer, provider, deployment)
		require.NoError(t, err)
	}, handler)
}

func withServer(t *testing.T, fn func(), h pmanifest.Handler) {
	donech := make(chan struct{})
	defer func() { <-donech }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer close(donech)
		err := manifest.RunServer(ctx, testutil.Logger(), "3001", h)
		assert.Error(t, http.ErrServerClosed, err)
	}()

	fn()
}
