// nolint: goerr113
package rest

import (
	"context"
	"crypto/tls"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/mock"

	qmock "github.com/ovrclk/akash/client/mocks"
	akashmanifest "github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider"
	pcmock "github.com/ovrclk/akash/provider/cluster/mocks"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	pmmock "github.com/ovrclk/akash/provider/manifest/mocks"
	pmock "github.com/ovrclk/akash/provider/mocks"
	"github.com/ovrclk/akash/testutil"
	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	providertypes "github.com/ovrclk/akash/x/provider/types/v1beta2"
)

func Test_router_Status(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		expected := &provider.Status{}
		addr := testutil.AccAddress(t)
		mocks := createMocks()

		mocks.pclient.On("Status", mock.Anything).Return(expected, nil)
		withServer(t, addr, mocks.pclient, mocks.qclient, nil, func(host string) {
			client, err := NewClient(mocks.qclient, addr, nil)
			assert.NoError(t, err)
			result, err := client.Status(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, expected, result)
		})
		mocks.pclient.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		addr := testutil.AccAddress(t)
		mocks := createMocks()
		mocks.pclient.On("Status", mock.Anything).Return(nil, errors.New("oops"))
		withServer(t, addr, mocks.pclient, mocks.qclient, nil, func(host string) {
			client, err := NewClient(mocks.qclient, addr, nil)
			assert.NoError(t, err)
			_, err = client.Status(context.Background())
			assert.Error(t, err)
		})
		mocks.pclient.AssertExpectations(t)
	})
}

func Test_router_Validate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		expected := provider.ValidateGroupSpecResult{
			MinBidPrice: testutil.AkashDecCoin(t, 200),
		}
		addr := testutil.AccAddress(t)
		mocks := createMocks()
		mocks.pclient.On("Validate", mock.Anything, mock.Anything).Return(expected, nil)
		withServer(t, addr, mocks.pclient, mocks.qclient, nil, func(host string) {
			client, err := NewClient(mocks.qclient, addr, nil)
			assert.NoError(t, err)
			result, err := client.Validate(context.Background(), testutil.GroupSpec(t))
			assert.NoError(t, err)
			assert.Equal(t, expected, result)
		})
		mocks.pclient.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		addr := testutil.AccAddress(t)
		mocks := createMocks()
		mocks.pclient.On("Validate", mock.Anything, mock.Anything).Return(provider.ValidateGroupSpecResult{}, errors.New("oops"))
		withServer(t, addr, mocks.pclient, mocks.qclient, nil, func(host string) {
			client, err := NewClient(mocks.qclient, addr, nil)
			assert.NoError(t, err)
			_, err = client.Validate(context.Background(), dtypes.GroupSpec{})
			assert.Error(t, err)
			_, err = client.Validate(context.Background(), testutil.GroupSpec(t))
			assert.Error(t, err)
		})
		mocks.pclient.AssertExpectations(t)
	})
}

func Test_router_Manifest(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		paddr := testutil.AccAddress(t)
		caddr := testutil.AccAddress(t)

		did := testutil.DeploymentIDForAccount(t, caddr)
		mocks := createMocks()

		mocks.pmclient.On("Submit", mock.Anything, did, akashmanifest.Manifest(nil)).Return(nil)
		withServer(t, paddr, mocks.pclient, mocks.qclient, nil, func(host string) {
			cert := testutil.Certificate(t, caddr, testutil.CertificateOptionMocks(mocks.qclient))
			client, err := NewClient(mocks.qclient, paddr, cert.Cert)
			assert.NoError(t, err)
			err = client.SubmitManifest(context.Background(), did.DSeq, nil)
			assert.NoError(t, err)
		})
		mocks.pmclient.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		paddr := testutil.AccAddress(t)
		caddr := testutil.AccAddress(t)

		did := testutil.DeploymentIDForAccount(t, caddr)

		mocks := createMocks()

		mocks.pmclient.On("Submit", mock.Anything, did, akashmanifest.Manifest(nil)).Return(errors.New("ded"))
		withServer(t, paddr, mocks.pclient, mocks.qclient, nil, func(host string) {
			cert := testutil.Certificate(t, caddr, testutil.CertificateOptionMocks(mocks.qclient))
			client, err := NewClient(mocks.qclient, paddr, cert.Cert)
			assert.NoError(t, err)
			err = client.SubmitManifest(context.Background(), did.DSeq, nil)
			assert.Error(t, err)
		})
		mocks.pmclient.AssertExpectations(t)
	})
}

func Test_router_LeaseStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		expected := &ctypes.LeaseStatus{}
		paddr := testutil.AccAddress(t)
		caddr := testutil.AccAddress(t)

		id := testutil.LeaseIDForAccount(t, caddr, paddr)
		mocks := createMocks()
		mocks.pcclient.On("LeaseStatus", mock.Anything, id).Return(expected, nil)
		withServer(t, paddr, mocks.pclient, mocks.qclient, nil, func(host string) {
			cert := testutil.Certificate(t, caddr, testutil.CertificateOptionMocks(mocks.qclient))
			client, err := NewClient(mocks.qclient, paddr, cert.Cert)
			assert.NoError(t, err)
			status, err := client.LeaseStatus(context.Background(), id)
			assert.Equal(t, expected, status)
			assert.NoError(t, err)
		})
		mocks.pcclient.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		paddr := testutil.AccAddress(t)
		caddr := testutil.AccAddress(t)

		id := testutil.LeaseIDForAccount(t, caddr, paddr)
		mocks := createMocks()

		mocks.pcclient.On("LeaseStatus", mock.Anything, id).Return(nil, errors.New("ded"))
		withServer(t, paddr, mocks.pclient, mocks.qclient, nil, func(host string) {
			cert := testutil.Certificate(t, caddr, testutil.CertificateOptionMocks(mocks.qclient))
			client, err := NewClient(mocks.qclient, paddr, cert.Cert)
			assert.NoError(t, err)
			status, err := client.LeaseStatus(context.Background(), id)
			assert.Nil(t, status)
			assert.Error(t, err)
		})
		mocks.pcclient.AssertExpectations(t)
	})
}

func Test_router_ServiceStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		paddr := testutil.AccAddress(t)
		caddr := testutil.AccAddress(t)

		id := testutil.LeaseIDForAccount(t, caddr, paddr)

		expected := &ctypes.ServiceStatus{}
		service := "svc"

		mocks := createMocks()

		mocks.pcclient.On("ServiceStatus", mock.Anything, id, service).Return(expected, nil)
		withServer(t, paddr, mocks.pclient, mocks.qclient, nil, func(host string) {
			cert := testutil.Certificate(t, caddr, testutil.CertificateOptionMocks(mocks.qclient))
			client, err := NewClient(mocks.qclient, paddr, cert.Cert)
			assert.NoError(t, err)
			status, err := client.ServiceStatus(context.Background(), id, service)
			assert.NoError(t, err)
			assert.Equal(t, expected, status)
		})
		mocks.pcclient.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		paddr := testutil.AccAddress(t)
		caddr := testutil.AccAddress(t)

		id := testutil.LeaseIDForAccount(t, caddr, paddr)

		service := "svc"

		mocks := createMocks()

		mocks.pcclient.On("ServiceStatus", mock.Anything, id, service).Return(nil, errors.New("ded"))
		withServer(t, paddr, mocks.pclient, mocks.qclient, nil, func(host string) {
			cert := testutil.Certificate(t, caddr, testutil.CertificateOptionMocks(mocks.qclient))
			client, err := NewClient(mocks.qclient, paddr, cert.Cert)
			assert.NoError(t, err)
			status, err := client.ServiceStatus(context.Background(), id, service)
			assert.Nil(t, status)
			assert.Error(t, err)
		})
		mocks.pcclient.AssertExpectations(t)
	})
}

type integrationMocks struct {
	pmclient       *pmmock.Client
	pcclient       *pcmock.Client
	pclient        *pmock.Client
	qclient        *qmock.QueryClient
	hostnameClient *pcmock.HostnameServiceClient
	clusterService *pcmock.Service
}

func createMocks() integrationMocks {
	var (
		pmclient       = &pmmock.Client{}
		pcclient       = &pcmock.Client{}
		pclient        = &pmock.Client{}
		qclient        = &qmock.QueryClient{}
		hostnameClient = &pcmock.HostnameServiceClient{}
		clusterService = &pcmock.Service{}
	)

	pclient.On("Manifest").Return(pmclient)
	pclient.On("Cluster").Return(pcclient)

	// TODO - return stubs here when tests are added
	pclient.On("Hostname").Return(hostnameClient)
	pclient.On("ClusterService").Return(clusterService)

	return integrationMocks{
		pmclient:       pmclient,
		pcclient:       pcclient,
		pclient:        pclient,
		qclient:        qclient,
		hostnameClient: hostnameClient,
		clusterService: clusterService,
	}
}

func withServer(t testing.TB, addr sdk.Address, pclient provider.Client, qclient *qmock.QueryClient, certs []tls.Certificate, fn func(string)) {
	t.Helper()
	router := newRouter(testutil.Logger(t), addr, pclient, map[interface{}]interface{}{})

	if len(certs) == 0 {
		crt := testutil.Certificate(
			t,
			addr,
			testutil.CertificateOptionDomains([]string{"localhost", "127.0.0.1"}),
			testutil.CertificateOptionMocks(qclient),
		)

		certs = append(certs, crt.Cert...)
	}

	server := testutil.NewServer(t, qclient, router, certs)
	defer server.Close()

	host := "https://" + server.Listener.Addr().String()
	qclient.On("Provider", mock.Anything, &providertypes.QueryProviderRequest{Owner: addr.String()}).
		Return(&providertypes.QueryProviderResponse{
			Provider: providertypes.Provider{
				Owner:   addr.String(),
				HostURI: host,
			},
		}, nil)

	fn(host)
}
