package grpc

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/ovrclk/akash/provider/cluster"
	cmocks "github.com/ovrclk/akash/provider/cluster/mocks"
	mmocks "github.com/ovrclk/akash/provider/manifest/mocks"
	pmocks "github.com/ovrclk/akash/provider/mocks"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func TestStatus(t *testing.T) {

	sclient := &pmocks.StatusClient{}
	sclient.On("Status", mock.Anything).
		Return(&types.ProviderStatus{}, nil)

	provider := &types.Provider{
		Address: testutil.Address(t),
	}

	session := session.New(testutil.Logger(), provider, nil, nil)

	server := create(session, nil, sclient, nil)
	status, err := server.Status(context.TODO(), nil)
	assert.NoError(t, err)
	require.Equal(t, "OK", status.Message)
	require.Equal(t, http.StatusOK, int(status.Code))
	require.NotNil(t, status.Provider)
}

func TestLeaseStatus(t *testing.T) {
	handler := new(mmocks.Handler)
	client := new(cmocks.Client)
	mockResp := types.LeaseStatusResponse{}
	client.On("LeaseStatus", mock.Anything, mock.Anything).Return(&mockResp, nil).Once()

	session := session.New(testutil.Logger(), nil, nil, nil)
	server := create(session, client, nil, handler)

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
	client := new(cmocks.Client)
	mockResp := types.ServiceStatusResponse{}
	client.On("ServiceStatus", mock.Anything, mock.Anything).Return(&mockResp, nil).Once()

	session := session.New(testutil.Logger(), nil, nil, nil)
	server := create(session, client, nil, handler)

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
	client := new(cmocks.Client)
	message := "logs logs logs logs\n"
	stream := testutil.ReadCloser{Reader: bytes.NewBuffer([]byte(message))}
	serviceLog := cluster.NewServiceLog(t.Name(), stream)
	mockResp := []*cluster.ServiceLog{serviceLog}
	client.On("ServiceLogs", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mockResp, nil).Once()
	streamServer := mockCluserServiceLogServer{}

	session := session.New(testutil.Logger(), nil, nil, nil)
	server := create(session, client, nil, handler)

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
