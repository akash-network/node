package grpc

import (
	"context"
	"os"
	"testing"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/manifest/mocks"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tmlibs/log"
)

func TestDeployManifest(t *testing.T) {
	sdl, err := sdl.ReadFile("../../../_docs/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	_, kmgr := testutil.NewNamedKey(t)
	signer := testutil.Signer(t, kmgr)

	deployment := testutil.DeploymentAddress(t)

	req, err := manifest.SignManifest(mani, signer, deployment)
	assert.NoError(t, err)

	handler := &mocks.Handler{}
	handler.On("HandleManifest", mock.Anything).Return(nil)

	server := NewServer(log.NewTMLogger(os.Stdout), "tcp", "0", handler)

	_, err = server.DeployManifest(context.TODO(), req)
	assert.NoError(t, err)
}
