package ipoperator

import (
	"context"
	manifest "github.com/ovrclk/akash/manifest/v2beta1"
	"github.com/ovrclk/akash/provider/cluster/mocks"
	"github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	"github.com/ovrclk/akash/provider/operator/operatorcommon"
	"github.com/ovrclk/akash/testutil"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"testing"
	"time"
)

type ipOperatorScaffold struct {
	op *ipOperator

	clusterMock *mocks.Client
	metalMock   *mocks.MetalLBClient

	ilc operatorcommon.IgnoreListConfig
}

func runIPOperator(t *testing.T, run bool, prerun, fn func(ctx context.Context, s ipOperatorScaffold)) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	providerAddr := testutil.AccAddress(t)

	l := testutil.Logger(t)
	client := &mocks.Client{}
	mllbc := &mocks.MetalLBClient{}
	mllbc.On("Stop")
	poolChangesMock := make(chan struct{})
	// nolint: golint, gosimple
	var poolChangesInterface <-chan struct{}
	poolChangesInterface = poolChangesMock
	mllbc.On("DetectPoolChanges", mock.Anything).Return(poolChangesInterface, nil)

	ilc := operatorcommon.IgnoreListConfig{
		FailureLimit: 100,
		EntryLimit:   9999,
		AgeLimit:     time.Hour,
	}
	opcfg := operatorcommon.OperatorConfig{
		PruneInterval:      time.Second,
		WebRefreshInterval: time.Second,
		RetryDelay:         time.Second,
		ProviderAddress:    providerAddr.String(),
	}
	op, err := newIPOperator(l, client, opcfg, ilc, mllbc)

	require.NoError(t, err)
	require.NotNil(t, op)

	s := ipOperatorScaffold{
		op:          op,
		metalMock:   mllbc,
		clusterMock: client,
		ilc:         ilc,
	}

	if run {
		if prerun != nil {
			prerun(ctx, s)
		}
		done := make(chan error)
		go func() {
			defer close(done)
			done <- op.run(ctx)
		}()

		fn(ctx, s)
		cancel()

		select {
		case err = <-done:
			require.Error(t, err)
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(10 * time.Second):
			t.Fatal("timed out waiting for ip operator to stop")
		}
	} else {
		fn(ctx, s)
	}
}

type fakeIPEvent struct {
	leaseID      mtypes.LeaseID
	externalPort uint32
	port         uint32
	sharingKey   string
	serviceName  string
	protocol     manifest.ServiceProtocol
	eventType    v1beta2.ProviderResourceEvent
}

func (fipe fakeIPEvent) GetLeaseID() mtypes.LeaseID {
	return fipe.leaseID
}
func (fipe fakeIPEvent) GetExternalPort() uint32 {
	return fipe.externalPort
}
func (fipe fakeIPEvent) GetPort() uint32 {
	return fipe.port
}

func (fipe fakeIPEvent) GetSharingKey() string {
	return fipe.sharingKey
}

func (fipe fakeIPEvent) GetServiceName() string {
	return fipe.serviceName
}

func (fipe fakeIPEvent) GetProtocol() manifest.ServiceProtocol {
	return fipe.protocol
}

func (fipe fakeIPEvent) GetEventType() v1beta2.ProviderResourceEvent {
	return fipe.eventType
}

func TestIPOperatorAddEvent(t *testing.T) {
	runIPOperator(t, false, nil, func(ctx context.Context, s ipOperatorScaffold) {
		require.NotNil(t, s.op)
		leaseID := testutil.LeaseID(t)

		s.metalMock.On("CreateIPPassthrough", mock.Anything,
			v1beta2.ClusterIPPassthroughDirective{
				LeaseID:      leaseID,
				ServiceName:  "aservice",
				Port:         10000,
				ExternalPort: 10001,
				SharingKey:   "akey",
				Protocol:     "TCP",
			}).Return(nil)

		err := s.op.applyEvent(ctx, fakeIPEvent{
			leaseID:      leaseID,
			externalPort: 10001,
			port:         10000,
			sharingKey:   "akey",
			serviceName:  "aservice",
			protocol:     manifest.TCP,
			eventType:    v1beta2.ProviderResourceAdd,
		})
		require.NoError(t, err)
	})
}

// Add for updating to a different lease
func TestIPOperatorUpdateEvent(t *testing.T) {
	runIPOperator(t, false, nil, func(ctx context.Context, s ipOperatorScaffold) {
		require.NotNil(t, s.op)
		leaseID := testutil.LeaseID(t)

		s.metalMock.On("CreateIPPassthrough", mock.Anything,
			v1beta2.ClusterIPPassthroughDirective{
				LeaseID:      leaseID,
				ServiceName:  "aservice",
				Port:         10000,
				ExternalPort: 10001,
				SharingKey:   "akey",
				Protocol:     "TCP",
			}).Return(nil)

		err := s.op.applyEvent(ctx, fakeIPEvent{
			leaseID:      leaseID,
			externalPort: 10001,
			port:         10000,
			sharingKey:   "akey",
			serviceName:  "aservice",
			protocol:     manifest.TCP,
			eventType:    v1beta2.ProviderResourceUpdate,
		})
		require.NoError(t, err)
	})
}

func TestIPOperatorDeleteEvent(t *testing.T) {
	runIPOperator(t, false, nil, func(ctx context.Context, s ipOperatorScaffold) {
		require.NotNil(t, s.op)
		leaseID := testutil.LeaseID(t)

		s.metalMock.On("PurgeIPPassthrough", mock.Anything,
			v1beta2.ClusterIPPassthroughDirective{
				LeaseID:      leaseID,
				ServiceName:  "aservice",
				Port:         10000,
				ExternalPort: 10001,
				SharingKey:   "akey",
				Protocol:     "TCP",
			}).Return(nil)

		err := s.op.applyEvent(ctx, fakeIPEvent{
			leaseID:      leaseID,
			externalPort: 10001,
			port:         10000,
			sharingKey:   "akey",
			serviceName:  "aservice",
			protocol:     manifest.TCP,
			eventType:    v1beta2.ProviderResourceDelete,
		})
		require.NoError(t, err)
	})
}

func TestIPOperatorGivesUpOnErrors(t *testing.T) {
	var fakeError = kubeErrors.NewNotFound(schema.GroupResource{
		Group:    "thegroup",
		Resource: "theresource",
	}, "bob")
	runIPOperator(t, false, nil, func(ctx context.Context, s ipOperatorScaffold) {
		require.NotNil(t, s.op)
		leaseID := testutil.LeaseID(t)

		s.metalMock.On("CreateIPPassthrough", mock.Anything,
			v1beta2.ClusterIPPassthroughDirective{
				LeaseID:      leaseID,
				ServiceName:  "aservice",
				Port:         10000,
				ExternalPort: 10001,
				SharingKey:   "akey",
				Protocol:     "TCP",
			}).Return(fakeError).Times(int(s.ilc.FailureLimit))

		require.Greater(t, s.ilc.FailureLimit, uint(0))

		fakeEvent := fakeIPEvent{
			leaseID:      leaseID,
			externalPort: 10001,
			port:         10000,
			sharingKey:   "akey",
			serviceName:  "aservice",
			protocol:     manifest.TCP,
			eventType:    v1beta2.ProviderResourceAdd,
		}
		for i := uint(0); i != s.ilc.FailureLimit; i++ {
			err := s.op.applyEvent(ctx, fakeEvent)
			require.ErrorIs(t, err, fakeError)
		}

		err := s.op.applyEvent(ctx, fakeEvent)
		require.NoError(t, err) // Nothing happens because this is ignored
	})
}

func TestIPOperatorRun(t *testing.T) {
	leaseID := testutil.LeaseID(t)
	waitForEventRead := make(chan struct{}, 1)
	runIPOperator(t, true, func(ctx context.Context, s ipOperatorScaffold) {
		s.metalMock.On("GetIPPassthroughs", mock.Anything).Return(nil, nil)
		s.metalMock.On("GetIPAddressUsage", mock.Anything).Return(uint(0), uint(3), nil)
		events := make(chan v1beta2.IPResourceEvent)
		go func() {
			select {
			case events <- fakeIPEvent{
				leaseID:      leaseID,
				externalPort: 100,
				port:         101,
				sharingKey:   "akey",
				serviceName:  "aservice",
				protocol:     "UDP",
				eventType:    v1beta2.ProviderResourceAdd,
			}:
			case <-ctx.Done():
				return
			}
			close(events)
			select {
			case waitForEventRead <- struct{}{}:
			default:
			}
		}()
		// nolint: golint, gosimple
		var eventsRead <-chan v1beta2.IPResourceEvent
		eventsRead = events
		s.clusterMock.On("ObserveIPState", mock.Anything).Return(eventsRead, nil)

		s.metalMock.On("CreateIPPassthrough", mock.Anything,
			v1beta2.ClusterIPPassthroughDirective{
				LeaseID:      leaseID,
				ServiceName:  "aservice",
				Port:         101,
				ExternalPort: 100,
				SharingKey:   "akey",
				Protocol:     manifest.UDP,
			}).Return(nil)

	}, func(ctx context.Context, s ipOperatorScaffold) {
		require.NotNil(t, s.op)

		select {
		case <-waitForEventRead:
		case <-ctx.Done():
			t.Fatalf("timeout waiting for event read")
		}
	})
}
