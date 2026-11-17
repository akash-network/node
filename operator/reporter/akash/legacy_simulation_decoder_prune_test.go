package akash_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	rpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/status"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/akash"
	"pkg.akt.dev/node/v2/operator/reporter/metrics"
	"pkg.akt.dev/node/v2/operator/reporter/policy"
	reportertx "pkg.akt.dev/node/v2/operator/reporter/tx"
)

func TestLegacySimulationDecoderRetry(t *testing.T) {
	for _, body := range []string{
		`{"gas_info":{"gas_wanted":"10","gas_used":"7"},"gasInfo":null}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":"7","gasUsed":null}}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":"7"},"gas_info":{"gas_wanted":"10"}}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":"null"}}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":"7"},"result":{"log":"message index: 1","msg_responses":[{"@type":"/google.protobuf.UInt64Value","value":null}]}}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":"7"},"result":{"msg_responses":[{"@type":"/google.protobuf.Timestamp","value":null}]}}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":"7"},"result":{"msg_responses":[{"@type":"/google.protobuf.Duration","value":null}]}}`,
	} {
		t.Run(body, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request rpctypes.RPCRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				var result any
				if request.Method == "status" {
					result = &ctypes.ResultStatus{}
				} else {
					require.Equal(t, "abci_query", request.Method)
					value := body
					if calls.Add(1) == 2 {
						value = `{"gas_info":{"gas_wanted":"18446744073709551615","gas_used":"7"}}`
					}
					result = &ctypes.ResultABCIQuery{
						Response: abci.ResponseQuery{
							Height: 42,
							Value:  []byte(value),
						},
					}
				}
				data, err := json.Marshal(rpctypes.NewRPCSuccessResponse(request.ID, result))
				require.NoError(t, err)
				_, _ = w.Write(data)
			}))
			defer server.Close()
			reg := prometheus.NewRegistry()
			c, err := akash.Dial(t.Context(), akash.Options{
				ABCI:           true,
				Node:           server.URL,
				RequestTimeout: time.Second,
				Concurrency:    1,
				Metrics:        metrics.New(reg),
			})
			require.NoError(t, err)
			defer c.Close()
			batch := []policy.Planned{
				{
					Msg: &sinkv1.MsgReportOutcome{},
				},
				{
					Msg: &sinkv1.MsgReportOutcome{},
				},
			}
			simulate := func([]sdk.Msg) (uint64, error) {
				return c.Simulate(t.Context(), []byte{1})
			}
			first := reportertx.Prune(simulate, batch)
			require.Error(t, first.Unavailable)
			_, native := status.FromError(first.Unavailable)
			require.False(t, native)
			require.Equal(t, batch, first.Retry)
			require.Empty(t, first.Kept)
			require.Empty(t, first.Dropped)
			require.Empty(t, first.Deferred)
			require.Zero(t, first.GasUsed)
			require.EqualValues(t, 1, calls.Load(), "malformed response must stop pruning without probing")

			second := reportertx.Prune(simulate, first.Retry)
			require.Nil(t, second.Unavailable, "decoder errors must release the single permit")
			require.Equal(t, batch, second.Kept)
			require.Empty(t, second.Dropped)
			require.EqualValues(t, 7, second.GasUsed)
			require.EqualValues(t, 2, calls.Load())

			families, err := reg.Gather()
			require.NoError(t, err)
			var recorded uint64
			for _, family := range families {
				if family.GetName() != "akt_reporter_rpc_duration_seconds" {
					continue
				}
				for _, metric := range family.Metric {
					for _, label := range metric.Label {
						if label.GetName() == "method" && label.GetValue() == "/cosmos.tx.v1beta1.Service/Simulate" {
							recorded += metric.Histogram.GetSampleCount()
						}
					}
				}
			}
			require.EqualValues(t, 2, recorded, "one interceptor measurement per attempt, including decoder failures")
		})
	}
}
