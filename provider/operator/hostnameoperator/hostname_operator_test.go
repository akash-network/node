package hostnameoperator

import (
	"context"
	"encoding/json"
	crd "github.com/ovrclk/akash/pkg/apis/akash.network/v2beta1"
	"github.com/ovrclk/akash/provider/cluster/mocks"
	cluster "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	"github.com/ovrclk/akash/provider/operator/operatorcommon"
	"github.com/ovrclk/akash/testutil"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type testHostnameResourceEv struct {
	leaseID      mtypes.LeaseID
	hostname     string
	eventType    cluster.ProviderResourceEvent
	serviceName  string
	externalPort uint32
}

func (ev testHostnameResourceEv) GetLeaseID() mtypes.LeaseID {
	return ev.leaseID
}

func (ev testHostnameResourceEv) GetHostname() string {
	return ev.hostname
}

func (ev testHostnameResourceEv) GetEventType() cluster.ProviderResourceEvent {
	return ev.eventType
}

func (ev testHostnameResourceEv) GetServiceName() string {
	return ev.serviceName
}

func (ev testHostnameResourceEv) GetExternalPort() uint32 {
	return ev.externalPort
}

func TestBuildDirectiveWithDefaults(t *testing.T) {
	ev := testHostnameResourceEv{
		leaseID:      testutil.LeaseID(t),
		hostname:     "foobar.com",
		eventType:    cluster.ProviderResourceAdd, // not relevant in this test
		serviceName:  "some-awesome-service",
		externalPort: 1337,
	}
	directive := buildDirective(ev, crd.ManifestServiceExpose{
		/* Other fields of no consequence in this test */
		HTTPOptions: crd.ManifestServiceExposeHTTPOptions{},
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

func TestBuildDirectiveWithValues(t *testing.T) {
	ev := testHostnameResourceEv{
		leaseID:      testutil.LeaseID(t),
		hostname:     "data.io",
		eventType:    cluster.ProviderResourceAdd, // not relevant in this test
		serviceName:  "some-lame-service",
		externalPort: 22713,
	}
	directive := buildDirective(ev, crd.ManifestServiceExpose{
		/* Other fields of no consequence in this test */
		HTTPOptions: crd.ManifestServiceExposeHTTPOptions{
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
	ctx    context.Context
	cancel context.CancelFunc
	op     *hostnameOperator
	client *mocks.Client
}

func makeHostnameOperatorScaffold(t *testing.T) *hostnameOperatorScaffold {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)

	client := &mocks.Client{}
	scaffold := &hostnameOperatorScaffold{
		ctx:    ctx,
		cancel: cancel,
		client: client,
	}
	l := testutil.Logger(t)

	op, err := newHostnameOperator(l, client, hostnameOperatorConfig{
		pruneInterval:      time.Hour,
		webRefreshInterval: time.Second,
		retryDelay:         time.Second,
	},
		operatorcommon.IgnoreListConfig{
			FailureLimit: 3,
			EntryLimit:   19,
			AgeLimit:     time.Hour,
		})
	require.NoError(t, err)

	scaffold.op = op
	return scaffold
}

func TestHostnameOperatorPrune(t *testing.T) {
	s := makeHostnameOperatorScaffold(t)
	require.NotNil(t, s)
	defer s.cancel()

	s.op.prune() // does nothing, should be fine to call
	s.client.On("GetManifestGroup", mock.Anything, mock.Anything).Return(false, crd.ManifestGroup{}, nil)
	const testIterationCount = 20
	for i := 0; i != testIterationCount; i++ {
		ev := testHostnameResourceEv{
			leaseID:      testutil.LeaseID(t),
			hostname:     "foobar.com",
			eventType:    cluster.ProviderResourceAdd,
			serviceName:  "the-ervice",
			externalPort: 1234,
		}
		err := s.op.applyEvent(s.ctx, ev)
		require.Error(t, err)
	}

	require.Equal(t, s.op.leasesIgnored.Size(), testIterationCount)
	s.op.prune()
	require.Less(t, s.op.leasesIgnored.Size(), testIterationCount)
}

func TestHostnameOperatorApplyDelete(t *testing.T) {
	s := makeHostnameOperatorScaffold(t)
	require.NotNil(t, s)
	defer s.cancel()

	const hostname = "qux.test"
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

	managed := grabManagedHostnames(t, s.op.server.GetRouter().ServeHTTP)
	require.Empty(t, managed)

	err := s.op.applyDeleteEvent(s.ctx, ev)
	require.NoError(t, err)

	_, exists := s.op.hostnames[hostname]
	require.False(t, exists) // Removed from dataset

	require.NoError(t, s.op.server.PrepareAll())
	managed = grabManagedHostnames(t, s.op.server.GetRouter().ServeHTTP)
	require.Empty(t, managed)
}

func TestHostnameOperatorApplyDeleteFails(t *testing.T) {
	s := makeHostnameOperatorScaffold(t)
	require.NotNil(t, s)
	defer s.cancel()

	const hostname = "lsn.test"
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

type ignoreListTestEntry struct {
	FailureCount uint `json:"failure-count"`
}

func grabIgnoredList(t *testing.T, handler http.HandlerFunc) map[string]ignoreListTestEntry {
	req, err := http.NewRequest(http.MethodGet, "/ignore-list", nil)
	require.NoError(t, err)
	rr := httptest.NewRecorder()

	handler(rr, req)

	switch rr.Code {
	case http.StatusNoContent:
		return nil
	case http.StatusOK:

	default:
		t.Fatalf("unknown status code from ignore list endpoint %v", rr.Code)
	}

	decoder := json.NewDecoder(rr.Body)
	data := make(map[string]ignoreListTestEntry)
	err = decoder.Decode(&data)
	require.NoError(t, err)

	return data
}

func grabManagedHostnames(t *testing.T, handler http.HandlerFunc) map[string]interface{} {
	req, err := http.NewRequest(http.MethodGet, "/managed-hostnames", nil)
	require.NoError(t, err)
	rr := httptest.NewRecorder()

	handler(rr, req)

	switch rr.Code {
	case http.StatusNoContent:
		return nil
	case http.StatusOK:

	default:
		t.Fatalf("unknown status code from managed hostnames endpoint %v", rr.Code)
	}

	decoder := json.NewDecoder(rr.Body)
	data := make(map[string]interface{})
	err = decoder.Decode(&data)
	require.NoError(t, err)

	return data
}

func TestHostnameOperatorApplyAddNoManifestGroup(t *testing.T) {
	s := makeHostnameOperatorScaffold(t)
	require.NotNil(t, s)
	defer s.cancel()

	const hostname = "qux.test"

	leaseID := testutil.LeaseID(t)
	ev := testHostnameResourceEv{
		leaseID:      leaseID,
		hostname:     hostname,
		eventType:    cluster.ProviderResourceAdd,
		serviceName:  "the-ervice",
		externalPort: 1234,
	}

	s.client.On("GetManifestGroup", mock.Anything, leaseID).Return(false, crd.ManifestGroup{}, nil)

	ignored := grabIgnoredList(t, s.op.server.GetRouter().ServeHTTP)
	require.NotContains(t, ignored, leaseID.String())

	err := s.op.applyEvent(s.ctx, ev)
	require.Error(t, err)
	require.Regexp(t, "^.*resource not found: manifest.*$", err)

	_, exists := s.op.hostnames[hostname]
	require.False(t, exists) // not added

	require.NoError(t, s.op.server.PrepareAll())
	ignored = grabIgnoredList(t, s.op.server.GetRouter().ServeHTTP)
	require.Contains(t, ignored, leaseID.String())
	require.Equal(t, ignored[leaseID.String()].FailureCount, uint(1))
}

func TestHostnameOperatorApplyAddWithError(t *testing.T) {
	s := makeHostnameOperatorScaffold(t)
	require.NotNil(t, s)
	defer s.cancel()

	const hostname = "zab.test"

	leaseID := testutil.LeaseID(t)
	ev := testHostnameResourceEv{
		leaseID:      leaseID,
		hostname:     hostname,
		eventType:    cluster.ProviderResourceAdd,
		serviceName:  "the-ervice",
		externalPort: 1234,
	}

	s.client.On("GetManifestGroup", mock.Anything, leaseID).Return(false, crd.ManifestGroup{}, io.EOF)
	require.False(t, s.op.leasesIgnored.IsFlagged(leaseID))

	for i := 0; i != 100; i++ {
		err := s.op.applyEvent(s.ctx, ev)
		require.Error(t, err)
		require.ErrorIs(t, err, io.EOF)
	}

	_, exists := s.op.hostnames[hostname]
	require.False(t, exists) // not added

	require.False(t, s.op.leasesIgnored.IsFlagged(leaseID))
}

func TestHostnameOperatorIgnoresAfterLimit(t *testing.T) {
	s := makeHostnameOperatorScaffold(t)
	require.NotNil(t, s)
	defer s.cancel()

	const hostname = "qux.test"

	leaseID := testutil.LeaseID(t)

	s.client.On("GetManifestGroup", mock.Anything, leaseID).Return(false, crd.ManifestGroup{}, nil)

	const testEventCount = 10
	ev := testHostnameResourceEv{
		leaseID:      leaseID,
		hostname:     hostname,
		eventType:    cluster.ProviderResourceAdd,
		serviceName:  "the-ervice",
		externalPort: 1234,
	}

	for i := 0; i != testEventCount; i++ {
		err := s.op.applyEvent(s.ctx, ev)
		if err != nil {
			require.Error(t, err, "iteration %d", i)
			require.Regexp(t, "^.*resource not found: manifest.*$", err, "iteration %d", i)
		}
	}

	_, exists := s.op.hostnames[hostname]
	require.False(t, exists) // not added

	require.True(t, s.op.isEventIgnored(ev))
}

func TestHostnameOperatorApplyAdd(t *testing.T) {
	s := makeHostnameOperatorScaffold(t)
	require.NotNil(t, s)
	defer s.cancel()

	const hostname = "qux.test"

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
		Name: "a-manifest-group",
		Services: []crd.ManifestService{
			{
				Name: "the-service",
				Expose: []crd.ManifestServiceExpose{
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

	managed := grabManagedHostnames(t, s.op.server.GetRouter().ServeHTTP)
	require.Empty(t, managed)

	ignored := grabIgnoredList(t, s.op.server.GetRouter().ServeHTTP)
	require.Empty(t, ignored)

	err := s.op.applyEvent(s.ctx, ev)
	require.NoError(t, err)

	managedValue, exists := s.op.hostnames[hostname]
	require.True(t, exists) // not added
	require.Equal(t, managedValue.presentLease, leaseID)

	require.NoError(t, s.op.server.PrepareAll())

	ignored = grabIgnoredList(t, s.op.server.GetRouter().ServeHTTP)
	require.Empty(t, ignored)

	managed = grabManagedHostnames(t, s.op.server.GetRouter().ServeHTTP)

	require.Contains(t, managed, hostname)
}

func TestHostnameOperatorApplyAddMultipleServices(t *testing.T) {
	s := makeHostnameOperatorScaffold(t)
	require.NotNil(t, s)
	defer s.cancel()

	const hostname = "qux.test"

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
		Port:    3125,
		Service: "public-service",
		Proto:   "TCP",
		Global:  false,
	}
	serviceExposeB := crd.ManifestServiceExpose{
		Port:    31211,
		Service: "public-service",
		Proto:   "UDP",
		Global:  true,
	}
	serviceExposeC := crd.ManifestServiceExpose{
		Port:    22,
		Service: "public-service",
		Proto:   "TCP",
		Global:  true,
	}

	mg := crd.ManifestGroup{
		Name: "a-manifest-group",
		Services: []crd.ManifestService{
			{
				Name: "public-service",
				Expose: []crd.ManifestServiceExpose{
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

	err := s.op.applyEvent(s.ctx, ev)
	require.NoError(t, err)

	managedValue, exists := s.op.hostnames[hostname]
	require.True(t, exists) // not added
	require.Equal(t, managedValue.presentLease, leaseID)
}

func TestHostnameOperatorApplyUpdate(t *testing.T) {
	s := makeHostnameOperatorScaffold(t)
	require.NotNil(t, s)
	defer s.cancel()

	const hostname = "qux.test"

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
		leaseID:      secondLeaseID,
		hostname:     hostname,
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
		Name: "a-manifest-group",
		Services: []crd.ManifestService{
			{
				Name: "the-service",
				Expose: []crd.ManifestServiceExpose{
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
		Name: "some-manifest-group",
		Services: []crd.ManifestService{
			{
				Name: "second-service",
				Expose: []crd.ManifestServiceExpose{
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

	err := s.op.applyEvent(s.ctx, ev)
	require.NoError(t, err)

	managedValue, exists := s.op.hostnames[hostname]
	require.True(t, exists) // not added
	require.Equal(t, managedValue.presentLease, leaseID)

	err = s.op.applyEvent(s.ctx, secondEv)
	require.NoError(t, err)

	managedValue, exists = s.op.hostnames[hostname]
	require.True(t, exists) // not added
	require.Equal(t, managedValue.presentLease, secondLeaseID)
}
