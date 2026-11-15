package native

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/p2p"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	cmttypes "github.com/cometbft/cometbft/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sink "pkg.akt.dev/go/node/sink/v1"
)

type request struct {
	ID     json.RawMessage            `json:"id"`
	Method string                     `json:"method"`
	Params map[string]json.RawMessage `json:"params"`
}

func stringParam(t *testing.T, r request, name string) string {
	t.Helper()
	var value string
	require.NoError(t, json.Unmarshal(r.Params[name], &value))
	return value
}

func dataParam(t *testing.T, r request) []byte {
	t.Helper()
	value := stringParam(t, r, "data")
	raw, err := hex.DecodeString(strings.TrimPrefix(value, "0x"))
	require.NoError(t, err)
	return raw
}

func mockClient(t *testing.T, handle func(request) (any, error)) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var input request
		require.NoError(t, json.Unmarshal(raw, &input))
		result, failure := handle(input)
		body := map[string]any{
			"jsonrpc": "2.0",
			"id":      input.ID,
		}
		if failure != nil {
			body["error"] = map[string]any{
				"code":    -32603,
				"message": failure.Error(),
			}
		} else {
			raw, err := cmtjson.Marshal(result)
			require.NoError(t, err)
			body["result"] = json.RawMessage(raw)
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(body))
	}))
	t.Cleanup(server.Close)
	client, err := NewWithHTTP(server.URL, "test-1", 60*time.Second, &http.Client{Timeout: 2 * time.Second})
	require.NoError(t, err)
	client.now = func() time.Time { return time.Unix(1000, 500000000) }
	return client
}

func transfer(sequence uint64) sink.TransferState {
	return sink.TransferState{
		ID: sink.ID{
			Owner:    sdk.AccAddress([]byte{42}).String(),
			Height:   9,
			Sequence: sequence,
		},
		Data: sink.RecordData{
			Dest:   "11111111111111111111111111111111",
			Amount: sdk.NewInt64Coin("uakt", 123),
		},
		Settlement: sink.SettlementData{
			Round: 1,
			Policy: sink.ReporterPolicy{
				Version:   7,
				Threshold: sdkmath.LegacyOneDec(),
			},
		},
		Status: sink.StatusTransferPending,
	}
}

func response(t *testing.T, message proto.Message, height int64) *ctypes.ResultABCIQuery {
	t.Helper()
	raw, err := proto.Marshal(message)
	require.NoError(t, err)
	return &ctypes.ResultABCIQuery{
		Response: abci.ResponseQuery{
			Height: height,
			Value:  raw,
		},
	}
}

func TestTransferErrorsAndHeight(t *testing.T) {
	for _, test := range []struct {
		name, space string
		code        uint32
		expected    error
	}{
		{
			name:     "missing",
			space:    "sdk",
			code:     22,
			expected: ErrNotFound,
		},
		{
			name:     "pruned",
			space:    "sdk",
			code:     38,
			expected: ErrPruned,
		},
		{
			name:  "foreign-notfound",
			space: "sink",
			code:  22,
		},
		{
			name:  "foreign-pruned",
			space: "sink",
			code:  38,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := mockClient(t, func(r request) (any, error) {
				require.Equal(t, "abci_query", r.Method)
				require.Equal(t, "10", stringParam(t, r, "height"))
				var request sink.QueryTransferRequest
				require.NoError(t, request.Unmarshal(dataParam(t, r)))
				require.Equal(t, transfer(1).ID, request.ID)
				return &ctypes.ResultABCIQuery{
					Response: abci.ResponseQuery{
						Code:      test.code,
						Codespace: test.space,
						Log:       "exact detail",
						Height:    10,
					},
				}, nil
			})
			_, err := client.Transfer(context.Background(), transfer(1).ID, 10)
			var failure *QueryError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, test.code, failure.Code)
			require.Equal(t, test.space, failure.Codespace)
			require.Equal(t, "exact detail", failure.Log)
			if test.expected != nil {
				require.ErrorIs(t, err, test.expected)
			} else {
				require.NotErrorIs(t, err, ErrNotFound)
				require.NotErrorIs(t, err, ErrPruned)
			}
		})
	}
	for _, test := range []struct {
		name   string
		height int64
		value  sink.TransferState
		raw    []byte
		outage bool
	}{
		{
			name:   "valid",
			height: 10,
			value:  transfer(1),
		},
		{
			name:   "wrong-height",
			height: 11,
			value:  transfer(1),
		},
		{
			name:   "wrong-ID",
			height: 10,
			value:  transfer(2),
		},
		{
			name:   "missing-transfer",
			height: 10,
			raw:    []byte{},
		},
		{
			name:   "outage",
			outage: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := mockClient(t, func(r request) (any, error) {
				if test.outage {
					return nil, errors.New("transport unavailable")
				}
				out := response(t, &sink.QueryTransferResponse{Transfer: test.value}, test.height)
				if test.raw != nil {
					out.Response.Value = test.raw
				}
				return out, nil
			})
			got, err := client.Transfer(context.Background(), transfer(1).ID, 10)
			if test.name == "valid" {
				require.NoError(t, err)
				require.Equal(t, test.value, got)
			} else {
				require.Error(t, err)
				require.NotErrorIs(t, err, ErrNotFound)
			}
		})
	}
}

func TestSnapshotCommittedHeight(t *testing.T) {
	var mu sync.Mutex
	paths := make(map[string]int)
	client := mockClient(t, func(r request) (any, error) {
		switch r.Method {
		case "status":
			return &ctypes.ResultStatus{
				NodeInfo: p2p.DefaultNodeInfo{
					Network: "test-1",
				},
				SyncInfo: ctypes.SyncInfo{
					LatestBlockHeight: 12,
					LatestBlockTime:   time.Unix(999, 0),
				},
			}, nil
		case "abci_info":
			return &ctypes.ResultABCIInfo{
				Response: abci.ResponseInfo{
					LastBlockHeight: 11,
				},
			}, nil
		case "block":
			require.Equal(t, "11", stringParam(t, r, "height"))
			return &ctypes.ResultBlock{
				Block: &cmttypes.Block{
					Header: cmttypes.Header{
						Height:  11,
						ChainID: "test-1",
						Time:    time.Unix(998, 900000000),
					},
				},
			}, nil
		case "abci_query":
			require.Equal(t, "11", stringParam(t, r, "height"))
			path := stringParam(t, r, "path")
			mu.Lock()
			paths[path]++
			mu.Unlock()
			switch path {
			case "/akash.sink.v1.Query/State":
				return response(t, &sink.QueryStateResponse{
					Epoch: 2,
					SendUsage: sink.EpochUsage{
						Epoch:  2,
						Amount: sdk.NewInt64Coin("uakt", 3),
					},
					RefundUsage: sink.EpochUsage{
						Epoch:  2,
						Amount: sdk.NewInt64Coin("uakt", 5),
					},
				}, 11), nil
			case "/akash.sink.v1.Query/Params":
				return response(t, &sink.QueryParamsResponse{}, 11), nil
			case "/akash.sink.v1.Query/Reporters":
				return response(t, &sink.QueryReportersResponse{
					Reporters: []sink.ReporterPolicy{transfer(1).Settlement.Policy},
				}, 11), nil
			}
		}
		return nil, fmt.Errorf("unexpected RPC %s", r.Method)
	})
	got, err := client.Snapshot(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(11), got.Height)
	require.Equal(t, int64(998), got.Time.Unix())
	require.Equal(t, uint64(7), got.Policy.Version)
	require.Len(t, paths, 3)
	for _, n := range paths {
		require.Equal(t, 1, n)
	}
}

func TestSnapshotRejectsUnfreshStatus(t *testing.T) {
	for _, test := range []struct {
		name, chain string
		catching    bool
		clock       int64
	}{
		{
			name:  "wrong-chain",
			chain: "other",
			clock: 999,
		},
		{
			name:     "syncing",
			chain:    "test-1",
			catching: true,
			clock:    999,
		},
		{
			name:  "stale",
			chain: "test-1",
			clock: 900,
		},
		{
			name:  "future",
			chain: "test-1",
			clock: 1011,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := mockClient(t, func(r request) (any, error) {
				require.Equal(t, "status", r.Method)
				return &ctypes.ResultStatus{
					NodeInfo: p2p.DefaultNodeInfo{
						Network: test.chain,
					},
					SyncInfo: ctypes.SyncInfo{
						CatchingUp:        test.catching,
						LatestBlockHeight: 12,
						LatestBlockTime:   time.Unix(test.clock, 0),
					},
				}, nil
			})
			_, err := client.Snapshot(context.Background())
			require.Error(t, err)
		})
	}
}

func TestOpenPriorityAndPagination(t *testing.T) {
	held, refund, voided, pending := transfer(1), transfer(2), transfer(3), transfer(4)
	held.Settlement.Hold = sink.Hold(1)
	refund.Status = sink.StatusRefundPending
	voided.Status = sink.StatusVoided
	var visited []sink.Status
	client := mockClient(t, func(r request) (any, error) {
		require.Equal(t, "10", stringParam(t, r, "height"))
		path := stringParam(t, r, "path")
		if path == "/akash.sink.v1.Query/Disputes" {
			return response(t, &sink.QueryDisputesResponse{
				Transfers:  []sink.TransferState{held},
				Pagination: &query.PageResponse{},
			}, 10), nil
		}
		var input sink.QueryTransfersRequest
		require.NoError(t, input.Unmarshal(dataParam(t, r)))
		visited = append(visited, input.Status)
		values := map[sink.Status][]sink.TransferState{
			sink.StatusRefundPending:   {refund},
			sink.StatusVoided:          {voided},
			sink.StatusTransferPending: {held, pending},
		}
		return response(t, &sink.QueryTransfersResponse{
			Transfers:  values[input.Status],
			Pagination: &query.PageResponse{},
		}, 10), nil
	})
	got, err := client.Open(context.Background(), 10, nil)
	require.NoError(t, err)
	require.Equal(t, []sink.TransferState{held, refund, voided, pending}, got)
	require.Equal(t, []sink.Status{sink.StatusRefundPending, sink.StatusVoided, sink.StatusTransferPending}, visited)
	for _, mode := range []string{"duplicate", "loop", "empty", "filter", "hold", "missing-pagination", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			client := mockClient(t, func(r request) (any, error) {
				calls++
				values := []sink.TransferState{held}
				pagination := &query.PageResponse{NextKey: []byte("opaque")}
				switch mode {
				case "duplicate":
					values = append(values, held)
				case "loop":
					values[0].ID.Sequence = uint64(calls)
				case "empty":
					values = nil
				case "filter":
					values[0].Status = sink.StatusMinted
				case "hold":
					values[0].Settlement.Hold = sink.HoldNone
				case "missing-pagination":
					pagination = nil
				case "oversized":
					values = make([]sink.TransferState, 501)
				}
				if mode == "filter" {
					return response(t, &sink.QueryTransfersResponse{
						Transfers:  values,
						Pagination: pagination,
					}, 10), nil
				}
				return response(t, &sink.QueryDisputesResponse{
					Transfers:  values,
					Pagination: pagination,
				}, 10), nil
			})
			_, err := client.collect(context.Background(), 10, sink.StatusTransferPending, mode != "filter", nil)
			require.Error(t, err)
			assert.LessOrEqual(t, calls, 2)
		})
	}
}

func TestHistoryCheckpoints(t *testing.T) {
	for _, test := range []struct {
		name     string
		after    *Cursor
		ids      []uint64
		next     string
		expected []uint64
		kind     string
		fail     bool
	}{
		{
			name:     "initial",
			ids:      []uint64{1, 2},
			next:     "opaque",
			expected: []uint64{1, 2},
			kind:     "next",
		},
		{
			name: "next",
			after: &Cursor{
				Kind:   "next",
				Key:    []byte("opaque"),
				LastID: transfer(2).ID,
			},
			ids:      []uint64{3},
			expected: []uint64{3},
			kind:     "tail",
		},
		{
			name: "tail",
			after: &Cursor{
				Kind:   "tail",
				Key:    []byte("opaque"),
				LastID: transfer(2).ID,
			},
			ids:      []uint64{1, 2, 3},
			expected: []uint64{3},
			kind:     "tail",
		},
		{
			name: "tail-no-new",
			after: &Cursor{
				Kind:   "tail",
				LastID: transfer(2).ID,
			},
			ids:  []uint64{1, 2},
			kind: "tail",
		},
		{
			name: "missing-tail",
			after: &Cursor{
				Kind:   "tail",
				LastID: transfer(2).ID,
			},
			ids:  []uint64{1, 3},
			fail: true,
		},
		{
			name: "missing-next",
			after: &Cursor{
				Kind:   "next",
				LastID: transfer(2).ID,
			},
			fail: true,
		},
		{
			name: "empty-tail",
			after: &Cursor{
				Kind:   "tail",
				LastID: transfer(2).ID,
			},
			fail: true,
		},
		{
			name: "duplicate",
			ids:  []uint64{1, 1},
			fail: true,
		},
		{
			name: "reordered",
			ids:  []uint64{2, 1},
			fail: true,
		},
		{
			name: "backwards",
			after: &Cursor{
				Kind:   "next",
				LastID: transfer(2).ID,
			},
			ids:  []uint64{1},
			fail: true,
		},
		{
			name: "stuck",
			after: &Cursor{
				Kind:   "next",
				Key:    []byte("opaque"),
				LastID: transfer(2).ID,
			},
			ids:  []uint64{3},
			next: "opaque",
			fail: true,
		},
		{
			name: "empty-progress",
			next: "opaque",
			fail: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := mockClient(t, func(r request) (any, error) {
				var input sink.QueryTransfersRequest
				require.NoError(t, input.Unmarshal(dataParam(t, r)))
				if test.after != nil {
					require.Equal(t, test.after.Key, input.Pagination.Key)
				}
				values := make([]sink.TransferState, len(test.ids))
				for i, id := range test.ids {
					values[i] = transfer(id)
				}
				return response(t, &sink.QueryTransfersResponse{
					Transfers: values,
					Pagination: &query.PageResponse{
						NextKey: []byte(test.next),
					},
				}, 10), nil
			})
			got, cursor, more, err := client.History(context.Background(), 10, test.after)
			if test.fail {
				require.Error(t, err)
				require.Nil(t, cursor)
				require.False(t, more)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.kind, cursor.Kind)
			require.Equal(t, test.next != "", more)
			var ids []uint64
			for _, value := range got {
				ids = append(ids, value.ID.Sequence)
			}
			require.Equal(t, test.expected, ids)
		})
	}
}

func TestHeaderSource(t *testing.T) {
	var heights []int64
	client := mockClient(t, func(r request) (any, error) {
		height, err := strconv.ParseInt(stringParam(t, r, "height"), 10, 64)
		require.NoError(t, err)
		if r.Method == "commit" {
			return &ctypes.ResultCommit{
				SignedHeader: cmttypes.SignedHeader{
					Header: &cmttypes.Header{
						Height:  20,
						ChainID: "test-1",
					},
					Commit: &cmttypes.Commit{
						Height: 20,
					},
				},
				CanonicalCommit: true,
			}, nil
		}
		require.Equal(t, "validators", r.Method)
		heights = append(heights, height)
		return &ctypes.ResultValidators{
			BlockHeight: height,
			Validators:  []*cmttypes.Validator{},
			Count:       0,
			Total:       0,
		}, nil
	})
	got, err := client.HeaderSource(context.Background(), 10, 20, 7)
	require.NoError(t, err)
	require.Equal(t, []int64{20, 11}, heights)
	require.Equal(t, uint64(10), got.TrustedHeight)
	require.Equal(t, uint64(7), got.Revision)
}

func TestValidatorPaginationFailures(t *testing.T) {
	for _, mode := range []string{"wrong-height", "wrong-count", "exhausted", "multi-page"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			client := mockClient(t, func(r request) (any, error) {
				calls++
				page, err := strconv.Atoi(stringParam(t, r, "page"))
				require.NoError(t, err)
				require.Equal(t, calls, page)
				result := &ctypes.ResultValidators{
					BlockHeight: 10,
					Validators: []*cmttypes.Validator{
						{
							VotingPower: 1,
						},
					},
					Count: 1,
					Total: 2,
				}
				switch mode {
				case "wrong-height":
					result.BlockHeight = 9
				case "wrong-count":
					result.Count = 2
				case "exhausted":
					result.Total = 11
				}
				return result, nil
			})
			result, err := client.validators(context.Background(), 10)
			if mode == "multi-page" {
				require.NoError(t, err)
				require.Len(t, result, 2)
				require.Equal(t, 2, calls)
			} else {
				require.Error(t, err)
			}
			if mode == "exhausted" {
				require.Equal(t, 10, calls)
			}
		})
	}
}
