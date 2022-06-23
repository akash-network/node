package util

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
)

func (hwsc *httpWrapperServiceClient) CreateRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
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
	return &httpWrapperServiceClient{
		url: baseURL,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}
