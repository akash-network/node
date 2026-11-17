package akash_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	rpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/akash"
	"pkg.akt.dev/node/v2/operator/reporter/metrics"
	"pkg.akt.dev/node/v2/operator/reporter/policy"
	reportertx "pkg.akt.dev/node/v2/operator/reporter/tx"
)

func TestLegacyHTTPStatusPruneMetricsAndPrivacy(t *testing.T) {
	for _, phase := range []string{"dial", "simulation"} {
		for _, code := range []int{200, 201, 299, 300, 399, 400, 401, 403, 404, 429, 500, 503, 599} {
			t.Run(phase+"/"+strconv.Itoa(code), func(t *testing.T) {
				value, err := codec.ProtoMarshalJSON(&sdk.SimulationResponse{
					GasInfo: sdk.GasInfo{
						GasUsed: 777,
					},
				}, nil)
				require.NoError(t, err)
				var calls atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.RequestURI != "/private%2Fpath?token=secret" {
						t.Errorf("configured escaped target not retained: %s", r.RequestURI)
					}
					var request rpctypes.RPCRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						w.WriteHeader(500)
						return
					}
					statusCode := code
					var result any
					switch request.Method {
					case "status":
						result = &ctypes.ResultStatus{}
						if phase == "simulation" {
							statusCode = 200
						}
					case "abci_query":
						var responseLog string
						if statusCode >= 400 {
							responseLog = "response-secret message index: 0"
						}

						result = &ctypes.ResultABCIQuery{
							Response: abci.ResponseQuery{
								Height: 42,
								Value:  value,
								Log:    responseLog,
							},
						}
					default:
						t.Errorf("unexpected method, especially any send: %s", request.Method)
						w.WriteHeader(500)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(statusCode)
					if err := json.NewEncoder(w).Encode(rpctypes.NewRPCSuccessResponse(request.ID, result)); err != nil {
						t.Error(err)
					}
				}))
				t.Cleanup(server.Close)
				reg := prometheus.NewRegistry()
				c, err := akash.Dial(t.Context(), akash.Options{
					ABCI:           true,
					Node:           server.URL + "/private%2Fpath?token=secret",
					RequestTimeout: time.Second,
					Metrics:        metrics.New(reg),
				})
				if c != nil {
					t.Cleanup(func() { require.NoError(t, c.Close()) })
				}

				var wantCalls uint64 = 1
				if phase == "simulation" {
					require.NoError(t, err)
					batch := []policy.Planned{
						{
							Msg: &sinkv1.MsgReportOutcome{Round: 1},
						},
						{
							Msg: &sinkv1.MsgReportOutcome{Round: 2},
						},
					}
					attempts := 0
					result := reportertx.Prune(func(msgs []sdk.Msg) (uint64, error) {
						attempts++
						require.Len(t, msgs, len(batch))
						// Only the raw simulation request reaches the mock; no builder or signer runs.
						return c.Simulate(t.Context(), []byte("unsigned synthetic simulation"))
					}, batch)
					require.Equal(t, 1, attempts)
					require.Empty(t, result.Dropped)
					require.Empty(t, result.Deferred)
					if code >= 400 {
						require.Empty(t, result.Kept)
						require.Equal(t, batch, result.Retry)
						require.Zero(t, result.GasUsed)
					} else {
						require.Empty(t, result.Retry)
						require.Equal(t, batch, result.Kept)
						require.Equal(t, uint64(777), result.GasUsed)
					}
					err = result.Unavailable
					wantCalls = 2
				}
				if code >= 400 {
					require.EqualError(t, err, "legacy RPC HTTP request failed")
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, int64(wantCalls), calls.Load())

				families, err := reg.Gather()
				require.NoError(t, err)
				var measuredCalls uint64
				var failures float64
				for _, family := range families {
					if family.GetName() != "akt_reporter_rpc_duration_seconds" && family.GetName() != "akt_reporter_rpc_errors_total" {
						continue
					}
					for _, metric := range family.Metric {
						labels := map[string]string{}
						for _, label := range metric.Label {
							labels[label.GetName()] = label.GetValue()
						}
						require.Len(t, labels, 2)
						require.Equal(t, "akash", labels["chain"])
						require.Contains(t, []string{"status", "/cosmos.tx.v1beta1.Service/Simulate"}, labels["method"])
						if family.GetName() == "akt_reporter_rpc_duration_seconds" {
							measuredCalls += metric.GetHistogram().GetSampleCount()
						} else {
							failures += metric.GetCounter().GetValue()
						}
					}
				}
				require.Equal(t, wantCalls, measuredCalls)
				if code >= 400 {
					require.Equal(t, float64(1), failures)
				} else {
					require.Zero(t, failures)
				}
			})
		}
	}
}
