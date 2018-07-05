package grpc

import (
	"bytes"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/cluster"
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

func TestLeaseStatus(t *testing.T) {
	handler := new(mmocks.Handler)
	client := new(kmocks.Client)
	mockResp := types.LeaseStatusResponse{}
	client.On("LeaseStatus", mock.Anything, mock.Anything).Return(&mockResp, nil).Once()

	server := newServer(log.NewTMLogger(os.Stdout), "tcp", "3002", handler, client)
	response, err := server.LeaseStatus(context.TODO(), &types.LeaseStatusRequest{
		Deployment: "d6f4b6728c7deb187a07afe8e145e214c716e287039a204e7fac1fc121dc0cef",
		Group:      "1",
		Order:      "2",
		Provider:   "8224e14f903a2e136a6362527b19f11935197175cb69981940933aa04459a2a9",
	})
	assert.NoError(t, err)
	assert.NotNil(t, response)
}

func TestServiceStatus(t *testing.T) {
	handler := new(mmocks.Handler)
	client := new(kmocks.Client)
	mockResp := types.ServiceStatusResponse{}
	client.On("ServiceStatus", mock.Anything, mock.Anything).Return(&mockResp, nil).Once()

	server := newServer(log.NewTMLogger(os.Stdout), "tcp", "3002", handler, client)
	response, err := server.ServiceStatus(context.TODO(), &types.ServiceStatusRequest{
		Name:       "web",
		Deployment: "d6f4b6728c7deb187a07afe8e145e214c716e287039a204e7fac1fc121dc0cef",
		Group:      "1",
		Order:      "2",
		Provider:   "8224e14f903a2e136a6362527b19f11935197175cb69981940933aa04459a2a9",
	})
	assert.NoError(t, err)
	assert.NotNil(t, response)
}

type mockCluserServiceLogServer struct {
	testutil.MockGRPCStreamServer
}

var sent []*types.Log

func (s mockCluserServiceLogServer) Send(log *types.Log) error {
	sent = append(sent, log)
	return nil
}

func TestServiceLogs(t *testing.T) {
	handler := new(mmocks.Handler)
	client := new(kmocks.Client)
	message := "logs logs logs logs\n"
	stream := testutil.ReadCloser{Reader: bytes.NewBuffer([]byte(message))}
	serviceLog := cluster.NewServiceLog(t.Name(), stream)
	mockResp := []*cluster.ServiceLog{serviceLog}
	client.On("ServiceLogs", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mockResp, nil).Once()
	streamServer := mockCluserServiceLogServer{}
	server := newServer(log.NewTMLogger(os.Stdout), "tcp", "3002", handler, client)
	err := server.ServiceLogs(&types.LogRequest{
		Name:       t.Name(),
		Deployment: "d6f4b6728c7deb187a07afe8e145e214c716e287039a204e7fac1fc121dc0cef",
		Group:      "1",
		Order:      "2",
		Provider:   "8224e14f903a2e136a6362527b19f11935197175cb69981940933aa04459a2a9",
		Options: &types.LogOptions{
			TailLines: 1000,
			Follow:    false,
		},
	}, streamServer)
	assert.NoError(t, err)
	assert.Len(t, sent, 1)
	assert.Equal(t, strings.TrimSuffix(message, "\n"), sent[0].Message)
	assert.Equal(t, t.Name(), sent[0].Name)
}
