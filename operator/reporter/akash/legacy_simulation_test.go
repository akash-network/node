package akash

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	rpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"
	grpctypes "github.com/cosmos/cosmos-sdk/types/grpc"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"pkg.akt.dev/node/v2/operator/reporter/metrics"
)

const legacySimulationMethod = "/cosmos.tx.v1beta1.Service/Simulate"
const legacySimulationJSON = `{"gas_info":{"gas_wanted":"18446744073709551615","gas_used":"75984"},"result":null}`

func TestLegacySimulationWireAndMetrics(t *testing.T) {
	input := []byte{0, 1, 255, 0, 42}
	var calls atomic.Int64
	c := mockABCI(t, func(_ *http.Request, wire abciWireRequest) abci.ResponseQuery {
		calls.Add(1)
		require.Equal(t, "/app/simulate", wire.Path)
		require.Equal(t, input, []byte(wire.Data))
		require.Zero(t, wire.Height)
		require.False(t, wire.Prove)
		return abci.ResponseQuery{
			Codespace: "sdk",
			Height:    42,
			Value:     []byte(legacySimulationJSON),
		}
	})
	reg := prometheus.NewRegistry()
	c.metrics = metrics.New(reg)
	var header metadata.MD
	res, err := c.txs.Simulate(t.Context(), &txtypes.SimulateRequest{TxBytes: input}, grpc.Header(&header))
	require.NoError(t, err)
	require.Equal(t, uint64(75984), res.GasInfo.GasUsed)
	require.Equal(t, ^uint64(0), res.GasInfo.GasWanted)
	require.Equal(t, []string{"42"}, header.Get(grpctypes.GRPCBlockHeightHeader))
	used, err := c.Simulate(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, uint64(75984), used)
	require.Equal(t, int64(2), calls.Load())
	families, err := reg.Gather()
	require.NoError(t, err)
	var recorded uint64
	for _, family := range families {
		if family.GetName() == "akt_reporter_rpc_duration_seconds" {
			for _, metric := range family.Metric {
				for _, label := range metric.Label {
					if label.GetName() == "method" {
						require.Equal(t, legacySimulationMethod, label.GetValue())
					}
				}
				recorded += metric.Histogram.GetSampleCount()
			}
		}
	}
	require.EqualValues(t, 2, recorded, "one interceptor measurement per simulation")
}

func TestLegacySimulationResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		height int64
		code   uint32
		space  string
		log    string
		native bool
		gas    uint64
		valid  bool
	}{
		{
			name:   "native success",
			body:   legacySimulationJSON,
			height: 42,
			valid:  true,
			gas:    75984,
		},
		{
			name:   "explicit zero gas",
			body:   `{"gas_info":{"gas_wanted":"0","gas_used":"0"}}`,
			height: 42,
			valid:  true,
		},
		{
			name:   "numeric protobuf JSON",
			body:   `{"gas_info":{"gas_wanted":10,"gas_used":7}}`,
			height: 42,
			valid:  true,
			gas:    7,
		},
		{
			name:   "indexed failure before zero height",
			code:   24,
			space:  "sink",
			log:    "failed to simulate tx: failed to execute message; message index: 1: stale reporter policy or round",
			native: true,
		},
		{
			name:   "unindexed block cap",
			code:   41,
			space:  "sdk",
			log:    "failed to simulate tx: tx gas limit exceeds block max gas",
			native: true,
		},
		{
			name:   "sdk6 is still a native answer not a fallback trigger",
			code:   6,
			space:  "sdk",
			log:    "failed to simulate tx: native rejection",
			native: true,
		},
		{
			name:  "missing error log",
			code:  24,
			space: "sink",
		},
		{
			name: "missing error codespace",
			code: 24,
			log:  "message index: 0",
		},
		{
			name:   "missing error code cannot become success",
			body:   legacySimulationJSON,
			height: 42,
			space:  "sink",
			log:    "message index: 0",
		},
		{
			name:  "contradictory failure value",
			body:  legacySimulationJSON,
			code:  24,
			space: "sink",
			log:   "message index: 0",
		},
		{
			name: "success zero height",
			body: legacySimulationJSON,
		},
		{
			name:   "success negative height",
			body:   legacySimulationJSON,
			height: -1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			c := mockABCI(t, func(_ *http.Request, wire abciWireRequest) abci.ResponseQuery {
				calls.Add(1)
				require.Equal(t, "/app/simulate", wire.Path)
				return abci.ResponseQuery{
					Value:     []byte(tc.body),
					Height:    tc.height,
					Code:      tc.code,
					Codespace: tc.space,
					Log:       tc.log,
				}
			})
			used, err := c.Simulate(t.Context(), []byte{1})
			require.EqualValues(t, 1, calls.Load(), "never probe or fall back")
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, tc.gas, used)
				return
			}
			require.Zero(t, used)
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.Equal(t, tc.native, ok, "only valid native failures are deterministic")
			if tc.native {
				require.Equal(t, codes.Unknown, st.Code())
				require.Contains(t, st.Message(), tc.log)
				require.Contains(t, st.Message(), tc.space+"/"+strconv.FormatUint(uint64(tc.code), 10))
				require.NotContains(t, st.Message(), "with gas used:")
			}
		})
	}
}

func TestLegacySimulationMalformedJSON(t *testing.T) {
	for _, body := range []string{
		``, `null`, `{}`, `[]`, `"text"`, `{`,
		`{"gas_info":null}`, `{"gas_info":{}}`,
		`{"gas_info":{"gas_wanted":"10"}}`,
		`{"gas_info":{"gas_used":"7"}}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":null}}`,
		`{"gas_info":{"gas_wanted":null,"gas_used":"7"}}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":true}}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":"-1"}}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":"18446744073709551616"}}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":"1.5"}}`,
		`{"gas_info":{"gas_wanted":{},"gas_used":"1"}}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":[]}}`,
		`{"gas_info":"wrong"}`, `{"gas_info":[]}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":"7","unknown":1}}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":"7"},"result":"wrong"}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":"7"},"unexpected":true}`,
		`{"gas_info":{"gas_wanted":"10","gas_used":"7"},"result":{"msg_responses":[{"@type":"/unknown.Response"}]}}`,
		legacySimulationJSON + `{}`,
	} {
		t.Run(body, func(t *testing.T) {
			c := mockABCI(t, func(_ *http.Request, _ abciWireRequest) abci.ResponseQuery {
				return abci.ResponseQuery{
					Height: 42,
					Value:  []byte(body),
				}
			})
			used, err := c.Simulate(t.Context(), []byte{1})
			require.Zero(t, used)
			require.Error(t, err)
			_, native := status.FromError(err)
			require.False(t, native, "bad JSON must not drop a batch as a native failure")
		})
	}
}

func TestLegacySimulationLatestOnlyAndTypes(t *testing.T) {
	var calls atomic.Int64
	c := mockABCI(t, func(_ *http.Request, _ abciWireRequest) abci.ResponseQuery {
		calls.Add(1)
		return abci.ResponseQuery{}
	})
	for _, heights := range [][]string{{"1"}, {"42"}, {"142"}, {"0"}, {"-1"}, {"bad"}, {"42", "42"}, {}} {
		ctx := metadata.NewOutgoingContext(t.Context(), metadata.MD{grpctypes.GRPCBlockHeightHeader: heights})
		_, err := c.Simulate(ctx, []byte{1})
		require.ErrorContains(t, err, "latest-state")
	}
	for _, req := range []any{nil, (*txtypes.SimulateRequest)(nil), "wrong", &txtypes.SimulateRequest{}, &txtypes.SimulateRequest{
		Tx:      &txtypes.Tx{},
		TxBytes: []byte{1},
	}} {
		err := c.queries.Invoke(t.Context(), legacySimulationMethod, req, &txtypes.SimulateResponse{})
		require.Error(t, err)
	}
	for _, reply := range []any{nil, (*txtypes.SimulateResponse)(nil), "wrong"} {
		err := c.queries.Invoke(t.Context(), legacySimulationMethod, &txtypes.SimulateRequest{TxBytes: []byte{1}}, reply)
		require.Error(t, err)
	}
	require.Zero(t, calls.Load())
}

func TestLegacySimulationCancellationAndQueue(t *testing.T) {
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	c := mockABCI(t, func(r *http.Request, _ abciWireRequest) abci.ResponseQuery {
		started <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-release:
		}
		return abci.ResponseQuery{Height: 42}
	})
	t.Cleanup(func() { close(release) })
	c.timeout = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := c.Simulate(ctx, []byte{1})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("simulation did not start")
	}
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	_, err := c.Simulate(t.Context(), []byte{1})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	<-started

	require.NoError(t, c.reads.Acquire(t.Context(), 2))
	queue, stop := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer stop()
	_, err = c.Simulate(queue, []byte{1})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	stopping, stopNow := context.WithCancel(t.Context())
	stopNow()
	_, err = c.Simulate(WithStop(t.Context(), stopping), []byte{1})
	require.ErrorIs(t, err, context.Canceled)
	c.reads.Release(2)
	select {
	case <-started:
		t.Fatal("a queued request reached HTTP")
	default:
	}
	require.NoError(t, c.reads.Acquire(t.Context(), 2), "permits released after failure")
	c.reads.Release(2)
}

func TestLegacySimulationBounds(t *testing.T) {
	// Marshal once, outside the request deadline: under -race this takes seconds.
	raw, err := cmtjson.Marshal(&ctypes.ResultABCIQuery{Response: abci.ResponseQuery{
		Height: 42,
		Value:  []byte(strings.Repeat(" ", maxRecvBytes+1)),
	}})
	require.NoError(t, err)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpctypes.RPCRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		if req.Method == "status" {
			data, err := json.Marshal(rpctypes.NewRPCSuccessResponse(req.ID, &ctypes.ResultStatus{}))
			require.NoError(t, err)
			_, _ = w.Write(data)
			return
		}
		calls.Add(1)
		id, err := json.Marshal(req.ID)
		require.NoError(t, err)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(id) + `,"result":`))
		_, _ = w.Write(raw)
		_, _ = w.Write([]byte(`}`))
	}))
	defer server.Close()
	c, err := Dial(t.Context(), Options{ABCI: true, Node: server.URL, RequestTimeout: time.Second})
	require.NoError(t, err)
	defer func() { require.NoError(t, c.Close()) }()
	_, err = c.Simulate(t.Context(), make([]byte, maxRecvBytes+1))
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.Zero(t, calls.Load())
	c.timeout = 10 * time.Second
	_, err = c.Simulate(t.Context(), []byte{1})
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.EqualValues(t, 1, calls.Load())
}

func TestLegacySimulationMalformedEnvelopeAndPrivacy(t *testing.T) {
	for _, mode := range []string{"malformed", "missing result", "missing response", "wrong code", "wrong log", "transport", "oversize envelope"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req rpctypes.RPCRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				if req.Method == "status" {
					data, err := json.Marshal(rpctypes.NewRPCSuccessResponse(req.ID, &ctypes.ResultStatus{}))
					require.NoError(t, err)
					_, _ = w.Write(data)
					return
				}
				id, err := json.Marshal(req.ID)
				require.NoError(t, err)
				switch mode {
				case "transport":
					conn, _, err := w.(http.Hijacker).Hijack()
					require.NoError(t, err)
					require.NoError(t, conn.Close())
				case "oversize envelope":
					w.Header().Set("Content-Length", strconv.Itoa(maxABCIEnvelope+1))
					w.WriteHeader(200)
				case "malformed":
					_, _ = w.Write([]byte(`{`))
				case "missing result":
					_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(id) + `}`))
				default:
					result := `{}`
					if mode == "wrong code" {
						result = `{"response":{"code":"not-a-code","log":"message index: 0"}}`
					} else if mode == "wrong log" {
						result = `{"response":{"code":24,"codespace":"sink","log":{}}}`
					}
					_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(id) + `,"result":` + result + `}`))
				}
			}))
			defer server.Close()
			c, err := Dial(t.Context(), Options{
				ABCI:           true,
				Node:           server.URL + "/SECRETpath?token=SECRETquery",
				RequestTimeout: time.Second,
			})
			require.NoError(t, err)
			defer c.Close()
			used, err := c.Simulate(t.Context(), []byte{1})
			require.Error(t, err)
			require.Zero(t, used)
			require.NotContains(t, err.Error(), "SECRET")
			_, native := status.FromError(err)
			require.False(t, native)
			require.False(t, errors.Is(err, context.Canceled))
		})
	}
}
