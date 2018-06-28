package grpc

import (
	"net/http"
	"os"
	"testing"

	"github.com/ovrclk/akash/manifest"
	kmocks "github.com/ovrclk/akash/provider/cluster/kube/mocks"
	"github.com/ovrclk/akash/provider/manifest/mocks"
	mmocks "github.com/ovrclk/akash/provider/manifest/mocks"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tmlibs/log"
	"golang.org/x/net/context"
	"k8s.io/api/apps/v1"
)

func TestDeployManifest(t *testing.T) {
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

	server := newServer(log.NewTMLogger(os.Stdout), "tcp", "0", handler, client)

	_, err = server.Deploy(context.TODO(), req)
	assert.NoError(t, err)
}
func TestStatus(t *testing.T) {
	server := newServer(nil, "tcp", "3002", nil, nil)
	status, err := server.Status(context.TODO(), nil)
	assert.NoError(t, err)
	require.Equal(t, "OK", status.Message)
	require.Equal(t, http.StatusOK, int(status.Code))
}

func TestLease(t *testing.T) {
	handler := new(mmocks.Handler)
	client := new(kmocks.Client)
	mockResp := v1.DeploymentList{}
	client.On("KubeDeployments", mock.Anything, mock.Anything).Return(&mockResp, nil).Once()

	server := newServer(nil, "tcp", "3002", handler, client)
	response, err := server.LeaseStatus(context.TODO(), &types.LeaseStatusRequest{})
	assert.NoError(t, err)
	assert.NotNil(t, response)
}
