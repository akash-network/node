package grpc

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ovrclk/akash/manifest"
	kmocks "github.com/ovrclk/akash/provider/cluster/kube/mocks"
	"github.com/ovrclk/akash/provider/manifest/mocks"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tmlibs/log"
)

func TestSendManifest(t *testing.T) {
	c, err := NewClient("localhost:3001")
	assert.NoError(t, err)

	sdl, err := sdl.ReadFile("../../_docs/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	_, kmgr := testutil.NewNamedKey(t)
	signer := testutil.Signer(t, kmgr)

	deployment := testutil.DeploymentAddress(t)

	req, _, err := manifest.SignManifest(mani, signer, deployment)
	assert.NoError(t, err)

	handler := &mocks.Handler{}
	handler.On("HandleManifest", mock.Anything).Return(nil)

	client := &kmocks.Client{}

	server := newServer(log.NewTMLogger(os.Stdout), "tcp", ":3001", handler, client)
	go func() {
		err := server.listenAndServe()
		require.NoError(t, err)
	}()

	time.Sleep(1 * time.Second)

	_, err = c.Deploy(context.TODO(), req)
	assert.NoError(t, err)
}
