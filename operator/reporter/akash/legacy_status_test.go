package akash

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	rpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"pkg.akt.dev/node/v2/operator/reporter/internal/simnode"
)

type statusRoundTripper func(*http.Request) (*http.Response, error)

func (f statusRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type statusBody struct {
	io.Reader
	reads  int
	closes int
}

func (b *statusBody) Read(p []byte) (int, error) {
	b.reads++
	return b.Reader.Read(p)
}

func (b *statusBody) Close() error {
	b.closes++
	return nil
}

func TestLegacyHTTPStatusBoundaryAndBodyClose(t *testing.T) {
	for _, code := range []int{200, 201, 204, 205, 299, 300, 301, 302, 303, 304, 307, 308, 399, 400, 401, 403, 404, 429, 500, 503, 599} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			body := &statusBody{Reader: strings.NewReader("private response body")}
			target, err := url.Parse("http://127.0.0.1/private%2Fpath?token=secret")
			require.NoError(t, err)
			transport := &legacyTarget{
				target: *target,
				transport: statusRoundTripper(func(r *http.Request) (*http.Response, error) {
					require.Equal(t, target.String(), r.URL.String())
					return &http.Response{
						StatusCode:    code,
						Status:        "untrusted status text",
						Body:          body,
						ContentLength: -1,
					}, nil
				}),
			}
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://127.0.0.1", nil)
			require.NoError(t, err)
			response, err := transport.RoundTrip(request)
			if code >= 400 {
				require.Nil(t, response)
				require.EqualError(t, err, "legacy RPC HTTP status indicates failure")
				require.Equal(t, 1, body.closes)
				require.Zero(t, body.reads, "failure body must not be read or surfaced")
				return
			}
			require.NoError(t, err)
			require.Zero(t, body.closes)
			got, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, "private response body", string(got))
			require.NoError(t, response.Body.Close())
			require.Equal(t, 1, body.closes)
		})
	}
}

func TestLegacyHTTPSuccessSizeLimitStillClosesBody(t *testing.T) {
	body := &statusBody{Reader: strings.NewReader("unused")}
	transport := &legacyTarget{
		transport: statusRoundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    200,
				Body:          body,
				ContentLength: maxABCIEnvelope + 1,
			}, nil
		}),
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://127.0.0.1", nil)
	require.NoError(t, err)
	response, err := transport.RoundTrip(request)
	require.Nil(t, response)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.Equal(t, 1, body.closes)
	require.Zero(t, body.reads)
}

func TestCanonicalCometHTTPStatusUnchanged(t *testing.T) {
	n, fake := startNode(t, clean(), simnode.Options{})
	original := dial(t, options(n, fake))
	st, err := original.rpc.Status(t.Context())
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request rpctypes.RPCRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		w.WriteHeader(503)
		if err := json.NewEncoder(w).Encode(rpctypes.NewRPCSuccessResponse(request.ID, st)); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	opts := options(n, fake)
	opts.Node = server.URL
	c, err := Dial(t.Context(), opts)
	require.NoError(t, err, "canonical Comet status behavior must remain unchanged")
	t.Cleanup(func() { require.NoError(t, c.Close()) })
}
