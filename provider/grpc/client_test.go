package grpc

import (
	"context"
	"testing"

	"github.com/ovrclk/akash/manifest"
	cmocks "github.com/ovrclk/akash/provider/cluster/mocks"
	"github.com/ovrclk/akash/provider/manifest/mocks"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSendManifest(t *testing.T) {
	c, err := NewClient("localhost:3001")
	assert.NoError(t, err)

	sdl, err := sdl.ReadFile("../../_docs/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	key, kmgr := testutil.NewNamedKey(t)
	signer := testutil.Signer(t, kmgr)

	provider := testutil.Provider(key.GetAddress().Bytes(), 1)
	session := session.New(testutil.Logger(), provider, nil, nil)

	deployment := testutil.DeploymentAddress(t)

	req, _, err := manifest.SignManifest(mani, signer, deployment)
	assert.NoError(t, err)

	handler := &mocks.Handler{}
	handler.On("HandleManifest", mock.Anything, mock.Anything).Return(nil)

	client := &cmocks.Client{}

	donech := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer close(donech)
		assert.NoError(t, Run(ctx, ":3001", session, client, nil, handler))
	}()

	testutil.SleepForThreadStart(t)

	_, err = c.Deploy(context.TODO(), req)
	assert.NoError(t, err)

	testutil.SleepForThreadStart(t)

	cancel()
	<-donech
}
