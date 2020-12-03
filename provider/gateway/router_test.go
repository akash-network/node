package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/ovrclk/akash/provider"
	kubeClient "github.com/ovrclk/akash/provider/cluster/kube"
	clusterMocks "github.com/ovrclk/akash/provider/cluster/mocks"
	clustertypes "github.com/ovrclk/akash/provider/cluster/types"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	"github.com/ovrclk/akash/provider/manifest"
	manifestMocks "github.com/ovrclk/akash/provider/manifest/mocks"
	"github.com/ovrclk/akash/provider/mocks"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/testutil"
	manifestValidation "github.com/ovrclk/akash/validation"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"io"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"net/http/httptest"
	"testing"
)

type testScaffold struct {
	w                  *httptest.ResponseRecorder
	mockClient         *mocks.Client
	manifestClientMock *manifestMocks.Client
	clusterClientMock  *clusterMocks.Client
	router             *mux.Router
}

func newRouterForTest(t *testing.T) *testScaffold {
	logger := testutil.Logger(t)
	mockClient := &mocks.Client{}
	manifestMocks := &manifestMocks.Client{}
	clusterMocks := &clusterMocks.Client{}

	mockClient.On("Manifest").Return(manifestMocks)
	mockClient.On("Cluster").Return(clusterMocks)

	r := newRouter(logger, mockClient)

	scaffold := &testScaffold{
		w:                  httptest.NewRecorder(),
		mockClient:         mockClient,
		manifestClientMock: manifestMocks,
		clusterClientMock:  clusterMocks,
		router:             r,
	}

	return scaffold
}

func (s *testScaffold) serveHTTP(method string, target string, body io.Reader) {
	s.router.ServeHTTP(s.w, httptest.NewRequest(method, target, body))
}

func TestRouteDoesNotExist(t *testing.T) {
	scaffold := newRouterForTest(t)
	scaffold.serveHTTP("GET", "/foobar", nil)
	require.Equal(t, scaffold.w.Code, http.StatusNotFound)
}

func TestRouteStatusOK(t *testing.T) {
	scaffold := newRouterForTest(t)
	status := &provider.Status{
		Cluster:               nil,
		Bidengine:             nil,
		Manifest:              nil,
		ClusterPublicHostname: "foobar",
	}
	scaffold.mockClient.On("Status", mock.Anything).Return(status, nil)

	scaffold.serveHTTP("GET", "/status", nil)
	require.Equal(t, scaffold.w.Code, http.StatusOK)
	data := make(map[string]interface{})
	decoder := json.NewDecoder(scaffold.w.Body)
	err := decoder.Decode(&data)
	require.NoError(t, err)
	cph, ok := data["cluster-public-hostname"].(string)
	require.True(t, ok)
	require.Equal(t, cph, "foobar")
}

var errGeneric = errors.New("generic test error")

func TestRouteStatusFails(t *testing.T) {
	scaffold := newRouterForTest(t)

	scaffold.mockClient.On("Status", mock.Anything).Return(nil, errGeneric)

	scaffold.serveHTTP("GET", "/status", nil)
	require.Equal(t, scaffold.w.Code, http.StatusInternalServerError)
	require.Regexp(t, "^generic test error(?s:.)*$", scaffold.w.Body.String())
}

func TestRoutePutManifestOK(t *testing.T) {
	scaffold := newRouterForTest(t)
	scaffold.manifestClientMock.On("Submit", mock.Anything, mock.AnythingOfType("*manifest.SubmitRequest")).Return(nil)

	dseq := uint64(testutil.RandRangeInt(1, 1000))
	owner := testutil.AccAddress(t)
	path := fmt.Sprintf("/deployment/%v/%v/manifest", owner.String(), dseq)

	sdl, err := sdl.ReadFile("../../sdl/_testdata/simple.yaml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	submitRequest := manifest.SubmitRequest{
		Deployment: dtypes.DeploymentID{
			Owner: owner.String(),
			DSeq:  dseq,
		},
		Manifest: mani,
	}

	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	err = enc.Encode(submitRequest)
	require.NoError(t, err)

	scaffold.serveHTTP("PUT", path, body)

	require.Equal(t, scaffold.w.Code, http.StatusOK)
	require.Equal(t, scaffold.w.Body.String(), "")
}

func TestRoutePutManifestDSeqMismatch(t *testing.T) {
	scaffold := newRouterForTest(t)

	dseq := uint64(testutil.RandRangeInt(1, 1000))
	owner := testutil.AccAddress(t)
	path := fmt.Sprintf("/deployment/%v/%v/manifest", owner.String(), dseq)

	sdl, err := sdl.ReadFile("../../sdl/_testdata/simple.yaml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	submitRequest := manifest.SubmitRequest{
		Deployment: dtypes.DeploymentID{
			Owner: owner.String(),
			DSeq:  dseq + 1,
		},
		Manifest: mani,
	}

	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	err = enc.Encode(submitRequest)
	require.NoError(t, err)

	scaffold.serveHTTP("PUT", path, body)

	require.Equal(t, scaffold.w.Code, http.StatusBadRequest)
	require.Regexp(t, "^deployment ID in request body does not match this resource(?s:.)*$", scaffold.w.Body.String())
}

func TestRoutePutManifestOwnerMismatch(t *testing.T) {
	scaffold := newRouterForTest(t)

	dseq := uint64(testutil.RandRangeInt(1, 1000))
	owner := testutil.AccAddress(t)
	path := fmt.Sprintf("/deployment/%v/%v/manifest", owner.String(), dseq)

	sdl, err := sdl.ReadFile("../../sdl/_testdata/simple.yaml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	submitRequest := manifest.SubmitRequest{
		Deployment: dtypes.DeploymentID{
			Owner: owner.String() + "a",
			DSeq:  dseq,
		},
		Manifest: mani,
	}

	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	err = enc.Encode(submitRequest)
	require.NoError(t, err)

	scaffold.serveHTTP("PUT", path, body)

	require.Equal(t, scaffold.w.Code, http.StatusBadRequest)
	require.Regexp(t, "^deployment ID in request body does not match this resource(?s:.)*$", scaffold.w.Body.String())
}

func TestRoutePutInvalidManifest(t *testing.T) {
	scaffold := newRouterForTest(t)

	scaffold.manifestClientMock.On("Submit", mock.Anything, mock.AnythingOfType("*manifest.SubmitRequest")).Return(manifestValidation.ErrInvalidManifest)

	dseq := uint64(33)
	owner := testutil.AccAddress(t)
	path := fmt.Sprintf("/deployment/%v/%v/manifest", owner.String(), dseq)

	sdl, err := sdl.ReadFile("../../sdl/_testdata/simple.yaml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	submitRequest := manifest.SubmitRequest{
		Deployment: dtypes.DeploymentID{
			Owner: owner.String(),
			DSeq:  dseq,
		},
		Manifest: mani,
	}

	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	err = enc.Encode(submitRequest)
	require.NoError(t, err)

	scaffold.serveHTTP("PUT", path, body)

	require.Equal(t, scaffold.w.Code, http.StatusUnprocessableEntity)
	require.Regexp(t, "^invalid manifest(?s:.)*$", scaffold.w.Body.String())
}

func TestRouteLeaseStatusOk(t *testing.T) {
	scaffold := newRouterForTest(t)

	owner := testutil.AccAddress(t)
	provider := testutil.AccAddress(t)
	dseq := uint64(testutil.RandRangeInt(1, 1000))
	oseq := uint32(testutil.RandRangeInt(2000, 3000))
	gseq := uint32(testutil.RandRangeInt(4000, 5000))

	status := &clustertypes.LeaseStatus{
		Services:       nil,
		ForwardedPorts: nil,
	}

	scaffold.clusterClientMock.On("LeaseStatus", mock.Anything, types.LeaseID{
		Owner:    owner.String(),
		DSeq:     dseq,
		GSeq:     gseq,
		OSeq:     oseq,
		Provider: provider.String(),
	}).Return(status, nil)

	path := fmt.Sprintf("/lease/%v/%v/%v/%v/%v/status", owner.String(), dseq, gseq, oseq, provider.String())

	scaffold.serveHTTP("GET", path, nil)
	require.Equal(t, scaffold.w.Code, http.StatusOK)
	data := make(map[string]interface{})
	dec := json.NewDecoder(scaffold.w.Body)
	err := dec.Decode(&data)
	require.NoError(t, err)
}

func TestRouteLeaseStatusNoGlobalServices(t *testing.T) {
	scaffold := newRouterForTest(t)

	owner := testutil.AccAddress(t)
	provider := testutil.AccAddress(t)
	dseq := uint64(testutil.RandRangeInt(1, 1000))
	oseq := uint32(testutil.RandRangeInt(2000, 3000))
	gseq := uint32(testutil.RandRangeInt(4000, 5000))

	scaffold.clusterClientMock.On("LeaseStatus", mock.Anything, types.LeaseID{
		Owner:    owner.String(),
		DSeq:     dseq,
		GSeq:     gseq,
		OSeq:     oseq,
		Provider: provider.String(),
	}).Return(nil, kubeClient.ErrNoGlobalServicesForLease)

	path := fmt.Sprintf("/lease/%v/%v/%v/%v/%v/status", owner.String(), dseq, gseq, oseq, provider.String())

	scaffold.serveHTTP("GET", path, nil)
	require.Equal(t, scaffold.w.Code, http.StatusServiceUnavailable)
	require.Regexp(t, "^kube: no global services(?s:.)*$", scaffold.w.Body.String())
}

type fakeKubernetesStatusError struct {
	status metav1.Status
}

func (fkse fakeKubernetesStatusError) Status() metav1.Status {
	return fkse.status
}

func (fkse fakeKubernetesStatusError) Error() string {
	return "fake error"
}

func TestRouteLeaseNotInKubernetes(t *testing.T) {
	scaffold := newRouterForTest(t)

	owner := testutil.AccAddress(t)
	provider := testutil.AccAddress(t)
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

	scaffold.clusterClientMock.On("LeaseStatus", mock.Anything, types.LeaseID{
		Owner:    owner.String(),
		DSeq:     dseq,
		GSeq:     gseq,
		OSeq:     oseq,
		Provider: provider.String(),
	}).Return(nil, kubeStatus)

	path := fmt.Sprintf("/lease/%v/%v/%v/%v/%v/status", owner.String(), dseq, gseq, oseq, provider.String())

	scaffold.serveHTTP("GET", path, nil)
	require.Equal(t, scaffold.w.Code, http.StatusNotFound)
}

func TestRouteLeaseStatusErr(t *testing.T) {
	scaffold := newRouterForTest(t)

	owner := testutil.AccAddress(t)
	provider := testutil.AccAddress(t)
	dseq := uint64(testutil.RandRangeInt(1, 1000))
	oseq := uint32(testutil.RandRangeInt(2000, 3000))
	gseq := uint32(testutil.RandRangeInt(4000, 5000))

	scaffold.clusterClientMock.On("LeaseStatus", mock.Anything, types.LeaseID{
		Owner:    owner.String(),
		DSeq:     dseq,
		GSeq:     gseq,
		OSeq:     oseq,
		Provider: provider.String(),
	}).Return(nil, errGeneric)

	path := fmt.Sprintf("/lease/%v/%v/%v/%v/%v/status", owner.String(), dseq, gseq, oseq, provider.String())

	scaffold.serveHTTP("GET", path, nil)
	require.Equal(t, scaffold.w.Code, http.StatusInternalServerError)
	require.Regexp(t, "^generic test error(?s:.)*$", scaffold.w.Body.String())
}

const serviceName = "database"

func TestRouteServiceStatusOK(t *testing.T) {
	scaffold := newRouterForTest(t)

	owner := testutil.AccAddress(t)
	provider := testutil.AccAddress(t)
	dseq := uint64(testutil.RandRangeInt(1, 1000))
	oseq := uint32(testutil.RandRangeInt(2000, 3000))
	gseq := uint32(testutil.RandRangeInt(4000, 5000))

	status := &ctypes.ServiceStatus{
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
	scaffold.clusterClientMock.On("ServiceStatus", mock.Anything, types.LeaseID{
		Owner:    owner.String(),
		DSeq:     dseq,
		GSeq:     gseq,
		OSeq:     oseq,
		Provider: provider.String(),
	}, serviceName).Return(status, nil)

	path := fmt.Sprintf("/lease/%v/%v/%v/%v/%v/service/%v/status", owner.String(), dseq, gseq, oseq, provider.String(), serviceName)

	scaffold.serveHTTP("GET", path, nil)
	require.Equal(t, scaffold.w.Code, http.StatusOK)
	data := make(map[string]interface{})
	dec := json.NewDecoder(scaffold.w.Body)
	err := dec.Decode(&data)
	require.NoError(t, err)
}

func TestRouteServiceStatusNoDeployment(t *testing.T) {
	scaffold := newRouterForTest(t)

	owner := testutil.AccAddress(t)
	provider := testutil.AccAddress(t)
	dseq := uint64(testutil.RandRangeInt(1, 1000))
	oseq := uint32(testutil.RandRangeInt(2000, 3000))
	gseq := uint32(testutil.RandRangeInt(4000, 5000))

	scaffold.clusterClientMock.On("ServiceStatus", mock.Anything, types.LeaseID{
		Owner:    owner.String(),
		DSeq:     dseq,
		GSeq:     gseq,
		OSeq:     oseq,
		Provider: provider.String(),
	}, serviceName).Return(nil, kubeClient.ErrNoDeploymentForLease)

	path := fmt.Sprintf("/lease/%v/%v/%v/%v/%v/service/%v/status", owner.String(), dseq, gseq, oseq, provider.String(), serviceName)

	scaffold.serveHTTP("GET", path, nil)
	require.Equal(t, scaffold.w.Code, http.StatusNotFound)
	require.Regexp(t, "^kube: no deployment(?s:.)*$", scaffold.w.Body.String())
}

func TestRouteServiceStatusKubernetesNotFound(t *testing.T) {
	scaffold := newRouterForTest(t)

	owner := testutil.AccAddress(t)
	provider := testutil.AccAddress(t)
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

	scaffold.clusterClientMock.On("ServiceStatus", mock.Anything, types.LeaseID{
		Owner:    owner.String(),
		DSeq:     dseq,
		GSeq:     gseq,
		OSeq:     oseq,
		Provider: provider.String(),
	}, serviceName).Return(nil, kubeStatus)

	path := fmt.Sprintf("/lease/%v/%v/%v/%v/%v/service/%v/status", owner.String(), dseq, gseq, oseq, provider.String(), serviceName)

	scaffold.serveHTTP("GET", path, nil)
	require.Equal(t, scaffold.w.Code, http.StatusNotFound)
	require.Regexp(t, "^fake error(?s:.)*$", scaffold.w.Body.String())
}

func TestRouteServiceStatusError(t *testing.T) {
	scaffold := newRouterForTest(t)

	owner := testutil.AccAddress(t)
	provider := testutil.AccAddress(t)
	dseq := uint64(testutil.RandRangeInt(1, 1000))
	oseq := uint32(testutil.RandRangeInt(2000, 3000))
	gseq := uint32(testutil.RandRangeInt(4000, 5000))

	scaffold.clusterClientMock.On("ServiceStatus", mock.Anything, types.LeaseID{
		Owner:    owner.String(),
		DSeq:     dseq,
		GSeq:     gseq,
		OSeq:     oseq,
		Provider: provider.String(),
	}, serviceName).Return(nil, errGeneric)

	path := fmt.Sprintf("/lease/%v/%v/%v/%v/%v/service/%v/status", owner.String(), dseq, gseq, oseq, provider.String(), serviceName)

	scaffold.serveHTTP("GET", path, nil)
	require.Equal(t, scaffold.w.Code, http.StatusInternalServerError)
	require.Regexp(t, "^generic test error(?s:.)*$", scaffold.w.Body.String())
}
