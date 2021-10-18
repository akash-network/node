package rest

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
	"time"

	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	qmock "github.com/ovrclk/akash/client/mocks"
	"github.com/ovrclk/akash/provider"
	kubeClient "github.com/ovrclk/akash/provider/cluster/kube"
	pcmock "github.com/ovrclk/akash/provider/cluster/mocks"
	clustertypes "github.com/ovrclk/akash/provider/cluster/types"
	pmmock "github.com/ovrclk/akash/provider/manifest/mocks"
	pmock "github.com/ovrclk/akash/provider/mocks"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	manifestValidation "github.com/ovrclk/akash/validation"
	types "github.com/ovrclk/akash/x/market/types/v1beta2"
)

const (
	testSDL     = "../../../sdl/_testdata/simple.yaml"
	serviceName = "database"
)

var errGeneric = errors.New("generic test error")

type fakeKubernetesStatusError struct {
	status metav1.Status
}

func (fkse fakeKubernetesStatusError) Status() metav1.Status {
	return fkse.status
}

func (fkse fakeKubernetesStatusError) Error() string {
	return "fake error"
}

type routerTest struct {
	caddr          sdk.Address
	paddr          sdk.Address
	pmclient       *pmmock.Client
	pcclient       *pcmock.Client
	pclient        *pmock.Client
	qclient        *qmock.QueryClient
	clusterService *pcmock.Service
	hostnameClient *pcmock.HostnameServiceClient
	gclient        *client
	ccert          testutil.TestCertificate
	pcert          testutil.TestCertificate
	host           *url.URL
}

func runRouterTest(t *testing.T, authClient bool, fn func(*routerTest)) {
	t.Helper()

	mocks := createMocks()

	mf := &routerTest{
		caddr:          testutil.AccAddress(t),
		paddr:          testutil.AccAddress(t),
		pmclient:       mocks.pmclient,
		pcclient:       mocks.pcclient,
		pclient:        mocks.pclient,
		qclient:        mocks.qclient,
		hostnameClient: mocks.hostnameClient,
		clusterService: mocks.clusterService,
	}

	mf.ccert = testutil.Certificate(t, mf.caddr, testutil.CertificateOptionMocks(mocks.qclient))
	mf.pcert = testutil.Certificate(
		t,
		mf.paddr,
		testutil.CertificateOptionDomains([]string{"localhost", "127.0.0.1"}),
		testutil.CertificateOptionMocks(mocks.qclient))

	var certs []tls.Certificate
	if authClient {
		certs = mf.ccert.Cert
	}

	withServer(t, mf.paddr, mocks.pclient, mocks.qclient, mf.pcert.Cert, func(host string) {
		var err error
		mf.host, err = url.Parse(host)
		require.NoError(t, err)

		gclient, err := NewClient(mocks.qclient, mf.paddr, certs)
		require.NoError(t, err)
		require.NotNil(t, gclient)

		mf.gclient = gclient.(*client)

		fn(mf)
	})
}

func testCertHelper(t *testing.T, test *routerTest) {
	test.pmclient.On(
		"Submit",
		mock.Anything,
		mock.AnythingOfType("types.DeploymentID"),
		mock.AnythingOfType("manifest.Manifest"),
	).Return(nil)

	dseq := uint64(testutil.RandRangeInt(1, 1000))

	uri, err := makeURI(test.host, submitManifestPath(dseq))
	require.NoError(t, err)

	sdl, err := sdl.ReadFile(testSDL)
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	buf, err := json.Marshal(mani)
	require.NoError(t, err)

	req, err := http.NewRequest("PUT", uri, bytes.NewBuffer(buf))
	require.NoError(t, err)

	req.Header.Set("Content-Type", contentTypeJSON)

	_, err = test.gclient.hclient.Do(req)
	require.Error(t, err)
	// return error message looks like
	// Put "https://127.0.0.1:58536/deployment/652/manifest": tls: unable to verify certificate: x509: cannot validate certificate for 127.0.0.1 because it doesn't contain any IP SANs
	require.Regexp(t, "^(Put|Get) (\".*\": )tls: unable to verify certificate: .*$", err.Error())
}

func TestRouteNotActiveClientCert(t *testing.T) {
	mocks := createMocks()

	mf := &routerTest{
		caddr:    testutil.AccAddress(t),
		paddr:    testutil.AccAddress(t),
		pmclient: mocks.pmclient,
		pcclient: mocks.pcclient,
		pclient:  mocks.pclient,
		qclient:  mocks.qclient,
	}

	mf.ccert = testutil.Certificate(
		t,
		mf.caddr,
		testutil.CertificateOptionMocks(mocks.qclient),
		testutil.CertificateOptionNotBefore(time.Now().Add(time.Hour*24)),
	)
	mf.pcert = testutil.Certificate(t, mf.paddr, testutil.CertificateOptionMocks(mocks.qclient))

	withServer(t, mf.paddr, mocks.pclient, mocks.qclient, mf.pcert.Cert, func(host string) {
		var err error
		mf.host, err = url.Parse(host)
		require.NoError(t, err)

		gclient, err := NewClient(mocks.qclient, mf.paddr, mf.ccert.Cert)
		require.NoError(t, err)
		require.NotNil(t, gclient)

		mf.gclient = gclient.(*client)

		testCertHelper(t, mf)
	})
}

func TestRouteExpiredClientCert(t *testing.T) {
	mocks := createMocks()

	mf := &routerTest{
		caddr:    testutil.AccAddress(t),
		paddr:    testutil.AccAddress(t),
		pmclient: mocks.pmclient,
		pcclient: mocks.pcclient,
		pclient:  mocks.pclient,
		qclient:  mocks.qclient,
	}

	mf.ccert = testutil.Certificate(
		t,
		mf.caddr,
		testutil.CertificateOptionMocks(mocks.qclient),
		testutil.CertificateOptionNotBefore(time.Now().Add(time.Hour*(-48))),
		testutil.CertificateOptionNotAfter(time.Now().Add(time.Hour*(-24))),
	)
	mf.pcert = testutil.Certificate(t, mf.paddr, testutil.CertificateOptionMocks(mocks.qclient))

	withServer(t, mf.paddr, mocks.pclient, mocks.qclient, mf.pcert.Cert, func(host string) {
		var err error
		mf.host, err = url.Parse(host)
		require.NoError(t, err)

		gclient, err := NewClient(mocks.qclient, mf.paddr, mf.ccert.Cert)
		require.NoError(t, err)
		require.NotNil(t, gclient)

		mf.gclient = gclient.(*client)

		testCertHelper(t, mf)
	})
}

func TestRouteNotActiveServerCert(t *testing.T) {
	mocks := createMocks()

	mf := &routerTest{
		caddr:    testutil.AccAddress(t),
		paddr:    testutil.AccAddress(t),
		pmclient: mocks.pmclient,
		pcclient: mocks.pcclient,
		pclient:  mocks.pclient,
		qclient:  mocks.qclient,
	}

	mf.ccert = testutil.Certificate(
		t,
		mf.caddr,
		testutil.CertificateOptionMocks(mocks.qclient),
	)
	mf.pcert = testutil.Certificate(
		t,
		mf.paddr,
		testutil.CertificateOptionMocks(mocks.qclient),
		testutil.CertificateOptionNotBefore(time.Now().Add(time.Hour*24)),
	)

	withServer(t, mf.paddr, mocks.pclient, mocks.qclient, mf.pcert.Cert, func(host string) {
		var err error
		mf.host, err = url.Parse(host)
		require.NoError(t, err)

		gclient, err := NewClient(mocks.qclient, mf.paddr, mf.ccert.Cert)
		require.NoError(t, err)
		require.NotNil(t, gclient)

		mf.gclient = gclient.(*client)

		testCertHelper(t, mf)
	})
}

func TestRouteExpiredServerCert(t *testing.T) {
	mocks := createMocks()

	mf := &routerTest{
		caddr:    testutil.AccAddress(t),
		paddr:    testutil.AccAddress(t),
		pmclient: mocks.pmclient,
		pcclient: mocks.pcclient,
		pclient:  mocks.pclient,
		qclient:  mocks.qclient,
	}

	mf.ccert = testutil.Certificate(
		t,
		mf.caddr,
		testutil.CertificateOptionMocks(mocks.qclient),
	)
	mf.pcert = testutil.Certificate(
		t,
		mf.paddr,
		testutil.CertificateOptionMocks(mocks.qclient),
		testutil.CertificateOptionNotBefore(time.Now().Add(time.Hour*(-48))),
		testutil.CertificateOptionNotAfter(time.Now().Add(time.Hour*(-24))),
	)

	withServer(t, mf.paddr, mocks.pclient, mocks.qclient, mf.pcert.Cert, func(host string) {
		var err error
		mf.host, err = url.Parse(host)
		require.NoError(t, err)

		gclient, err := NewClient(mocks.qclient, mf.paddr, mf.ccert.Cert)
		require.NoError(t, err)
		require.NotNil(t, gclient)

		mf.gclient = gclient.(*client)

		testCertHelper(t, mf)
	})
}

func TestRouteDoesNotExist(t *testing.T) {
	runRouterTest(t, false, func(test *routerTest) {
		uri, err := makeURI(test.host, "foobar")
		require.NoError(t, err)

		req, err := http.NewRequest("GET", uri, nil)
		require.NoError(t, err)

		req.Header.Set("Content-Type", contentTypeJSON)

		resp, err := test.gclient.hclient.Do(req)
		require.NoError(t, err)
		require.Equal(t, resp.StatusCode, http.StatusNotFound)
	})
}

func TestRouteStatusOK(t *testing.T) {
	runRouterTest(t, false, func(test *routerTest) {
		status := &provider.Status{
			Cluster:               nil,
			Bidengine:             nil,
			Manifest:              nil,
			ClusterPublicHostname: "foobar",
		}

		test.pclient.On("Status", mock.Anything).Return(status, nil)

		uri, err := makeURI(test.host, statusPath())
		require.NoError(t, err)

		req, err := http.NewRequest("GET", uri, nil)
		require.NoError(t, err)

		req.Header.Set("Content-Type", contentTypeJSON)

		resp, err := test.gclient.hclient.Do(req)
		require.NoError(t, err)
		require.Equal(t, resp.StatusCode, http.StatusOK)
		data := make(map[string]interface{})
		decoder := json.NewDecoder(resp.Body)
		err = decoder.Decode(&data)
		require.NoError(t, err)
		cph, ok := data["cluster_public_hostname"].(string)
		require.True(t, ok)
		require.Equal(t, cph, "foobar")
	})
}

func TestRouteStatusFails(t *testing.T) {
	runRouterTest(t, false, func(test *routerTest) {
		test.pclient.On("Status", mock.Anything).Return(nil, errGeneric)

		uri, err := makeURI(test.host, statusPath())
		require.NoError(t, err)

		req, err := http.NewRequest("GET", uri, nil)
		require.NoError(t, err)

		req.Header.Set("Content-Type", contentTypeJSON)
		resp, err := test.gclient.hclient.Do(req)
		require.NoError(t, err)
		require.Equal(t, resp.StatusCode, http.StatusInternalServerError)

		data, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Regexp(t, "^generic test error(?s:.)*$", string(data))
	})
}

func TestRouteValidateOK(t *testing.T) {
	runRouterTest(t, false, func(test *routerTest) {
		validate := provider.ValidateGroupSpecResult{
			MinBidPrice: testutil.AkashDecCoin(t, 200),
		}

		test.pclient.On("Validate", mock.Anything, mock.Anything).Return(validate, nil)

		uri, err := makeURI(test.host, validatePath())
		require.NoError(t, err)

		gspec := testutil.GroupSpec(t)
		bgspec, err := json.Marshal(&gspec)
		require.NoError(t, err)

		req, err := http.NewRequest("GET", uri, bytes.NewReader(bgspec))
		require.NoError(t, err)

		req.Header.Set("Content-Type", contentTypeJSON)

		resp, err := test.gclient.hclient.Do(req)
		require.NoError(t, err)
		require.Equal(t, resp.StatusCode, http.StatusOK)
		data := make(map[string]interface{})
		decoder := json.NewDecoder(resp.Body)
		err = decoder.Decode(&data)
		require.NoError(t, err)
	})
}

func TestRouteValidateFails(t *testing.T) {
	runRouterTest(t, false, func(test *routerTest) {
		test.pclient.On("Validate", mock.Anything, mock.Anything).Return(provider.ValidateGroupSpecResult{}, errGeneric)

		uri, err := makeURI(test.host, validatePath())
		require.NoError(t, err)

		gspec := testutil.GroupSpec(t)
		bgspec, err := json.Marshal(&gspec)
		require.NoError(t, err)

		req, err := http.NewRequest("GET", uri, bytes.NewReader(bgspec))
		require.NoError(t, err)

		req.Header.Set("Content-Type", contentTypeJSON)
		resp, err := test.gclient.hclient.Do(req)
		require.NoError(t, err)
		require.Equal(t, resp.StatusCode, http.StatusInternalServerError)

		data, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Regexp(t, "^generic test error(?s:.)*$", string(data))
	})
}

func TestRouteValidateFailsEmptyBody(t *testing.T) {
	runRouterTest(t, false, func(test *routerTest) {
		test.pclient.On("Validate", mock.Anything, mock.Anything).Return(provider.ValidateGroupSpecResult{}, errGeneric)

		uri, err := makeURI(test.host, validatePath())
		require.NoError(t, err)

		req, err := http.NewRequest("GET", uri, nil)
		require.NoError(t, err)

		req.Header.Set("Content-Type", contentTypeJSON)
		resp, err := test.gclient.hclient.Do(req)
		require.NoError(t, err)
		require.Equal(t, resp.StatusCode, http.StatusBadRequest)

		data, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Regexp(t, "empty payload", string(data))
	})
}

func TestRoutePutManifestOK(t *testing.T) {
	runRouterTest(t, true, func(test *routerTest) {
		dseq := uint64(testutil.RandRangeInt(1, 1000))
		test.pmclient.On(
			"Submit",
			mock.Anything,
			dtypes.DeploymentID{
				Owner: test.caddr.String(),
				DSeq:  dseq,
			},
			mock.AnythingOfType("manifest.Manifest"),
		).Return(nil)

		uri, err := makeURI(test.host, submitManifestPath(dseq))
		require.NoError(t, err)

		sdl, err := sdl.ReadFile(testSDL)
		require.NoError(t, err)

		mani, err := sdl.Manifest()
		require.NoError(t, err)

		buf, err := json.Marshal(mani)
		require.NoError(t, err)

		req, err := http.NewRequest("PUT", uri, bytes.NewBuffer(buf))
		require.NoError(t, err)

		req.Header.Set("Content-Type", contentTypeJSON)

		resp, err := test.gclient.hclient.Do(req)
		require.NoError(t, err)
		require.Equal(t, resp.StatusCode, http.StatusOK)

		data, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, string(data), "")
	})
}

func TestRoutePutInvalidManifest(t *testing.T) {
	_ = dtypes.DeploymentID{}
	runRouterTest(t, true, func(test *routerTest) {
		dseq := uint64(testutil.RandRangeInt(1, 1000))
		test.pmclient.On("Submit",
			mock.Anything,
			dtypes.DeploymentID{
				Owner: test.caddr.String(),
				DSeq:  dseq,
			},

			mock.AnythingOfType("manifest.Manifest"),
		).Return(manifestValidation.ErrInvalidManifest)

		uri, err := makeURI(test.host, submitManifestPath(dseq))
		require.NoError(t, err)

		sdl, err := sdl.ReadFile(testSDL)
		require.NoError(t, err)

		mani, err := sdl.Manifest()
		require.NoError(t, err)

		buf, err := json.Marshal(mani)
		require.NoError(t, err)

		req, err := http.NewRequest("PUT", uri, bytes.NewBuffer(buf))
		require.NoError(t, err)

		req.Header.Set("Content-Type", contentTypeJSON)

		resp, err := test.gclient.hclient.Do(req)
		require.NoError(t, err)
		require.Equal(t, resp.StatusCode, http.StatusUnprocessableEntity)

		data, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Regexp(t, "^invalid manifest(?s:.)*$", string(data))
	})
}

func TestRouteLeaseStatusOk(t *testing.T) {
	runRouterTest(t, true, func(test *routerTest) {
		dseq := uint64(testutil.RandRangeInt(1, 1000))
		oseq := uint32(testutil.RandRangeInt(2000, 3000))
		gseq := uint32(testutil.RandRangeInt(4000, 5000))

		status := &clustertypes.LeaseStatus{
			Services:       nil,
			ForwardedPorts: nil,
		}

		test.pcclient.On("LeaseStatus", mock.Anything, types.LeaseID{
			Owner:    test.caddr.String(),
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: test.paddr.String(),
		}).Return(status, nil)

		lid := types.LeaseID{
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: test.paddr.String(),
		}

		uri, err := makeURI(test.host, leaseStatusPath(lid))
		require.NoError(t, err)

		sdl, err := sdl.ReadFile(testSDL)
		require.NoError(t, err)

		mani, err := sdl.Manifest()
		require.NoError(t, err)

		buf, err := json.Marshal(mani)
		require.NoError(t, err)

		req, err := http.NewRequest("GET", uri, bytes.NewBuffer(buf))
		require.NoError(t, err)

		req.Header.Set("Content-Type", contentTypeJSON)

		resp, err := test.gclient.hclient.Do(req)
		require.NoError(t, err)
		require.Equal(t, resp.StatusCode, http.StatusOK)

		data := make(map[string]interface{})
		dec := json.NewDecoder(resp.Body)
		err = dec.Decode(&data)
		require.NoError(t, err)
	})
}

func TestRouteLeaseNotInKubernetes(t *testing.T) {
	runRouterTest(t, true, func(test *routerTest) {
		dseq := uint64(testutil.RandRangeInt(1, 1000))
		oseq := uint32(testutil.RandRangeInt(2000, 3000))
		gseq := uint32(testutil.RandRangeInt(4000, 5000))

		kubeStatus := fakeKubernetesStatusError{
			status: metav1.Status{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Status:   "",
				Message:  "",
				Reason:   metav1.StatusReasonNotFound,
				Details:  nil,
				Code:     0,
			},
		}

		test.pcclient.On("LeaseStatus", mock.Anything, types.LeaseID{
			Owner:    test.caddr.String(),
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: test.paddr.String(),
		}).Return(nil, kubeStatus)

		lid := types.LeaseID{
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: test.paddr.String(),
		}

		uri, err := makeURI(test.host, leaseStatusPath(lid))
		require.NoError(t, err)

		sdl, err := sdl.ReadFile(testSDL)
		require.NoError(t, err)

		mani, err := sdl.Manifest()
		require.NoError(t, err)

		buf, err := json.Marshal(mani)
		require.NoError(t, err)

		req, err := http.NewRequest("GET", uri, bytes.NewBuffer(buf))
		require.NoError(t, err)

		req.Header.Set("Content-Type", contentTypeJSON)

		resp, err := test.gclient.hclient.Do(req)
		require.NoError(t, err)
		require.Equal(t, resp.StatusCode, http.StatusNotFound)
	})
}

func TestRouteLeaseStatusErr(t *testing.T) {
	runRouterTest(t, true, func(test *routerTest) {
		dseq := uint64(testutil.RandRangeInt(1, 1000))
		oseq := uint32(testutil.RandRangeInt(2000, 3000))
		gseq := uint32(testutil.RandRangeInt(4000, 5000))

		test.pcclient.On("LeaseStatus", mock.Anything, types.LeaseID{
			Owner:    test.caddr.String(),
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: test.paddr.String(),
		}).Return(nil, errGeneric)

		lid := types.LeaseID{
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: test.paddr.String(),
		}

		uri, err := makeURI(test.host, leaseStatusPath(lid))
		require.NoError(t, err)

		req, err := http.NewRequest("GET", uri, nil)
		require.NoError(t, err)

		req.Header.Set("Content-Type", contentTypeJSON)

		resp, err := test.gclient.hclient.Do(req)
		require.NoError(t, err)

		require.Equal(t, resp.StatusCode, http.StatusInternalServerError)
		data, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Regexp(t, "^generic test error(?s:.)*$", string(data))
	})
}

func TestRouteServiceStatusOK(t *testing.T) {
	runRouterTest(t, true, func(test *routerTest) {
		dseq := uint64(testutil.RandRangeInt(1, 1000))
		oseq := uint32(testutil.RandRangeInt(2000, 3000))
		gseq := uint32(testutil.RandRangeInt(4000, 5000))

		status := &clustertypes.ServiceStatus{
			Name:               "",
			Available:          0,
			Total:              0,
			URIs:               nil,
			ObservedGeneration: 0,
			Replicas:           0,
			UpdatedReplicas:    0,
			ReadyReplicas:      0,
			AvailableReplicas:  0,
		}
		test.pcclient.On("ServiceStatus", mock.Anything, types.LeaseID{
			Owner:    test.caddr.String(),
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: test.paddr.String(),
		}, serviceName).Return(status, nil)

		lid := types.LeaseID{
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: test.paddr.String(),
		}

		uri, err := makeURI(test.host, serviceStatusPath(lid, serviceName))
		require.NoError(t, err)

		req, err := http.NewRequest("GET", uri, nil)
		require.NoError(t, err)

		req.Header.Set("Content-Type", contentTypeJSON)

		resp, err := test.gclient.hclient.Do(req)
		require.NoError(t, err)

		require.Equal(t, resp.StatusCode, http.StatusOK)
		data := make(map[string]interface{})
		dec := json.NewDecoder(resp.Body)
		err = dec.Decode(&data)
		require.NoError(t, err)
	})
}

func TestRouteServiceStatusNoDeployment(t *testing.T) {
	runRouterTest(t, true, func(test *routerTest) {
		dseq := uint64(testutil.RandRangeInt(1, 1000))
		oseq := uint32(testutil.RandRangeInt(2000, 3000))
		gseq := uint32(testutil.RandRangeInt(4000, 5000))

		test.pcclient.On("ServiceStatus", mock.Anything, types.LeaseID{
			Owner:    test.caddr.String(),
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: test.paddr.String(),
		}, serviceName).Return(nil, kubeClient.ErrNoDeploymentForLease)

		lid := types.LeaseID{
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: test.paddr.String(),
		}

		uri, err := makeURI(test.host, serviceStatusPath(lid, serviceName))
		require.NoError(t, err)

		req, err := http.NewRequest("GET", uri, nil)
		require.NoError(t, err)

		req.Header.Set("Content-Type", contentTypeJSON)

		resp, err := test.gclient.hclient.Do(req)
		require.NoError(t, err)

		require.Equal(t, resp.StatusCode, http.StatusNotFound)
		data, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Regexp(t, "^kube: no deployment(?s:.)*$", string(data))
	})
}

func TestRouteServiceStatusKubernetesNotFound(t *testing.T) {
	runRouterTest(t, true, func(test *routerTest) {
		dseq := uint64(testutil.RandRangeInt(1, 1000))
		oseq := uint32(testutil.RandRangeInt(2000, 3000))
		gseq := uint32(testutil.RandRangeInt(4000, 5000))

		kubeStatus := fakeKubernetesStatusError{
			status: metav1.Status{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Status:   "",
				Message:  "",
				Reason:   metav1.StatusReasonNotFound,
				Details:  nil,
				Code:     0,
			},
		}

		test.pcclient.On("ServiceStatus", mock.Anything, types.LeaseID{
			Owner:    test.caddr.String(),
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: test.paddr.String(),
		}, serviceName).Return(nil, kubeStatus)

		lid := types.LeaseID{
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: test.paddr.String(),
		}

		uri, err := makeURI(test.host, serviceStatusPath(lid, serviceName))
		require.NoError(t, err)

		req, err := http.NewRequest("GET", uri, nil)
		require.NoError(t, err)

		req.Header.Set("Content-Type", contentTypeJSON)

		resp, err := test.gclient.hclient.Do(req)
		require.NoError(t, err)

		require.Equal(t, resp.StatusCode, http.StatusNotFound)
		data, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Regexp(t, "^fake error(?s:.)*$", string(data))
	})
}

func TestRouteServiceStatusError(t *testing.T) {
	runRouterTest(t, true, func(test *routerTest) {
		dseq := uint64(testutil.RandRangeInt(1, 1000))
		oseq := uint32(testutil.RandRangeInt(2000, 3000))
		gseq := uint32(testutil.RandRangeInt(4000, 5000))

		test.pcclient.On("ServiceStatus", mock.Anything, types.LeaseID{
			Owner:    test.caddr.String(),
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: test.paddr.String(),
		}, serviceName).Return(nil, errGeneric)

		lid := types.LeaseID{
			DSeq:     dseq,
			GSeq:     gseq,
			OSeq:     oseq,
			Provider: test.paddr.String(),
		}

		uri, err := makeURI(test.host, serviceStatusPath(lid, serviceName))
		require.NoError(t, err)

		req, err := http.NewRequest("GET", uri, nil)
		require.NoError(t, err)

		req.Header.Set("Content-Type", contentTypeJSON)

		resp, err := test.gclient.hclient.Do(req)
		require.NoError(t, err)

		require.Equal(t, resp.StatusCode, http.StatusInternalServerError)
		data, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Regexp(t, "^generic test error(?s:.)*$", string(data))
	})
}
