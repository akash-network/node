package util

import (
	"context"
	"github.com/boz/go-lifecycle"
	"github.com/tendermint/tendermint/libs/log"
	"io"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/http"
)

type ServiceDiscoveryAgent interface {
	Stop()
	GetClient(ctx context.Context, isHTTPS, secure bool) (ServiceClient, error)
	DiscoverNow()
}

type ServiceClient interface {
	CreateRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error)
	DoRequest(req *http.Request) (*http.Response, error)
}

type serviceDiscoveryAgent struct {
	serviceName string
	namespace   string
	portName    string
	lc          lifecycle.Lifecycle

	discoverch chan struct{}

	requests        chan serviceDiscoveryRequest
	pendingRequests []serviceDiscoveryRequest
	result          clientFactory
	log             log.Logger

	kube       kubernetes.Interface
	kubeConfig *rest.Config
}

type serviceDiscoveryRequest struct {
	errCh    chan<- error
	resultCh chan<- clientFactory
}

type clientFactory func(isHttps, secure bool) ServiceClient

type httpWrapperServiceClient struct {
	httpClient *http.Client
	url        string
}
