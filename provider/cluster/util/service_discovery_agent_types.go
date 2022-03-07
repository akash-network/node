package util

import (
	"context"
	"github.com/boz/go-lifecycle"
	"github.com/gorilla/websocket"
	"github.com/tendermint/tendermint/libs/log"
	"io"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/http"
)

type ServiceDiscoveryAgent interface {
	Stop()
	GetClient(ctx context.Context, isHTTPS, secure bool) (ServiceClient, error)
	GetWebsocketClient(ctx context.Context, isHTTPS, secure bool) (WebsocketServiceClient, error)
	DiscoverNow()
}

type ServiceClient interface {
	CreateRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error)
	DoRequest(req *http.Request) (*http.Response, error)
}

type WebsocketServiceClient interface {
	DialWebsocket(ctx context.Context, path string, requestHeader http.Header) (*websocket.Conn, error)
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

type clientFactory interface {
	MakeServiceClient(isHTTPS, secure bool) ServiceClient
	MakeWebsocketServiceClient(isHTTPS, secure bool)  WebsocketServiceClient
}

type httpWrapperServiceClient struct {
	httpClient *http.Client
	url        string
}

type websocketWrapperServiceClient struct {
	url        string
	dialer *websocket.Dialer
}

