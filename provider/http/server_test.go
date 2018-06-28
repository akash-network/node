package http

import (
	"context"
	"fmt"
	"io/ioutil"
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
	"k8s.io/api/apps/v1"
)

func TestStatus(t *testing.T) {
	withServer(t, func() {
		resp, err := http.Get("http://localhost:3003/status")
		require.NoError(t, err)
		body, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		fmt.Println(string(body))
		require.Equal(t, []byte("OK\n"), body)
	}, nil, nil, "3003")
}

func TestManifest(t *testing.T) {

	sdl, err := sdl.ReadFile("../../_run/multi/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	_, kmgr := testutil.NewNamedKey(t)
	signer := testutil.Signer(t, kmgr)

	provider := &types.Provider{
		HostURI: "http://localhost:3004",
	}

	deployment := testutil.DeploymentAddress(t)

	handler := new(pmock.Handler)
	handler.On("HandleManifest", mock.Anything).Return(nil).Once()
	client := new(cmock.Client)

	withServer(t, func() {
		err = SendManifest(mani, signer, provider, deployment)
		require.NoError(t, err)
	}, handler, client, "3004")
}

func TestLease(t *testing.T) {
	handler := new(pmock.Handler)
	client := new(cmock.Client)
	mockResp := v1.DeploymentList{}
	client.On("KubeDeployments", mock.Anything, mock.Anything).Return(&mockResp, nil).Once()

	withServer(t, func() {
		resp, err := http.Get("http://localhost:3002/lease/deployment/group/order/provider")
		require.NoError(t, err)
		body, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		fmt.Println(string(body))
		require.Equal(t, []byte("{}\n"), body)
	}, handler, client, "3002")
}

func withServer(t *testing.T, fn func(), h *pmanifest.Handler, c kube.Client, port string) {
	donech := make(chan struct{})
	defer func() { <-donech }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer close(donech)
		err := RunServer(ctx, testutil.Logger(), port, h, c)
		assert.Error(t, http.ErrServerClosed, err)
	}()

	testutil.SleepForThreadStart(t)

	fn()
}
