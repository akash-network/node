package util

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
)

func (hwsc *httpWrapperServiceClient) CreateRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	path = strings.TrimLeft(path, "/")
	serviceURL := fmt.Sprintf("%s/%s", hwsc.url, path)
	req, err := http.NewRequestWithContext(ctx, method, serviceURL, body)
	if err != nil {
		return nil, err
	}

	return req, nil
}

func (hwsc *httpWrapperServiceClient) DoRequest(req *http.Request) (*http.Response, error) {
	return hwsc.httpClient.Do(req)
}

func newHTTPWrapperServiceClient(isHTTPS, secure bool, baseURL string) *httpWrapperServiceClient {
	baseURL = strings.TrimRight(baseURL, "/")
	netDialer := &net.Dialer{}

	// By default, block both things
	netDial := func(_ context.Context, network, addr string) (net.Conn, error) {
		return nil, fmt.Errorf("%w: cannot connect to %v:%v TLS must be used", errServiceClient, network, addr)
	}
	dialTLS := func(_ context.Context, network string, addr string) (net.Conn, error) {
		return nil, fmt.Errorf("%w: cannot connect to %v:%v TLS is not supported", errServiceClient, network, addr)
	}

	// Unblock one of the dial methods
	if isHTTPS {
		tlsDialer := tls.Dialer{
			NetDialer: netDialer,
			Config: &tls.Config{
				InsecureSkipVerify: !secure, // nolint:gosec
			},
		}
		dialTLS = tlsDialer.DialContext
	} else {
		netDial = netDialer.DialContext
	}

	transport := &http.Transport{
		DialContext:     netDial,
		DialTLSContext:  dialTLS,
		MaxIdleConns:    2,
		MaxConnsPerHost: 2,
	}
	return newHTTPWrapperServiceClientWithTransport(transport, baseURL)
}

func newHTTPWrapperServiceClientWithTransport(transport http.RoundTripper, baseURL string) *httpWrapperServiceClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &httpWrapperServiceClient{
		url: baseURL,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}


func newWebsocketWrapperServiceClientFromDialer(dialer websocket.Dialer, baseURL string) *websocketWrapperServiceClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &websocketWrapperServiceClient{
		url: baseURL,
		dialer: &dialer,
	}
}

func (wwsc *websocketWrapperServiceClient) DialWebsocket(ctx context.Context, path string, requestHeaders http.Header) (*websocket.Conn, error) {
	path = strings.TrimLeft(path, "/")
	dialUrl := fmt.Sprintf("%s/%s", wwsc.url, path)

	if strings.HasPrefix(dialUrl, "https") {
		dialUrl = strings.Replace(dialUrl, "https", "wss", 1)
	} else if strings.HasPrefix(dialUrl, "http") {
		dialUrl = strings.Replace(dialUrl, "http", "ws", 1)
	}

	conn, resp, err  := wwsc.dialer.DialContext(ctx, dialUrl, requestHeaders)
	if err != nil {
		if resp == nil {
			return nil, err
		}

		buf, _ := ioutil.ReadAll(resp.Body) // nolint
		bufStr := strings.ReplaceAll(string(buf), "\n", " ")
		return nil, fmt.Errorf("%w: error response from server when dialing websocket %q; status %v; response: %s", err, dialUrl, resp.StatusCode,
			bufStr)
	}

	return conn, err

}