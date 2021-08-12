package manifest

import (
	"context"
	"errors"
	clustertypes "github.com/ovrclk/akash/provider/cluster/types"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	clientMocks "github.com/ovrclk/akash/client/mocks"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/cluster/util"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/deployment/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	markettypes "github.com/ovrclk/akash/x/market/types"
	ptypes "github.com/ovrclk/akash/x/provider/types"
)

type scaffold struct {
	svc       *service
	cancel    context.CancelFunc
	bus       pubsub.Bus
	queryMock *clientMocks.QueryClient
	hostnames clustertypes.HostnameServiceClient
}

func serviceForManifestTest(t *testing.T, cfg ServiceConfig, mani sdl.SDL, did dtypes.DeploymentID) *scaffold {

	clientMock := &clientMocks.Client{}
	queryMock := &clientMocks.QueryClient{}

	clientMock.On("Query").Return(queryMock)

	var version []byte
	var err error
	if mani != nil {
		m, err := mani.Manifest()
		require.NoError(t, err)
		version, err = sdl.ManifestVersion(m)
		require.NoError(t, err)
		require.NotNil(t, version)
	} else {
		version = []byte("test")
	}

	var groups []dtypes.Group
	if mani != nil {
		dgroups, err := mani.DeploymentGroups()
		require.NoError(t, err)
		require.NotNil(t, dgroups)
		for _, g := range dgroups {
			groups = append(groups, dtypes.Group{
				GroupID: dtypes.GroupID{
					Owner: "",
					DSeq:  0,
					GSeq:  0,
				},
				State:     0,
				GroupSpec: *g,
			})
		}
	}

	res := &types.QueryDeploymentResponse{
		Deployment: types.Deployment{
			DeploymentID: did,
			State:        0,
			Version:      version,
		},
		Groups: groups,
	}
	queryMock.On("Deployment", mock.Anything, mock.Anything).Return(res, nil)

	ctx, cancel := context.WithCancel(context.Background())

	// Use this type in test
	hostnames := cluster.NewSimpleHostnames()

	log := testutil.Logger(t)
	bus := pubsub.NewBus()

	accAddr := testutil.AccAddress(t)
	p := &ptypes.Provider{
		Owner:      accAddr.String(),
		HostURI:    "",
		Attributes: nil,
	}

	queryMock.On("ActiveLeasesForProvider", p.Address()).Return([]markettypes.QueryLeaseResponse{}, nil)

	serviceInterface, err := NewService(ctx, session.New(log, clientMock, p), bus, hostnames, cfg)
	require.NoError(t, err)

	svc := serviceInterface.(*service)

	return &scaffold{
		svc:       svc,
		cancel:    cancel,
		bus:       bus,
		queryMock: queryMock,
		hostnames: hostnames,
	}
}

func TestManagerReturnsNoLease(t *testing.T) {
	s := serviceForManifestTest(t, ServiceConfig{}, nil, dtypes.DeploymentID{})

	sdl2, err := sdl.ReadFile("../../x/deployment/testdata/deployment-v2.yaml")
	require.NoError(t, err)

	sdlManifest, err := sdl2.Manifest()
	require.NoError(t, err)

	did := testutil.DeploymentID(t)
	err = s.svc.Submit(context.Background(), did, sdlManifest)
	require.Error(t, err)
	require.True(t, errors.Is(ErrNoLeaseForDeployment, err))

	s.cancel()

	select {
	case <-s.svc.lc.Done():

	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for service shutdown")
	}
}

func TestManagerRequiresHostname(t *testing.T) {
	sdl2, err := sdl.ReadFile("../../x/deployment/testdata/deployment-v2-nohost.yaml")
	require.NoError(t, err)

	sdlManifest, err := sdl2.Manifest()
	require.NoError(t, err)
	require.Len(t, sdlManifest[0].Services[0].Expose[0].Hosts, 0)

	lid := testutil.LeaseID(t)
	did := lid.DeploymentID()
	dgroups, err := sdl2.DeploymentGroups()
	require.NoError(t, err)

	// Tell the service that a lease has been won
	dgroup := &dtypes.Group{
		GroupID:   lid.GroupID(),
		State:     0,
		GroupSpec: *dgroups[0],
	}

	ev := event.LeaseWon{
		LeaseID: lid,
		Group:   dgroup,
		Price: sdk.Coin{
			Denom:  "uakt",
			Amount: sdk.NewInt(111),
		},
	}
	version, err := sdl.ManifestVersion(sdlManifest)
	require.NoError(t, err)
	require.NotNil(t, version)
	s := serviceForManifestTest(t, ServiceConfig{HTTPServicesRequireAtLeastOneHost: true}, sdl2, did)
	err = s.bus.Publish(ev)
	require.NoError(t, err)

	time.Sleep(time.Second) // Wait for publish to do its thing

	err = s.svc.Submit(context.Background(), did, sdlManifest)
	require.Error(t, err)
	require.Regexp(t, `^.+service ".+" exposed on .+:.+ must have a hostname$`, err.Error())

	s.cancel()
	select {
	case <-s.svc.lc.Done():

	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for service shutdown")
	}
}

func TestManagerAllowsUpdate(t *testing.T) {
	sdl2, err := sdl.ReadFile("../../x/deployment/testdata/deployment-v2.yaml")
	require.NoError(t, err)
	sdl2NewContainer, err := sdl.ReadFile("../../x/deployment/testdata/deployment-v2-newcontainer.yaml")
	require.NoError(t, err)

	sdlManifest, err := sdl2.Manifest()
	require.NoError(t, err)

	lid := testutil.LeaseID(t)
	did := lid.DeploymentID()
	dgroups, err := sdl2.DeploymentGroups()
	require.NoError(t, err)

	// Tell the service that a lease has been won
	dgroup := &dtypes.Group{
		GroupID:   lid.GroupID(),
		State:     0,
		GroupSpec: *dgroups[0],
	}

	ev := event.LeaseWon{
		LeaseID: lid,
		Group:   dgroup,
		Price: sdk.Coin{
			Denom:  "uakt",
			Amount: sdk.NewInt(111),
		},
	}
	version, err := sdl.ManifestVersion(sdlManifest)
	require.NotNil(t, version)
	require.NoError(t, err)
	s := serviceForManifestTest(t, ServiceConfig{HTTPServicesRequireAtLeastOneHost: true}, sdl2, did)

	err = s.bus.Publish(ev)
	require.NoError(t, err)
	time.Sleep(time.Second) // Wait for publish to do its thing

	err = s.svc.Submit(context.Background(), did, sdlManifest)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)

	// Pretend that the hostname has been reserved by a running deployment
	withheld, err := s.hostnames.ReserveHostnames(ctx, util.AllHostnamesOfManifestGroup(sdlManifest.GetGroups()[0]), lid)
	require.NoError(t, err)
	cancel()
	require.Len(t, withheld, 0)

	sdlManifest, err = sdl2NewContainer.Manifest()
	require.NoError(t, err)

	version, err = sdl.ManifestVersion(sdlManifest)
	require.NoError(t, err)

	update := dtypes.EventDeploymentUpdated{
		Context: sdkutil.BaseModuleEvent{},
		ID:      did,
		Version: version,
	}

	err = s.bus.Publish(update)
	require.NoError(t, err)
	time.Sleep(time.Second) // Wait for publish to do its thing

	// Submit the new manifest
	err = s.svc.Submit(context.Background(), did, sdlManifest)
	require.NoError(t, err)

	s.cancel()
	select {
	case <-s.svc.lc.Done():

	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for service shutdown")
	}
}
