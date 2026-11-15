package native

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// RPCCheckRedirect matches the standard client's redirect limit without sending
// the previous endpoint's provider credentials in Referer. Only owned clients
// install this policy; NewWithHTTP leaves the caller's policy untouched.
func RPCCheckRedirect(request *http.Request, via []*http.Request) error {
	request.Header.Del("Referer")
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	return nil
}

// RPCEndpointHTTP keeps provider credentials out of SDK-generated diagnostics.
// The SDK sees only the origin; the private request carries the complete URL.
// The caller's Do owns cookies, deadlines, proxy selection and all redirects, so
// the configured endpoint is restored once, never on a redirected request.
func RPCEndpointHTTP(endpoint string, caller *http.Client) (string, *http.Client, error) {
	return rpcEndpointHTTP(endpoint, caller, false)
}

func rpcEndpointHTTP(endpoint string, caller *http.Client, cometAuthorization bool) (string, *http.Client, error) {
	if caller == nil {
		return "", nil, errors.New("nil RPC HTTP client")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", nil, rpcHTTPFailure{
			message: "invalid RPC endpoint",
			cause:   err,
		}
	}
	parsed.Fragment, parsed.RawFragment = "", ""
	safe := rpcOrigin(parsed)
	client := &http.Client{
		Transport: endpointHTTPTransport{
			client:             caller,
			endpoint:           *parsed,
			cometAuthorization: cometAuthorization,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return safe.String(), client, nil
}

func rpcOrigin(endpoint *url.URL) *url.URL {
	return &url.URL{
		Scheme: endpoint.Scheme,
		Host:   endpoint.Host,
	}
}

type endpointHTTPTransport struct {
	client             *http.Client
	endpoint           url.URL
	cometAuthorization bool
}

func (t endpointHTTPTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	forwarded := request.Clone(request.Context())
	endpoint := t.endpoint
	forwarded.URL = &endpoint
	forwarded.Host = endpoint.Host
	if t.cometAuthorization {
		// Comet sets BasicAuth before Client.Do, so Go copies it on same-host
		// redirects. Solana instead uses URL userinfo; keep those rules distinct.
		username := endpoint.User.Username()
		password, _ := endpoint.User.Password()
		forwarded.URL.User = nil
		if username != "" || password != "" {
			forwarded.SetBasicAuth(username, password)
		}
	}

	response, err := t.client.Do(forwarded)
	if err != nil {
		return nil, redactHTTPError(err)
	}

	// A response can expose the final request to SDK callbacks. Retain the final
	// origin and method, but do not return its credential-bearing URL.
	private := new(http.Response)
	*private = *response
	if response.Request != nil {
		private.Request = response.Request.Clone(response.Request.Context())
		private.Request.URL = rpcOrigin(response.Request.URL)
		private.Request.Response = nil
		private.Request.Header.Del("Authorization")
		private.Request.Header.Del("Proxy-Authorization")
	}
	private.Body = rpcResponseBody{ReadCloser: response.Body}
	return private, nil
}

func (t endpointHTTPTransport) CloseIdleConnections() {
	t.client.CloseIdleConnections()
}

type rpcResponseBody struct {
	io.ReadCloser
}

func (b rpcResponseBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	return n, redactHTTPError(err)
}

func (b rpcResponseBody) Close() error {
	return redactHTTPError(b.ReadCloser.Close())
}

// Preserve the original cause for errors.Is/As without letting ordinary error
// formatting reintroduce URLs. This boundary handles HTTP failures only: RPC
// envelope errors, program messages and simulation results stay unchanged.
type rpcHTTPFailure struct {
	message string
	cause   error
}

func (e rpcHTTPFailure) Error() string { return e.message }

func (e rpcHTTPFailure) Unwrap() error { return e.cause }

func (e rpcHTTPFailure) Timeout() bool {
	var failure interface{ Timeout() bool }
	return errors.As(e.cause, &failure) && failure.Timeout()
}

func (e rpcHTTPFailure) Temporary() bool {
	var failure interface{ Temporary() bool }
	return errors.As(e.cause, &failure) && failure.Temporary()
}

var httpErrorURL = regexp.MustCompile(`(?i)https?://[^\s"'<>]+`)

func redactHTTPError(err error) error {
	if err == nil || err == io.EOF {
		return err
	}

	var message string
	if failure, ok := err.(*url.Error); ok {
		if failure.Op == "parse" {
			message = "invalid RPC URL"
		} else if strings.HasPrefix(failure.Err.Error(), "failed to parse Location header ") {
			message = failure.Op + ": invalid RPC redirect location"
		} else {
			message = failure.Op + ": " + redactHTTPError(failure.Err).Error()
		}
	} else {
		message = err.Error()
	}
	message = httpErrorURL.ReplaceAllString(message, "[RPC endpoint]")
	return rpcHTTPFailure{
		message: message,
		cause:   err,
	}
}
