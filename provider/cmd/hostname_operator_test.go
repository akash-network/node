package cmd

import (
	"context"
	"github.com/ovrclk/akash/provider/cluster/mocks"
	crd "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
	cluster "github.com/ovrclk/akash/provider/cluster/types"
	"github.com/ovrclk/akash/testutil"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
	"time"
)

type testHostnameResourceEv struct {
	leaseID mtypes.LeaseID
	hostname string
	eventType cluster.ProviderResourceEvent
	serviceName string
	externalPort uint32
}

func (ev testHostnameResourceEv) GetLeaseID() mtypes.LeaseID {
	return ev.leaseID
}

func (ev testHostnameResourceEv) GetHostname() string {
	return ev.hostname
}

func (ev testHostnameResourceEv) GetEventType() cluster.ProviderResourceEvent{
	return ev.eventType
}

func (ev testHostnameResourceEv) GetServiceName() string {
	return ev.serviceName
}

func (ev testHostnameResourceEv) GetExternalPort() uint32 {
	return ev.externalPort
}

func TestBuildDirectiveWithDefaults(t *testing.T){
	ev := testHostnameResourceEv{
		leaseID:      testutil.LeaseID(t),
		hostname:     "foobar.com",
		eventType:    cluster.ProviderResourceAdd, // not relevant in this test
		serviceName:  "some-awesome-service",
		externalPort: 1337,
	}
	directive := buildDirective(ev, crd.ManifestServiceExpose{
		/* Other fields of no consequence in this test */
		HTTPOptions:  crd.ManifestServiceExposeHTTPOptions{},
	})

	require.Equal(t, directive.LeaseID, ev.leaseID)
	require.Equal(t, directive.ServiceName, ev.serviceName)
	require.Equal(t, directive.ServicePort, int32(ev.externalPort))
	require.Equal(t, directive.Hostname, ev.hostname)

	require.Equal(t, directive.ReadTimeout, uint32(60000))
	require.Equal(t, directive.SendTimeout, uint32(60000))
	require.Equal(t, directive.NextTimeout, uint32(60000))
	require.Equal(t, directive.MaxBodySize, uint32(1048576))
	require.Equal(t, directive.NextTries, uint32(3))
	require.Equal(t, directive.NextCases, []string{"error", "timeout"})
}

func TestBuildDirectiveWithValues(t *testing.T){
	ev := testHostnameResourceEv{
		leaseID:      testutil.LeaseID(t),
		hostname:     "data.io",
		eventType:    cluster.ProviderResourceAdd, // not relevant in this test
		serviceName:  "some-lame-service",
		externalPort: 22713,
	}
	directive := buildDirective(ev, crd.ManifestServiceExpose{
		/* Other fields of no consequence in this test */
		HTTPOptions:  crd.ManifestServiceExposeHTTPOptions{
			MaxBodySize: 1,
			ReadTimeout: 2,
			SendTimeout: 3,
			NextTries:   4,
			NextTimeout: 5,
			NextCases:   []string{"none"},
		},
	})

	require.Equal(t, directive.LeaseID, ev.leaseID)
	require.Equal(t, directive.ServiceName, ev.serviceName)
	require.Equal(t, directive.ServicePort, int32(ev.externalPort))
	require.Equal(t, directive.Hostname, ev.hostname)

	require.Equal(t, directive.MaxBodySize, uint32(1))
	require.Equal(t, directive.ReadTimeout, uint32(2))
	require.Equal(t, directive.SendTimeout, uint32(3))
	require.Equal(t, directive.NextTries, uint32(4))
	require.Equal(t, directive.NextTimeout, uint32(5))
	require.Equal(t, directive.NextCases, []string{"none"})
}

type hostnameOperatorScaffold struct {
	ctx context.Context
	cancel context.CancelFunc
	op *hostnameOperator
	client *mocks.Client
}

func makeHostnameOperatorScaffold(t *testing.T) *hostnameOperatorScaffold{
	ctx, cancel := context.WithTimeout(context.Background(), time.Second * 20)

	client := &mocks.Client{}
	scaffold := &hostnameOperatorScaffold{
		ctx : ctx,
		cancel: cancel,
		client: client,
	}
	l := testutil.Logger(t)


	op := &hostnameOperator{
		hostnames: make(map[string]managedHostname),
		client:    client,
		log:       l,
	}

	scaffold.op = op

	return scaffold
}

func TestHostnameOperatorApplyDelete(t *testing.T){
	s := makeHostnameOperatorScaffold(t)
	require.NotNil(t, s)
	defer s.cancel()

	const hostname = "qux.xyz"
	// Shove in something so we can confirm it is deleted
	s.op.hostnames[hostname] = managedHostname{}

	leaseID := testutil.LeaseID(t)
	ev := testHostnameResourceEv{
		leaseID:      leaseID,
		hostname:     hostname,
		eventType:    cluster.ProviderResourceDelete,
		serviceName:  "the-ervice",
		externalPort: 1234,
	}

	// Simulate everything working fine
	s.client.On("RemoveHostnameFromDeployment", mock.Anything, hostname, leaseID, true).Return(nil)

	err := s.op.applyDeleteEvent(s.ctx, ev)
	require.NoError(t, err)

	_, exists := s.op.hostnames[hostname]
	require.False(t, exists) // Removed from dataset
}

func TestHostnameOperatorApplyDeleteFails(t *testing.T){
	s := makeHostnameOperatorScaffold(t)
	require.NotNil(t, s)
	defer s.cancel()

	const hostname = "qux.io"
	// Shove in something so we can confirm it is deleted
	s.op.hostnames[hostname] = managedHostname{}

	leaseID := testutil.LeaseID(t)
	ev := testHostnameResourceEv{
		leaseID:      leaseID,
		hostname:     hostname,
		eventType:    cluster.ProviderResourceDelete,
		serviceName:  "the-ervice",
		externalPort: 1234,
	}

	// Simulate something failing
	s.client.On("RemoveHostnameFromDeployment", mock.Anything, hostname, leaseID, true).Return(io.EOF)

	err := s.op.applyDeleteEvent(s.ctx, ev)
	require.Error(t, err)

	_, exists := s.op.hostnames[hostname]
	require.True(t, exists) // Still in dataset
}

func TestHostnameOperatorApplyAddNoManifestGroup(t *testing.T){
	s := makeHostnameOperatorScaffold(t)
	require.NotNil(t, s)
	defer s.cancel()

	const hostname = "qux.io"

	leaseID := testutil.LeaseID(t)
	ev := testHostnameResourceEv{
		leaseID:      leaseID,
		hostname:     hostname,
		eventType:    cluster.ProviderResourceAdd,
		serviceName:  "the-ervice",
		externalPort: 1234,
	}

	s.client.On("GetManifestGroup", mock.Anything, leaseID).Return(false, crd.ManifestGroup{}, nil)

	err := s.op.applyAddOrUpdateEvent(s.ctx, ev)
	require.Error(t, err)
	require.Regexp(t, "^.*no manifest found.*$", err)

	_, exists := s.op.hostnames[hostname]
	require.False(t, exists) // not added
}

func TestHostnameOperatorApplyAdd(t *testing.T){
	s := makeHostnameOperatorScaffold(t)
	require.NotNil(t, s)
	defer s.cancel()

	const hostname = "qux.io"

	const externalPort = 41333
	leaseID := testutil.LeaseID(t)
	ev := testHostnameResourceEv{
		leaseID:      leaseID,
		hostname:     hostname,
		eventType:    cluster.ProviderResourceAdd,
		serviceName:  "the-service",
		externalPort: externalPort,
	}

	serviceExpose := crd.ManifestServiceExpose{
		Port:         3321,
		ExternalPort: externalPort,
		Proto:        "TCP",
		Service:      "the-service",
		Global:       true,
		Hosts:        []string{hostname},
	}
	mg := crd.ManifestGroup{
		Name:     "a-manifest-group",
		Services: []crd.ManifestService{
			crd.ManifestService{
				Name:      "the-service",
				Expose:    []crd.ManifestServiceExpose{
					serviceExpose,
				},
				Count: 1,
				/* Other fields not relevant in this test */
			},
		},

	}
	s.client.On("GetManifestGroup", mock.Anything, leaseID).Return(true, mg, nil)
	directive := buildDirective(ev, serviceExpose) // result tested in other unit tests
	s.client.On("ConnectHostnameToDeployment", mock.Anything, directive).Return(nil)

	err := s.op.applyAddOrUpdateEvent(s.ctx, ev)
	require.NoError(t, err)

	managedValue, exists := s.op.hostnames[hostname]
	require.True(t, exists) // not added
	require.Equal(t, managedValue.presentLease, leaseID)
}

func TestHostnameOperatorApplyAddMultipleServices(t *testing.T){
	s := makeHostnameOperatorScaffold(t)
	require.NotNil(t, s)
	defer s.cancel()

	const hostname = "qux.io"

	const externalPort = 41333
	leaseID := testutil.LeaseID(t)
	ev := testHostnameResourceEv{
		leaseID:      leaseID,
		hostname:     hostname,
		eventType:    cluster.ProviderResourceAdd,
		serviceName:  "public-service",
		externalPort: externalPort,
	}

	serviceExpose := crd.ManifestServiceExpose{
		Port:         3321,
		ExternalPort: externalPort,
		Proto:        "TCP",
		Service:      "public-service",
		Global:       true,
		Hosts:        []string{hostname},
	}
	serviceExposeA := crd.ManifestServiceExpose{
		Port: 3125,
		Service: "public-service",
		Proto: "TCP",
		Global: false,
	}
	serviceExposeB := crd.ManifestServiceExpose{
		Port: 31211,
		Service: "public-service",
		Proto: "UDP",
		Global: true,
	}
	serviceExposeC := crd.ManifestServiceExpose{
		Port: 22,
		Service: "public-service",
		Proto: "TCP",
		Global: true,
	}

	mg := crd.ManifestGroup{
		Name:     "a-manifest-group",
		Services: []crd.ManifestService{
			crd.ManifestService{
				Name:      "public-service",
				Expose:    []crd.ManifestServiceExpose{
					serviceExposeA,
					serviceExposeB,
					serviceExposeC,
					serviceExpose,
				},
				Count: 1,
				/* Other fields not relevant in this test */
			},
		},

	}
	s.client.On("GetManifestGroup", mock.Anything, leaseID).Return(true, mg, nil)
	directive := buildDirective(ev, serviceExpose) // result tested in other unit tests
	s.client.On("ConnectHostnameToDeployment", mock.Anything, directive).Return(nil)

	err := s.op.applyAddOrUpdateEvent(s.ctx, ev)
	require.NoError(t, err)

	managedValue, exists := s.op.hostnames[hostname]
	require.True(t, exists) // not added
	require.Equal(t, managedValue.presentLease, leaseID)
}

func TestHostnameOperatorApplyUpdate(t *testing.T){
	s := makeHostnameOperatorScaffold(t)
	require.NotNil(t, s)
	defer s.cancel()

	const hostname = "qux.io"

	const externalPort = 41333
	leaseID := testutil.LeaseID(t)
	secondLeaseID := testutil.LeaseID(t)

	ev := testHostnameResourceEv{
		leaseID:      leaseID,
		hostname:     hostname,
		eventType:    cluster.ProviderResourceAdd,
		serviceName:  "the-service",
		externalPort: externalPort,
	}

	const secondExternalPort = 55100
	secondEv := testHostnameResourceEv{
		leaseID: secondLeaseID,
		hostname: hostname,
		eventType:    cluster.ProviderResourceUpdate,
		serviceName:  "second-service",
		externalPort: secondExternalPort,
	}

	serviceExpose := crd.ManifestServiceExpose{
		Port:         3321,
		ExternalPort: externalPort,
		Proto:        "TCP",
		Service:      "the-service",
		Global:       true,
		Hosts:        []string{hostname},
	}
	mg := crd.ManifestGroup{
		Name:     "a-manifest-group",
		Services: []crd.ManifestService{
			crd.ManifestService{
				Name:      "the-service",
				Expose:    []crd.ManifestServiceExpose{
					serviceExpose,
				},
				Count: 1,
				/* Other fields not relevant in this test */
			},
		},
	}
	secondServiceExpose := crd.ManifestServiceExpose{
		Port:         11111,
		ExternalPort: secondExternalPort,
		Proto:        "TCP",
		Service:      "second-serivce",
		Global:       true,
		Hosts:        []string{hostname},
	}
	mg2 := crd.ManifestGroup{
		Name:     "some-manifest-group",
		Services: []crd.ManifestService{
			crd.ManifestService{
				Name:      "second-service",
				Expose:    []crd.ManifestServiceExpose{
					secondServiceExpose,
				},
				Count: 4000,
				/* Other fields not relevant in this test */
			},
		},
	}

	s.client.On("GetManifestGroup", mock.Anything, leaseID).Return(true, mg, nil)
	s.client.On("GetManifestGroup", mock.Anything, secondLeaseID).Return(true, mg2, nil)

	directive := buildDirective(ev, serviceExpose) // result tested in other unit tests
	s.client.On("ConnectHostnameToDeployment", mock.Anything, directive).Return(nil)
	secondDirective := buildDirective(secondEv, secondServiceExpose) // result tested in other unit tests
	s.client.On("ConnectHostnameToDeployment", mock.Anything, secondDirective).Return(nil)

	s.client.On("RemoveHostnameFromDeployment", mock.Anything, hostname, leaseID, false).Return(nil)

	err := s.op.applyAddOrUpdateEvent(s.ctx, ev)
	require.NoError(t, err)

	managedValue, exists := s.op.hostnames[hostname]
	require.True(t, exists) // not added
	require.Equal(t, managedValue.presentLease, leaseID)

	err = s.op.applyAddOrUpdateEvent(s.ctx, secondEv)
	require.NoError(t, err)

	managedValue, exists = s.op.hostnames[hostname]
	require.True(t, exists) // not added
	require.Equal(t, managedValue.presentLease, secondLeaseID)
}