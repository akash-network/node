package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	rpcclient "github.com/cometbft/cometbft/rpc/client"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rpcReply(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	var input request
	if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&input)) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      input.ID,
		"result":  map[string]any{},
	}))
}

func TestRPCEndpointRequestTarget(t *testing.T) {
	for _, test := range []struct {
		name   string
		suffix string
		target string
	}{
		{
			name:   "no-path",
			target: "/",
		},
		{
			name:   "path",
			suffix: "/akash/rpc",
			target: "/akash/rpc",
		},
		{
			name:   "no-path-query",
			suffix: "?apikey=probe-only",
			target: "/?apikey=probe-only",
		},
		{
			name:   "path-query",
			suffix: "/akash/rpc?apikey=probe-only",
			target: "/akash/rpc?apikey=probe-only",
		},
		{
			name:   "encoded-query-repeated-values",
			suffix: "/akash/rpc?token=%2f%2F%26%3D+%20&key=first&key=second&empty=&bare&odd=%zz",
			target: "/akash/rpc?token=%2f%2F%26%3D+%20&key=first&key=second&empty=&bare&odd=%zz",
		},
		{
			name:   "escaped-path",
			suffix: "/akash%2Frpc/%2f/%252F/%3F/%23/%20?token=%26",
			target: "/akash%2Frpc/%2f/%252F/%3F/%23/%20?token=%26",
		},
		{
			name:   "escaped-path-without-query",
			suffix: "/akash%2Frpc/%2f/%252F",
			target: "/akash%2Frpc/%2f/%252F",
		},
		{
			name:   "empty-query",
			suffix: "/akash/rpc?",
			target: "/akash/rpc?",
		},
		{
			name:   "fragment",
			suffix: "/akash/rpc?token=probe-only#not-transmitted",
			target: "/akash/rpc?token=probe-only",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, test.target, r.RequestURI)
				rpcReply(t, w, r)
			}))
			defer server.Close()

			client, err := NewWithHTTP(server.URL+test.suffix, "test-1", time.Minute, server.Client())
			require.NoError(t, err)
			ctx := context.Background()
			height, page, limit := int64(10), 1, 100

			// These are all Comet RPC methods used by native state, proof and header reads.
			_, err = client.rpc.Status(ctx)
			require.NoError(t, err)
			_, err = client.rpc.ABCIInfo(ctx)
			require.NoError(t, err)
			_, err = client.rpc.Block(ctx, &height)
			require.NoError(t, err)
			_, err = client.rpc.ABCIQueryWithOptions(ctx, "/store/sink/key", []byte{1}, rpcclient.ABCIQueryOptions{
				Height: height,
				Prove:  true,
			})
			require.NoError(t, err)
			_, err = client.rpc.Commit(ctx, &height)
			require.NoError(t, err)
			_, err = client.rpc.Validators(ctx, &height, &page, &limit)
			require.NoError(t, err)
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRPCPreservesHTTPClientAndErrors(t *testing.T) {
	failure := errors.New("transport rejected request")
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	endpoint, err := url.Parse("http://rpc.invalid/akash/rpc")
	require.NoError(t, err)
	jar.SetCookies(endpoint, []*http.Cookie{
		{
			Name:  "session",
			Value: "test-only",
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "/akash/rpc?token=probe-only", r.URL.RequestURI())
		assert.Equal(t, "session=test-only", r.Header.Get("Cookie"))
		cancel()
		assert.ErrorIs(t, r.Context().Err(), context.Canceled)
		return nil, failure
	})
	caller := &http.Client{
		Transport: transport,
		Timeout:   2 * time.Second,
		Jar:       jar,
	}
	before := *caller
	client, err := NewWithHTTP(endpoint.String()+"?token=probe-only", "test-1", time.Minute, caller)
	require.NoError(t, err)
	require.Equal(t, before.Timeout, caller.Timeout)
	require.Same(t, jar, caller.Jar)
	require.Nil(t, caller.CheckRedirect)
	require.Equal(t, reflect.ValueOf(before.Transport).Pointer(), reflect.ValueOf(caller.Transport).Pointer())

	_, err = client.rpc.Status(ctx)
	require.ErrorIs(t, err, failure)
	require.NotContains(t, err.Error(), "probe-only")
}

func TestRPCRedirectPreservesPolicyAndTarget(t *testing.T) {
	var followed int
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		followed++
		assert.Equal(t, "/redirected?own=target", r.RequestURI)
		assert.Equal(t, "caller-controlled", r.Header.Get("Referer"))
		rpcReply(t, w, r)
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/akash/rpc?token=probe-only", r.RequestURI)
		w.Header().Set("Location", destination.URL+"/redirected?own=target")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	policyFailure := errors.New("redirect refused by caller")
	for _, allow := range []bool{false, true} {
		policyCalls := 0
		caller := &http.Client{
			Transport: origin.Client().Transport,
			Timeout:   2 * time.Second,
			CheckRedirect: func(r *http.Request, via []*http.Request) error {
				policyCalls++
				assert.Equal(t, destination.URL+"/redirected?own=target", r.URL.String())
				assert.Len(t, via, 1)
				if !allow {
					return policyFailure
				}
				r.Header.Set("Referer", "caller-controlled")
				return nil
			},
		}
		client, err := NewWithHTTP(origin.URL+"/akash/rpc?token=probe-only", "test-1", time.Minute, caller)
		require.NoError(t, err)
		_, err = client.rpc.Status(context.Background())
		if allow {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, policyFailure)
			require.NotContains(t, err.Error(), "probe-only")
		}
		require.Equal(t, 1, policyCalls)
	}
	require.Equal(t, 1, followed)
}

func TestRPCInvalidEndpointDoesNotExposeQuery(t *testing.T) {
	_, err := NewWithHTTP("http://rpc.invalid/%zz?token=probe-only", "test-1", time.Minute, &http.Client{})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "probe-only")
	var escape url.EscapeError
	require.ErrorAs(t, err, &escape)
}

func TestRPCHTTPTimeout(t *testing.T) {
	caller := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}),
		Timeout: 10 * time.Millisecond,
	}
	client, err := NewWithHTTP("http://rpc.invalid?token=probe-only", "test-1", time.Minute, caller)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = client.rpc.Status(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	var networkError net.Error
	require.ErrorAs(t, err, &networkError)
	require.True(t, networkError.Timeout())
	require.NoError(t, ctx.Err(), "the caller's HTTP timeout should expire before the context deadline")
	require.NotContains(t, err.Error(), "probe-only")
}

func TestRPCDefaultTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/?token=probe-only", r.RequestURI)
		rpcReply(t, w, r)
	}))
	defer server.Close()
	caller := &http.Client{Timeout: time.Second}
	defer caller.CloseIdleConnections()

	client, err := NewWithHTTP(server.URL+"?token=probe-only", "test-1", time.Minute, caller)
	require.NoError(t, err)
	_, err = client.rpc.Status(context.Background())
	require.NoError(t, err)
	require.Nil(t, caller.Transport)
}

func TestRPCRelativeRedirectUsesCompleteURL(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, "/akash/rpc?token=probe-only", r.RequestURI)
		if calls == 1 {
			w.Header().Set("Location", "#redirected")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		rpcReply(t, w, r)
	}))
	defer server.Close()

	client, err := NewWithHTTP(server.URL+"/akash/rpc?token=probe-only", "test-1", time.Minute, server.Client())
	require.NoError(t, err)
	_, err = client.rpc.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

func TestRPCUsesLastResponseWhenCallerRequestsIt(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, "/akash/rpc?token=probe-only", r.RequestURI)
		w.Header().Set("Location", "/must-not-follow")
		w.WriteHeader(http.StatusTemporaryRedirect)
		rpcReply(t, w, r)
	}))
	defer server.Close()
	caller := &http.Client{
		Transport: server.Client().Transport,
		Timeout:   time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	client, err := NewWithHTTP(server.URL+"/akash/rpc?token=probe-only", "test-1", time.Minute, caller)
	require.NoError(t, err)
	_, err = client.rpc.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

func TestRPCQueryTransportDoesNotMutateSharedRequest(t *testing.T) {
	failure := errors.New("offline transport")
	caller := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			assert.Equal(t, "/akash%2Frpc?token=%2f&key=1&key=2", r.URL.RequestURI())
			r.URL.Path = "/modified-by-caller-transport"
			r.Header.Set("X-Test", "modified-by-caller-transport")
			return nil, failure
		}),
	}
	endpoint, err := url.Parse("http://rpc.invalid/akash%2Frpc?token=%2f&key=1&key=2")
	require.NoError(t, err)
	transport := endpointHTTPTransport{
		client:   caller,
		endpoint: *endpoint,
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://rpc.invalid/akash%2Frpc?original=value", nil)
	require.NoError(t, err)
	request.Header.Set("X-Test", "original")
	original := request.Clone(request.Context())

	var group sync.WaitGroup
	for range 16 {
		group.Go(func() {
			_, err := transport.RoundTrip(request)
			assert.ErrorIs(t, err, failure)
		})
	}
	group.Wait()
	require.Equal(t, original.URL, request.URL)
	require.Equal(t, original.Header, request.Header)
}

func TestRPCDefaultBypassesProxyAndCustomClientPreservesIt(t *testing.T) {
	var directCalls, proxyCalls atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		directCalls.Add(1)
		assert.Equal(t, "rpc.invalid", r.Host)
		rpcReply(t, w, r)
	}))
	defer origin.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls.Add(1)
		assert.Equal(t, "rpc.invalid", r.URL.Host)
		rpcReply(t, w, r)
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)

	original := http.DefaultTransport
	guarded := original.(*http.Transport).Clone()
	guarded.Proxy = http.ProxyURL(proxyURL)
	guarded.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		// Map the test name before DNS. Reject every non-probe destination.
		switch address {
		case "rpc.invalid:80":
			address = origin.Listener.Addr().String()
		case proxy.Listener.Addr().String():
		default:
			return nil, errors.New("blocked non-probe connection")
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	http.DefaultTransport = guarded
	t.Cleanup(func() {
		http.DefaultTransport = original
		guarded.CloseIdleConnections()
	})

	for _, suffix := range []string{"/rpc", "/rpc?token=probe-only"} {
		client, err := New("http://rpc.invalid"+suffix, "test-1", time.Minute)
		require.NoError(t, err)
		_, err = client.rpc.Status(context.Background())
		require.NoError(t, err)
	}
	require.Equal(t, int32(2), directCalls.Load())
	require.Zero(t, proxyCalls.Load())
	require.Same(t, guarded, http.DefaultTransport)
	require.NotNil(t, guarded.Proxy)

	caller := &http.Client{
		Transport: guarded,
		Timeout:   time.Second,
	}
	for _, suffix := range []string{"/rpc", "/rpc?token=probe-only"} {
		client, err := NewWithHTTP("http://rpc.invalid"+suffix, "test-1", time.Minute, caller)
		require.NoError(t, err)
		_, err = client.rpc.Status(context.Background())
		require.NoError(t, err)
	}
	require.Equal(t, int32(2), directCalls.Load())
	require.Equal(t, int32(2), proxyCalls.Load())
	require.Same(t, guarded, caller.Transport)
	require.NotNil(t, guarded.Proxy)
}

func TestRPCOwnedRedirectPrivacy(t *testing.T) {
	for _, status := range []int{302, 307, 308} {
		for _, crossOrigin := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/cross-origin=%t", status, crossOrigin), func(t *testing.T) {
				const originalTarget = "/path-token-probe?token=query-token-probe&key=%2f&key=2"
				const redirectTarget = "/redirect-token-probe?own=target-token-probe"
				originalBody := make(chan []byte, 1)
				finish := func(w http.ResponseWriter, r *http.Request) {
					assert.Empty(t, r.Header.Get("Referer"))
					assert.Equal(t, redirectTarget, r.RequestURI)
					body, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					original := <-originalBody
					if status == 302 {
						assert.Equal(t, http.MethodGet, r.Method)
						assert.Empty(t, body)
					} else {
						assert.Equal(t, http.MethodPost, r.Method)
						assert.Equal(t, original, body)
					}
					var input request
					assert.NoError(t, json.Unmarshal(original, &input))
					assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"jsonrpc": "2.0",
						"id":      input.ID,
						"result":  map[string]any{},
					}))
				}
				destination := httptest.NewServer(http.HandlerFunc(finish))
				defer destination.Close()
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/redirect-token-probe" {
						finish(w, r)
						return
					}
					assert.Equal(t, originalTarget, r.RequestURI)
					assert.Equal(t, http.MethodPost, r.Method)
					body, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					originalBody <- body
					location := redirectTarget
					if crossOrigin {
						location = destination.URL + location
					}
					w.Header().Set("Location", location)
					w.WriteHeader(status)
				}))
				defer origin.Close()

				client, err := New(origin.URL+originalTarget, "test-1", time.Minute)
				require.NoError(t, err)
				_, err = client.rpc.Status(context.Background())
				require.NoError(t, err)
			})
		}
	}
}

func TestRPCOwnedRedirectLimit(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Empty(t, r.Header.Get("Referer"))
		w.Header().Set("Location", "/redirect-token-probe?token=redirect-query-probe")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	client, err := New(server.URL+"/path-token-probe?token=query-token-probe", "test-1", time.Minute)
	require.NoError(t, err)
	_, err = client.rpc.Status(context.Background())
	require.ErrorContains(t, err, "stopped after 10 redirects")
	require.NotContains(t, err.Error(), "probe")
	require.Equal(t, int64(10), calls.Load())
}

func TestRPCEndpointPrivacyAndCauseIdentity(t *testing.T) {
	failure := errors.New("wrapped transport to http://user-probe:password-probe@rpc.invalid/path-token-probe?token=query-token-probe")
	cause := &url.Error{
		Op:  "Post",
		URL: "http://user-probe:password-probe@rpc.invalid/path-token-probe?token=query-token-probe",
		Err: failure,
	}
	caller := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			username, password, ok := r.BasicAuth()
			assert.True(t, ok)
			assert.Equal(t, "user-probe", username)
			assert.Equal(t, "password-probe", password)
			assert.Equal(t, "/path-token-probe?token=query-token-probe", r.URL.RequestURI())
			return nil, cause
		}),
	}
	client, err := NewWithHTTP(cause.URL+"#fragment-probe", "test-1", time.Minute, caller)
	require.NoError(t, err)
	_, err = client.rpc.Status(context.Background())
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, failure)
	var extracted *url.Error
	require.ErrorAs(t, err, &extracted)
	require.ErrorIs(t, extracted, cause)
	require.NotContains(t, fmt.Errorf("outer: %w", err).Error(), "probe")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client, err = New("http://127.0.0.1:1/path-token-probe?token=query-token-probe", "test-1", time.Minute)
	require.NoError(t, err)
	_, err = client.rpc.Status(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.NotContains(t, err.Error(), "probe")
}

func TestRPCEndpointResponseMetadataAndBodyTimeout(t *testing.T) {
	for _, stalled := range []bool{false, true} {
		t.Run(fmt.Sprintf("stalled=%t", stalled), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				if stalled {
					<-r.Context().Done()
					return
				}
				fmt.Fprint(w, "body")
			}))
			defer server.Close()
			caller := &http.Client{
				Transport: server.Client().Transport,
				Timeout:   50 * time.Millisecond,
			}
			safe, client, err := RPCEndpointHTTP(server.URL+"/path-token-probe?token=query-token-probe", caller)
			require.NoError(t, err)
			require.Equal(t, server.URL, safe)
			response, err := client.Get(safe)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, server.URL, response.Request.URL.String())
			body, err := io.ReadAll(response.Body)
			if stalled {
				require.ErrorIs(t, err, context.DeadlineExceeded)
				var networkError net.Error
				require.ErrorAs(t, err, &networkError)
				require.True(t, networkError.Timeout())
				require.NotContains(t, err.Error(), "probe")
			} else {
				require.NoError(t, err)
				require.Equal(t, "body", string(body))
			}
		})
	}
}

func TestRPCMalformedRedirectPrivacy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/redirect-token-probe/%zz?token=query-token-probe")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client, err := New(server.URL+"/path-token-probe?token=query-token-probe", "test-1", time.Minute)
	require.NoError(t, err)
	_, err = client.rpc.Status(context.Background())
	require.ErrorContains(t, err, "invalid RPC redirect location")
	require.NotContains(t, err.Error(), "probe")
}

func solanaHTTPClient(t *testing.T, endpoint string, caller *http.Client) *rpc.Client {
	t.Helper()
	safe, client, err := RPCEndpointHTTP(endpoint, caller)
	require.NoError(t, err)
	return rpc.NewWithCustomRPCClient(jsonrpc.NewClientWithOpts(safe, &jsonrpc.RPCClientOpts{
		HTTPClient: client,
	}))
}

func TestSolanaRPCFailurePrivacy(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{
			name:   "malformed",
			status: 200,
			body:   "{",
			want:   "could not decode body",
		},
		{
			name:   "null",
			status: 200,
			body:   "null",
			want:   "rpc response missing",
		},
		{
			name:   "unknown-field",
			status: 200,
			body:   `{"jsonrpc":"2.0","unexpected":true}`,
			want:   "could not decode body",
		},
		{
			name:   "http-malformed",
			status: 503,
			body:   "bad gateway",
			want:   "could not decode body",
		},
		{
			name:   "http-null",
			status: 503,
			body:   "null",
			want:   "rpc response missing",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/path-token-probe?token=query-token-probe", r.RequestURI)
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			endpoint := strings.Replace(server.URL, "http://", "http://user-probe:password-probe@", 1)
			client := solanaHTTPClient(t, endpoint+"/path-token-probe?token=query-token-probe#fragment-probe", server.Client())
			_, err := client.GetGenesisHash(context.Background())
			require.ErrorContains(t, err, test.want)
			require.NotContains(t, fmt.Errorf("alert: %w", err).Error(), "probe")
			if test.status >= 400 {
				var failure *jsonrpc.HTTPError
				require.ErrorAs(t, err, &failure)
				require.Equal(t, test.status, failure.Code)
			}
		})
	}
}

func TestSolanaRPCTransportAndBodyErrors(t *testing.T) {
	t.Run("refused", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		server.Close()
		client := solanaHTTPClient(t, server.URL+"/path-token-probe?token=query-token-probe", server.Client())
		_, err := client.GetGenesisHash(context.Background())
		require.Error(t, err)
		var failure *net.OpError
		require.ErrorAs(t, err, &failure)
		require.NotContains(t, err.Error(), "probe")
	})

	for _, body := range []bool{false, true} {
		t.Run(fmt.Sprintf("body=%t", body), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if body {
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			caller := &http.Client{
				Transport: server.Client().Transport,
				Timeout:   50 * time.Millisecond,
			}
			client := solanaHTTPClient(t, server.URL+"/path-token-probe?token=query-token-probe", caller)
			started := time.Now()
			_, err := client.GetGenesisHash(context.Background())
			require.Less(t, time.Since(started), 2*time.Second)
			if body {
				// The pinned SDK decoder reports EOF for a zero-byte body read
				// failure. The HTTP seam still enforces the caller's deadline.
				require.ErrorIs(t, err, io.EOF)
			} else {
				require.ErrorIs(t, err, context.DeadlineExceeded)
				var failure net.Error
				require.ErrorAs(t, err, &failure)
				require.True(t, failure.Timeout())
			}
			require.NotContains(t, err.Error(), "probe")
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := solanaHTTPClient(t, "http://127.0.0.1:1/path-token-probe?token=query-token-probe", &http.Client{})
	_, err := client.GetGenesisHash(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.NotContains(t, err.Error(), "probe")
}

func TestSolanaRPCPreservesStructuredAndProgramErrors(t *testing.T) {
	const message = "Transaction simulation failed: custom program error: 0x1771"
	const programLog = "Program log: see https://docs.example.org/program/errors?code=6001"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&input))
		response := map[string]any{
			"jsonrpc": "2.0",
			"id":      input.ID,
		}
		if input.Method == "simulateTransaction" {
			response["result"] = map[string]any{
				"context": map[string]any{"slot": 42},
				"value": map[string]any{
					"err":  map[string]any{"InstructionError": []any{0, map[string]any{"Custom": 6001}}},
					"logs": []string{message, programLog},
				},
			}
		} else {
			response["error"] = map[string]any{
				"code":    -32002,
				"message": message,
				"data": map[string]any{
					"err":  map[string]any{"InstructionError": []any{0, map[string]any{"Custom": 6001}}},
					"logs": []string{programLog},
				},
			}
		}
		assert.NoError(t, json.NewEncoder(w).Encode(response))
	}))
	defer server.Close()
	client := solanaHTTPClient(t, server.URL+"/path-token-probe?token=query-token-probe", server.Client())
	_, err := client.GetGenesisHash(context.Background())
	var failure *jsonrpc.RPCError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, -32002, failure.Code)
	require.Equal(t, message, failure.Message)
	encoded, err := json.Marshal(failure.Data)
	require.NoError(t, err)
	require.JSONEq(t, `{"err":{"InstructionError":[0,{"Custom":6001}]},"logs":["`+programLog+`"]}`, string(encoded))
	require.Contains(t, failure.Error(), programLog)

	result, err := client.SimulateRawTransactionWithOpts(context.Background(), []byte("inert-loopback-fixture"), nil)
	require.NoError(t, err)
	require.Equal(t, uint64(42), result.Context.Slot)
	require.Equal(t, []string{message, programLog}, result.Value.Logs)
	encoded, err = json.Marshal(result.Value.Err)
	require.NoError(t, err)
	require.JSONEq(t, `{"InstructionError":[0,{"Custom":6001}]}`, string(encoded))
}

func TestSolanaRPCCallbackAndBatchPrivacy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "null")
	}))
	defer server.Close()
	client := solanaHTTPClient(t, server.URL+"/path-token-probe?token=query-token-probe", server.Client())
	err := client.RPCCallWithCallback(context.Background(), "probe", nil, func(request *http.Request, response *http.Response) error {
		assert.Equal(t, server.URL, request.URL.String())
		assert.Equal(t, server.URL, response.Request.URL.String())
		return nil
	})
	require.NoError(t, err)
	_, err = client.RPCCallBatch(context.Background(), jsonrpc.RPCRequests{jsonrpc.NewRequest("getGenesisHash")})
	require.ErrorContains(t, err, "rpc response missing")
	require.NotContains(t, err.Error(), "token-probe")
}

func TestRPCAllMethodParametersSurviveEndpointAdapter(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/path-token-probe?token=query-token-probe", r.RequestURI)
		var input request
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&input))
		calls.Add(1)
		switch input.Method {
		case "status", "abci_info":
			assert.Empty(t, input.Params)
		case "block", "commit":
			assert.Len(t, input.Params, 1)
			assert.JSONEq(t, `"10"`, string(input.Params["height"]))
		case "validators":
			assert.Len(t, input.Params, 3)
			assert.JSONEq(t, `"10"`, string(input.Params["height"]))
			assert.JSONEq(t, `"2"`, string(input.Params["page"]))
			assert.JSONEq(t, `"37"`, string(input.Params["per_page"]))
		case "abci_query":
			assert.Len(t, input.Params, 4)
			assert.JSONEq(t, `"/store/sink/key"`, string(input.Params["path"]))
			assert.JSONEq(t, `"0102"`, string(input.Params["data"]))
			assert.JSONEq(t, `"10"`, string(input.Params["height"]))
			assert.JSONEq(t, `true`, string(input.Params["prove"]))
		default:
			t.Errorf("unexpected method: %s", input.Method)
		}
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      input.ID,
			"result":  map[string]any{},
		}))
	}))
	defer server.Close()
	client, err := New(server.URL+"/path-token-probe?token=query-token-probe", "test-1", time.Minute)
	require.NoError(t, err)
	ctx := context.Background()
	height, page, limit := int64(10), 2, 37
	_, err = client.rpc.Status(ctx)
	require.NoError(t, err)
	_, err = client.rpc.ABCIInfo(ctx)
	require.NoError(t, err)
	_, err = client.rpc.Block(ctx, &height)
	require.NoError(t, err)
	_, err = client.rpc.ABCIQueryWithOptions(ctx, "/store/sink/key", []byte{1, 2}, rpcclient.ABCIQueryOptions{
		Height: height,
		Prove:  true,
	})
	require.NoError(t, err)
	_, err = client.rpc.Commit(ctx, &height)
	require.NoError(t, err)
	_, err = client.rpc.Validators(ctx, &height, &page, &limit)
	require.NoError(t, err)
	require.Equal(t, int64(6), calls.Load())
}

func TestRPCAuthorizationRedirectSemantics(t *testing.T) {
	for _, chain := range []string{"akash", "solana"} {
		for _, crossHost := range []bool{false, true} {
			for _, mode := range []string{"userinfo", "caller", "policy"} {
				t.Run(fmt.Sprintf("%s/cross-host=%t/%s", chain, crossHost, mode), func(t *testing.T) {
					const basic = "Basic dXNlci1wcm9iZTpwYXNzd29yZC1wcm9iZQ=="
					const callerAuth = "Bearer caller-probe"
					var calls atomic.Int64
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if r.URL.Path == "/original" {
							expected := basic
							if mode == "caller" {
								expected = callerAuth
							}
							assert.Equal(t, expected, r.Header.Get("Authorization"))
							host := "origin.invalid"
							if crossHost {
								host = "unrelated.invalid"
							}
							w.Header().Set("Location", "http://"+host+"/redirected")
							w.WriteHeader(http.StatusTemporaryRedirect)
							return
						}

						expected := ""
						if chain == "akash" && !crossHost {
							expected = basic
						}
						if mode == "policy" || mode == "caller" && (chain == "akash" || !crossHost) {
							expected = callerAuth
						}
						assert.Equal(t, expected, r.Header.Get("Authorization"))
						var input request
						assert.NoError(t, json.NewDecoder(r.Body).Decode(&input))
						var result any = map[string]any{}
						if chain == "solana" {
							result = "11111111111111111111111111111111"
						}
						assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
							"jsonrpc": "2.0",
							"id":      input.ID,
							"result":  result,
						}))
					}))
					defer server.Close()

					transport := http.DefaultTransport.(*http.Transport).Clone()
					transport.Proxy = nil
					transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
						if address != "origin.invalid:80" && address != "unrelated.invalid:80" {
							return nil, errors.New("blocked non-probe destination")
						}
						return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
					}
					defer transport.CloseIdleConnections()
					caller := &http.Client{
						Transport: transport,
						Timeout:   time.Second,
					}
					if chain == "akash" && mode == "caller" {
						// Comet has no custom-header option. Its caller may explicitly
						// supply credentials in its transport, including after redirects.
						caller.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
							r.Header.Set("Authorization", callerAuth)
							return transport.RoundTrip(r)
						})
					}
					if mode == "policy" {
						caller.CheckRedirect = func(r *http.Request, via []*http.Request) error {
							r.Header.Set("Authorization", callerAuth)
							return nil
						}
					}

					endpoint := "http://user-probe:password-probe@origin.invalid/original?token=query-token-probe"
					if chain == "akash" {
						client, err := NewWithHTTP(endpoint, "test-1", time.Minute, caller)
						require.NoError(t, err)
						_, err = client.rpc.Status(context.Background())
						require.NoError(t, err)
					} else {
						rpcEndpoint, rpcHTTP, err := RPCEndpointHTTP(endpoint, caller)
						require.NoError(t, err)
						headers := map[string]string{}
						if mode == "caller" {
							headers["Authorization"] = callerAuth
						}
						client := rpc.NewWithCustomRPCClient(jsonrpc.NewClientWithOpts(rpcEndpoint, &jsonrpc.RPCClientOpts{
							HTTPClient:    rpcHTTP,
							CustomHeaders: headers,
						}))
						_, err = client.GetGenesisHash(context.Background())
						require.NoError(t, err)
					}
					require.Equal(t, int64(2), calls.Load())
				})
			}
		}
	}
}

func TestRPCEmptyUserinfoAuthorization(t *testing.T) {
	for _, userinfo := range []string{"", "@", ":@"} {
		for _, chain := range []string{"akash", "solana"} {
			t.Run(chain+"/userinfo="+userinfo, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					step := calls.Add(1)
					_, _ = io.Copy(io.Discard, r.Body)
					want := ""
					if chain == "solana" && userinfo != "" {
						want = "Basic Og=="
					}
					if got := r.Header.Get("Authorization"); got != want {
						t.Errorf("hop %d Authorization: got %q want baseline %q", step, got, want)
					}
					if step == 1 {
						w.Header().Set("Location", "relative?probe=redirect")
						w.WriteHeader(307)
						return
					}
					_, _ = io.WriteString(w, "{")
				}))
				defer server.Close()
				endpoint := strings.Replace(server.URL, "http://", "http://"+userinfo, 1) + "/rpc?token=probe"
				var err error
				if chain == "akash" {
					var client *Client
					client, err = NewWithHTTP(endpoint, "test-1", time.Minute, server.Client())
					require.NoError(t, err)
					_, err = client.rpc.Status(context.Background())
				} else {
					client := solanaHTTPClient(t, endpoint, server.Client())
					_, err = client.GetGenesisHash(context.Background())
				}
				require.Error(t, err, "fixture should fail JSON decoding")
				if calls.Load() != 2 {
					t.Fatalf("got %d calls", calls.Load())
				}
			})
		}
	}
}
