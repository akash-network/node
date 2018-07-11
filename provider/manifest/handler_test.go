package manifest_test

import (
	"context"
	"testing"
	"time"

	manifestUtil "github.com/ovrclk/akash/manifest"
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
		h manifest.Service,
		bus event.Bus,
		mreq *types.ManifestRequest,
		lease *types.Lease,
		dgroup *types.DeploymentGroup) {

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		donech := make(chan struct{})

		go func() {
			defer close(donech)
			assert.NoError(t, h.HandleManifest(ctx, mreq))
		}()

		bus.Publish(event.LeaseWon{
			LeaseID: lease.LeaseID,
			Group:   dgroup,
			Price:   20,
		})

		<-donech
	})
}

func TestHandler_leaseFirst(t *testing.T) {
	withHandler(t, func(
		h manifest.Service,
		bus event.Bus,
		mreq *types.ManifestRequest,
		lease *types.Lease,
		dgroup *types.DeploymentGroup) {

		bus.Publish(event.LeaseWon{
			LeaseID: lease.LeaseID,
			Group:   dgroup,
			Price:   20,
		})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		require.NoError(t, h.HandleManifest(ctx, mreq))

		status, err := h.Status(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, status)
	})
}

type testfn func(manifest.Service, event.Bus, *types.ManifestRequest, *types.Lease, *types.DeploymentGroup)

func withHandler(t *testing.T, fn testfn) {
	info, kmgr := testutil.NewNamedKey(t)
	signer := testutil.Signer(t, kmgr)
	tenant := info.GetPubKey().Address().Bytes()

	deployment := testutil.Deployment(tenant, 1)
	dgroups := testutil.DeploymentGroups(deployment.Address, 2)
	dgroup := dgroups.Items[0]
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
	client.
		On("Deployment", mock.Anything, []byte(deployment.Address)).
		Return(deployment, nil)
	client.
		On("DeploymentGroupsForDeployment", mock.Anything, []byte(deployment.Address)).
		Return(dgroups, nil)
	client.
		On("ProviderLeases", mock.Anything, []uint8(provider.Address)).
		Return(&types.Leases{}, nil)

	sess := session.New(testutil.Logger(), provider, nil, client)

	h, err := manifest.NewHandler(ctx, sess, bus)
	require.NoError(t, err)

	mani := &types.Manifest{
		Groups: testutil.ManifestGroupsForDeploymentGroups(t, dgroups.Items),
	}

	mreq := &types.ManifestRequest{
		Deployment: deployment.Address,
		Manifest:   mani,
	}

	mreq, _, err = manifestUtil.SignManifest(mani, signer, deployment.Address)
	require.NoError(t, err)

	vsn, err := manifestUtil.Hash(mani)
	require.NoError(t, err)

	deployment.Version = vsn

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
