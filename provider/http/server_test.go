package http

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/ovrclk/akash/provider/cluster/kube"

	cmock "github.com/ovrclk/akash/provider/cluster/kube/mocks"
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

	sdl, err := sdl.ReadFile("../../_run/multi/deployment.yml")
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
	client := new(cmock.Client)

	withServer(t, func() {
		err = Send(mani, signer, provider, deployment)
		require.NoError(t, err)
	}, handler, client)
}

func TestStatus(t *testing.T) {
	t.Skip()
	// handler := new(pmock.Handler)
	// client := new(cmock.Client)
	// withServer(t, func() {
	// 	resp, err := http.Get("http://localhost:3001/status")
	// 	require.NoError(t, err)
	// 	body, err := ioutil.ReadAll(resp.Body)
	// 	require.NoError(t, err)
	// 	require.Equal(t, []byte("OK\n"), body)
	// }, handler, client)
}

func withServer(t *testing.T, fn func(), h *pmanifest.Handler, c kube.Client) {
	donech := make(chan struct{})
	defer func() { <-donech }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer close(donech)
		err := RunServer(ctx, testutil.Logger(), "3001", h, c)
		fmt.Println("server exited err:", err)
		assert.Error(t, http.ErrServerClosed, err)
	}()

	testutil.SleepForThreadStart(t)

	fn()
}
