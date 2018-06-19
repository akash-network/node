package http

import (
	"context"
	"io/ioutil"
	"net/http"
	"testing"

	pmanifest "github.com/ovrclk/akash/provider/manifest/mocks"
	pmock "github.com/ovrclk/akash/provider/manifest/mocks"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestManifest(t *testing.T) {

	sdl, err := sdl.ReadFile("../../../_docs/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	_, kmgr := testutil.NewNamedKey(t)
	signer := testutil.Signer(t, kmgr)

	provider := &types.Provider{
		HostURI: "http://localhost:3001",
	}

	deployment := testutil.DeploymentAddress(t)

	handler := new(pmock.Handler)
	handler.On("HandleManifest", mock.Anything).Return(nil).Once()

	withServer(t, func() {
		testutil.SleepForThreadStart(t)
		err = Send(mani, signer, provider, deployment)
		require.NoError(t, err)
	}, handler)
}

func TestStatus(t *testing.T) {
	handler := new(pmock.Handler)

	withServer(t, func() {
		resp, err := http.Get("http://localhost:3001/status")
		require.NoError(t, err)
		body, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, []byte("OK\n"), body)
	}, handler)
}

func withServer(t *testing.T, fn func(), h *pmanifest.Handler) {
	donech := make(chan struct{})
	defer func() { <-donech }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer close(donech)
		err := RunServer(ctx, testutil.Logger(), "3001", h)
		assert.Error(t, http.ErrServerClosed, err)
	}()

	fn()
}
