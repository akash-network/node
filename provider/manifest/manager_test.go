package manifest

import (
	"context"
	"errors"
	"github.com/boz/go-lifecycle"
	clientMocks "github.com/ovrclk/akash/client/mocks"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/deployment/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func serviceForManifestTest(t *testing.T) *service {
	svc := &service{}
	log := testutil.Logger(t)
	bus := pubsub.NewBus()
	svc.bus = bus
	var err error
	svc.sub, err = bus.Subscribe()
	require.NoError(t, err)

	clientMock := &clientMocks.Client{}
	queryMock := &clientMocks.QueryClient{}

	clientMock.On("Query").Return(queryMock)

	version := []byte("test")
	res := &types.QueryDeploymentResponse{
		Deployment: types.DeploymentResponse{
			Deployment: types.Deployment{},
			Groups:     nil,
			Version:    version,
		},
	}
	queryMock.On("Deployment", mock.Anything, mock.Anything).Return(res, nil)

	svc.session = session.New(log, clientMock, nil)
	svc.lc = lifecycle.New()
	svc.managerch = make(chan *manager, 1) // Size of 1 here, because the manager writes to this when closed
	return svc
}

func TestManagerReturnsNoLease(t *testing.T) {
	deploymentAddr := testutil.DeploymentID(t)
	svc := serviceForManifestTest(t)
	m, err := newManager(svc, deploymentAddr)
	require.NoError(t, err)
	require.NotNil(t, m)

	ch := make(chan error, 1)
	req := manifestRequest{
		value: nil,
		ch:    ch,
		ctx:   context.Background(),
	}

	m.handleManifest(req)

	select {
	case err := <-ch:
		require.Error(t, err)
		require.True(t, errors.Is(ErrNoLeaseForDeployment, err))
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for response")
	}
	close(ch)

	svc.lc.ShutdownInitiated(nil)

	select {
	case <-m.lc.Done():

	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for manager shutdown")
	}
}
