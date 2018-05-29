package manifest_test

import (
	"context"
	"testing"
	"time"

	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/manifest"
	"github.com/ovrclk/akash/provider/session"
	qmocks "github.com/ovrclk/akash/query/mocks"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestHandler_manifestFirst(t *testing.T) {
	withHandler(t, func(
		h manifest.Handler,
		bus event.Bus,
		mreq *types.ManifestRequest,
		lease *types.Lease,
		dgroup *types.DeploymentGroup) {

		require.NoError(t, h.HandleManifest(mreq))
		bus.Publish(event.LeaseWon{
			LeaseID: lease.LeaseID,
			Group:   dgroup,
			Price:   20,
		})
	})
}

func TestHandler_leaseFirst(t *testing.T) {
	withHandler(t, func(
		h manifest.Handler,
		bus event.Bus,
		mreq *types.ManifestRequest,
		lease *types.Lease,
		dgroup *types.DeploymentGroup) {

		bus.Publish(event.LeaseWon{
			LeaseID: lease.LeaseID,
			Group:   dgroup,
			Price:   20,
		})

		require.NoError(t, h.HandleManifest(mreq))
	})
}

type testfn func(manifest.Handler, event.Bus, *types.ManifestRequest, *types.Lease, *types.DeploymentGroup)

func withHandler(t *testing.T, fn testfn) {
	tenant := testutil.Address(t)
	deployment := testutil.Deployment(tenant, 1)
	dgroup := testutil.DeploymentGroups(deployment.Address, 2).Items[0]
	order := testutil.Order(deployment.Address, dgroup.Seq, 3)

	providerID := testutil.Address(t)
	provider := testutil.Provider(providerID, 4)

	lease := testutil.Lease(providerID, deployment.Address, order.Group, order.Seq, 10)

	bus := event.NewBus()
	defer bus.Close()

	sub, err := bus.Subscribe()
	require.NoError(t, err)
	defer sub.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &qmocks.Client{}
	client.On("Deployment",
		mock.Anything,
		[]byte(deployment.Address)).Return(deployment, nil)

	sess := session.New(testutil.Logger(), provider, nil, client)

	h, err := manifest.NewHandler(ctx, sess, bus)
	require.NoError(t, err)

	mani := &types.Manifest{}
	mreq := &types.ManifestRequest{
		Deployment: deployment.Address,
		Manifest:   mani,
	}

	fn(h, bus, mreq, lease, dgroup)

	timer := time.NewTimer(time.Second)
	defer timer.Stop()

	found := false
	for !found {
		select {
		case ev := <-sub.Events():
			if ev, ok := ev.(event.ManifestReceived); ok {
				found = assert.Equal(t, ev.LeaseID, lease.LeaseID)
			}
		case <-timer.C:
			require.Fail(t, "event not found")
		}
	}

}
