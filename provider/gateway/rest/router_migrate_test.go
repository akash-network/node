package rest

import (
	"context"
	v1 "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
	"time"
)

func TestRouteMigrateHostnameDoesNotExist(t *testing.T) {
	runRouterTest(t, true, func(test *routerTest) {
		const dseq = uint64(33)
		const gseq = uint32(34)

		test.clusterService.On("FindActiveLease", mock.Anything, mock.Anything, dseq, gseq).Return(false, cluster.ActiveLease{}, nil)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
		defer cancel()
		err := test.gclient.MigrateHostnames(ctx, []string{"foobar.com"}, dseq, gseq)
		require.Error(t, err)
		require.IsType(t, ClientResponseError{}, err)
		require.Regexp(t, `(?s)^.*destination deployment does not exist.*$`, err.(ClientResponseError).ClientError())
	})
}

func TestRouteMigrateHostnameDeploymentDoesNotUse(t *testing.T) {
	runRouterTest(t, true, func(test *routerTest) {
		const dseq = uint64(133)
		const gseq = uint32(134)

		lease := cluster.ActiveLease{
			ID:    testutil.LeaseID(t),
			Group: v1.ManifestGroup{},
		}
		lease.ID.Owner = test.caddr.String()
		test.clusterService.On("FindActiveLease", mock.Anything, mock.Anything, dseq, gseq).Return(true, lease, nil)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
		defer cancel()
		err := test.gclient.MigrateHostnames(ctx, []string{"foobar.org"}, dseq, gseq)
		require.Error(t, err)
		require.IsType(t, ClientResponseError{}, err)
		require.Regexp(t, `(?s)^.*the hostname "foobar.org" is not used by this deployment.*$`, err.(ClientResponseError).ClientError())
	})
}

func TestRouteMigrateHostname(t *testing.T) {
	const hostname = "kittens-purr.io"
	const dseq = uint64(133)
	const gseq = uint32(134)
	const serviceName = "hostly-service"
	const serviceExternalPort = uint32(1111)

	runRouterTest(t, true, func(test *routerTest) {
		lease  := cluster.ActiveLease{
			ID:    testutil.LeaseID(t),
			Group: v1.ManifestGroup{
				Name:     "some-group",
				Services: []v1.ManifestService{
					v1.ManifestService{
						Name:      serviceName,
						Image:     "some-awesome-image",
						Count:     1,
						Expose:    []v1.ManifestServiceExpose{
							v1.ManifestServiceExpose{
								Port:         1234,
								ExternalPort: uint16(serviceExternalPort),
								Proto:        "TCP",
								Service:      serviceName,
								Global:       true,
								Hosts:        []string{"dogs.pet", hostname},
								/* Remaining fields not relevant in this test */
							},
						},
					},
				},
			},
		}
		lease.ID.Owner = test.caddr.String()
		test.clusterService.On("FindActiveLease", mock.Anything, mock.Anything, dseq, gseq).Return(true, lease, nil)
		test.hostnameClient.On("PrepareHostnamesForTransfer", mock.Anything, []string{hostname}, lease.ID).Return(nil)
		test.clusterService.On("TransferHostname", mock.Anything, lease.ID, hostname, serviceName, serviceExternalPort).Return(nil)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
		defer cancel()
		err := test.gclient.MigrateHostnames(ctx, []string{hostname}, dseq, gseq)
		require.NoError(t, err)

		require.Equal(t, 2, len(test.clusterService.Calls))
		require.Equal(t, "TransferHostname", test.clusterService.Calls[1].Method)
	})
}

func TestRouteMigrateHostnamePrepareFails(t *testing.T) {
	const hostname = "alphabet-soup.io"
	const dseq = uint64(7133)
	const gseq = uint32(7134)
	const serviceName = "hostly-service"
	const serviceExternalPort = uint32(999)

	runRouterTest(t, true, func(test *routerTest) {
		lease  := cluster.ActiveLease{
			ID:    testutil.LeaseID(t),
			Group: v1.ManifestGroup{
				Name:     "some-group",
				Services: []v1.ManifestService{
					v1.ManifestService{
						Name:      serviceName,
						Image:     "some-awesome-image",
						Count:     1,
						Expose:    []v1.ManifestServiceExpose{
							v1.ManifestServiceExpose{
								Port:         1234,
								ExternalPort: uint16(serviceExternalPort),
								Proto:        "TCP",
								Service:      serviceName,
								Global:       true,
								Hosts:        []string{"dogs.pet", hostname},
								/* Remaining fields not relevant in this test */
							},
						},
					},
				},
			},
		}
		lease.ID.Owner = test.caddr.String()
		test.clusterService.On("FindActiveLease", mock.Anything, mock.Anything, dseq, gseq).Return(true, lease, nil)
		test.hostnameClient.On("PrepareHostnamesForTransfer", mock.Anything, []string{hostname}, lease.ID).Return(io.EOF)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
		defer cancel()
		err := test.gclient.MigrateHostnames(ctx, []string{hostname}, dseq, gseq)
		require.Error(t, err)
		require.Regexp(t, `^.*remote server returned 500.*$`, err)
		require.Equal(t, 1, len(test.clusterService.Calls))
	})
}

func TestRouteMigrateHostnameTransferFails(t *testing.T) {
	const hostname = "moo.snake"
	const dseq = uint64(333)
	const gseq = uint32(334)
	const serviceName = "decloud-thing"
	const serviceExternalPort = uint32(1112)

	runRouterTest(t, true, func(test *routerTest) {
		lease  := cluster.ActiveLease{
			ID:    testutil.LeaseID(t),
			Group: v1.ManifestGroup{
				Name:     "some-group",
				Services: []v1.ManifestService{
					v1.ManifestService{
						Name:      serviceName,
						Image:     "some-awesome-image",
						Count:     1,
						Expose:    []v1.ManifestServiceExpose{
							v1.ManifestServiceExpose{
								Port:         1234,
								ExternalPort: uint16(serviceExternalPort),
								Proto:        "TCP",
								Service:      serviceName,
								Global:       true,
								Hosts:        []string{"dogs.pet", hostname},
								/* Remaining fields not relevant in this test */
							},
						},
					},
				},
			},
		}
		lease.ID.Owner = test.caddr.String()
		test.clusterService.On("FindActiveLease", mock.Anything, mock.Anything, dseq, gseq).Return(true, lease, nil)
		test.hostnameClient.On("PrepareHostnamesForTransfer", mock.Anything, []string{hostname}, lease.ID).Return(nil)
		test.clusterService.On("TransferHostname", mock.Anything, lease.ID, hostname, serviceName, serviceExternalPort).Return(io.EOF)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
		defer cancel()
		err := test.gclient.MigrateHostnames(ctx, []string{hostname}, dseq, gseq)
		require.Error(t, err)
		require.Regexp(t, `^.*remote server returned 500.*$`, err)
		require.Equal(t, 2, len(test.clusterService.Calls))
	})
}