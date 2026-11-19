package solana

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLegacySolanaRequestTargetsAndRedirects(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, redirect := range []int{0, 301, 302, 303, 307, 308} {
			for _, cross := range []bool{false, true} {
				t.Run(strconv.FormatBool(legacy)+"/"+strconv.Itoa(redirect)+"/cross="+strconv.FormatBool(cross), func(t *testing.T) {
					var mu sync.Mutex
					var id json.RawMessage
					var targets, methods, refs, payloads, contentTypes []string
					var bodies []bool
					handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						data, err := io.ReadAll(r.Body)
						require.NoError(t, err)
						mu.Lock()
						defer mu.Unlock()
						if len(data) > 0 {
							var request struct {
								ID json.RawMessage `json:"id"`
							}

							require.NoError(t, json.Unmarshal(data, &request))
							id = request.ID
						}

						targets = append(targets, r.RequestURI)
						methods = append(methods, r.Method)
						refs = append(refs, r.Referer())
						bodies = append(bodies, len(data) > 0)
						payloads = append(payloads, string(data))
						contentTypes = append(contentTypes, r.Header.Get("Content-Type"))
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(id) + `,"result":"r176-genesis"}`))
					})
					other := httptest.NewUnstartedServer(handler)
					listener, err := net.Listen("tcp", "[::1]:0")
					require.NoError(t, err)
					other.Listener = listener
					other.Start()
					defer other.Close()
					var origin string
					first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if redirect == 0 || strings.HasPrefix(r.URL.Path, "/final") {
							handler.ServeHTTP(w, r)
							return
						}

						data, err := io.ReadAll(r.Body)
						require.NoError(t, err)
						var request struct {
							ID json.RawMessage `json:"id"`
						}

						require.NoError(t, json.Unmarshal(data, &request))
						mu.Lock()
						id = request.ID
						targets = append(targets, r.RequestURI)
						methods = append(methods, r.Method)
						refs = append(refs, r.Referer())
						bodies = append(bodies, len(data) > 0)
						payloads = append(payloads, string(data))
						contentTypes = append(contentTypes, r.Header.Get("Content-Type"))
						mu.Unlock()
						location := origin + "/final%2Fpath?keep=ok"
						if cross {
							location = other.URL + "/final%2Fpath?keep=ok"
						}

						w.Header().Set("Location", location)
						w.WriteHeader(redirect)
					}))
					defer first.Close()
					origin = first.URL
					rpc := NewRPC(RPCOptions{
						LegacyRedirects: legacy,
						URL:             origin + "/initial%2Fpath?token=SOURCE",
						Timeout:         time.Second,
					})
					defer rpc.http.CloseIdleConnections()
					result, err := rpc.GenesisHash(t.Context())
					require.NoError(t, err)
					require.Equal(t, "r176-genesis", result)
					mu.Lock()
					defer mu.Unlock()
					require.Equal(t, "/initial%2Fpath?token=SOURCE", targets[0])
					require.Equal(t, "POST", methods[0])
					require.True(t, bodies[0])
					if redirect != 0 {
						require.Len(t, targets, 2)
						require.Equal(t, "/final%2Fpath?keep=ok", targets[1])
						if redirect == 307 || redirect == 308 {
							require.Equal(t, "POST", methods[1])
							require.True(t, bodies[1])
							require.Equal(t, payloads[0], payloads[1])
							require.Equal(t, contentTypes[0], contentTypes[1])
						} else {
							require.Equal(t, "GET", methods[1])
							require.False(t, bodies[1])
							require.Empty(t, contentTypes[1])
						}

						if legacy {
							require.Empty(t, refs[1])
						} else {
							require.Contains(t, refs[1], "token=SOURCE")
						}
					}
				})
			}
		}
	}
}

func TestLegacySolanaRedirectLimitsAuthorizationAndErrors(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, scenario := range []string{"hop9", "hop10", "hop20", "hop21", "credentials", "invalid"} {
			t.Run(strconv.FormatBool(legacy)+"/"+scenario, func(t *testing.T) {
				var requests int
				var authorization bool
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests++
					if r.URL.Path == "/final" {
						authorization = r.Header.Get("Authorization") != ""
					} else if scenario == "credentials" {
						w.Header().Set("Location", "http://r176:SECRET@"+r.Host+"/final")
						w.WriteHeader(307)
						return
					} else if scenario == "invalid" {
						w.Header().Set("Location", "http://SECRET:%Q/")
						w.WriteHeader(307)
						return
					} else {
						hops, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/hop"))
						require.NoError(t, err)
						if hops > 0 {
							w.Header().Set("Location", "/hop"+strconv.Itoa(hops-1))
							w.WriteHeader(307)
							return
						}
					}

					var request struct {
						ID json.RawMessage `json:"id"`
					}

					require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
					_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(request.ID) + `,"result":"genesis"}`))
				}))
				defer server.Close()
				rpc := NewRPC(RPCOptions{
					LegacyRedirects: legacy,
					URL:             server.URL + "/" + scenario + "?token=SOURCE",
					Timeout:         time.Second,
				})
				defer rpc.http.CloseIdleConnections()
				_, err := rpc.GenesisHash(t.Context())
				success := scenario == "hop9" || (legacy && (scenario == "hop10" || scenario == "hop20")) || (!legacy && scenario == "credentials")
				if success {
					require.NoError(t, err)
				} else {
					require.EqualError(t, err, ErrUnavailable.Error())
					require.NotContains(t, err.Error(), "SECRET")
					require.NotContains(t, err.Error(), "SOURCE")
				}

				if scenario == "credentials" {
					require.Equal(t, !legacy, authorization)
					if legacy {
						require.Equal(t, 1, requests)
					} else {
						require.Equal(t, 2, requests)
					}
				}

				if scenario == "hop20" && legacy {
					require.Equal(t, 21, requests)
				}

				if scenario == "hop21" && legacy {
					require.Equal(t, 21, requests)
				}
			})
		}
	}
}
