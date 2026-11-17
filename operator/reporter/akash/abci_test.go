package akash

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	cmtbytes "github.com/cometbft/cometbft/libs/bytes"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/p2p"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	rpctypes "github.com/cometbft/cometbft/rpc/jsonrpc/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	grpctypes "github.com/cosmos/cosmos-sdk/types/grpc"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	consensustypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

type abciWireRequest struct {
	Path   string            `json:"path"`
	Data   cmtbytes.HexBytes `json:"data"`
	Height int64             `json:"height"`
	Prove  bool              `json:"prove"`
}

// mockABCI uses real Comet JSON and generated protobufs; it is not a native chain.
func mockABCI(t *testing.T, query func(*http.Request, abciWireRequest) abci.ResponseQuery) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/native%2Frpc?project=R176&n=1", r.RequestURI)
		var request rpctypes.RPCRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		var result any
		switch request.Method {
		case "status":
			result = &ctypes.ResultStatus{
				NodeInfo: p2p.DefaultNodeInfo{
					DefaultNodeID: "r176-node",
					Network:       "r176-test",
					Other: p2p.DefaultNodeInfoOther{
						TxIndex: "on",
					},
				},
				SyncInfo: ctypes.SyncInfo{
					LatestBlockHeight: 42,
					LatestBlockTime:   time.Now(),
				},
			}
		case "abci_query":
			var wire abciWireRequest
			require.NoError(t, cmtjson.Unmarshal(request.Params, &wire))
			result = &ctypes.ResultABCIQuery{Response: query(r, wire)}
		default:
			t.Errorf("unexpected method %s", request.Method)
			w.WriteHeader(500)
			return
		}

		data, err := json.Marshal(rpctypes.NewRPCSuccessResponse(request.ID, result))
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)
	c, err := Dial(t.Context(), Options{
		ABCI:           true,
		Node:           server.URL + "/native%2Frpc?project=R176&n=1",
		ChainID:        "r176-test",
		MaxAge:         time.Minute,
		RequestTimeout: time.Second,
		Concurrency:    2,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, c.Close())
	})
	require.True(t, c.TxIndexEnabled())
	return c
}

func TestABCIGeneratedQueries(t *testing.T) {
	id := sinkv1.ID{
		Owner:    "akash1fixture",
		Height:   17,
		Sequence: 9,
	}

	page := &sdkquery.PageRequest{
		Key:   []byte{0, 1, 255},
		Limit: 97,
	}

	pageResult := &sdkquery.PageResponse{
		NextKey: []byte{9, 8, 0},
		Total:   101,
	}

	requests := map[string]proto.Message{
		pathState:     &sinkv1.QueryStateRequest{},
		pathParams:    &sinkv1.QueryParamsRequest{},
		pathReporters: &sinkv1.QueryReportersRequest{Version: 7},
		pathTransfer:  &sinkv1.QueryTransferRequest{ID: id},
		"/akash.sink.v1.Query/Transfers": &sinkv1.QueryTransfersRequest{
			Status:     sinkv1.StatusTransferPending,
			Pagination: page,
		},
		"/akash.sink.v1.Query/Disputes": &sinkv1.QueryDisputesRequest{Pagination: page},
	}

	responses := map[string]proto.Message{
		pathState:  &sinkv1.QueryStateResponse{Epoch: 29},
		pathParams: &sinkv1.QueryParamsResponse{Params: sinkv1.DefaultParams()},
		pathReporters: &sinkv1.QueryReportersResponse{
			Reporters: []sinkv1.ReporterPolicy{
				{Version: 7},
			},
		},
		pathTransfer:                     &sinkv1.QueryTransferResponse{},
		"/akash.sink.v1.Query/Transfers": &sinkv1.QueryTransfersResponse{Pagination: pageResult},
		"/akash.sink.v1.Query/Disputes":  &sinkv1.QueryDisputesResponse{Pagination: pageResult},
	}

	var calls atomic.Int64
	c := mockABCI(t, func(_ *http.Request, wire abciWireRequest) abci.ResponseQuery {
		calls.Add(1)
		require.Equal(t, int64(42), wire.Height)
		require.False(t, wire.Prove)
		expected, ok := requests[wire.Path]
		require.True(t, ok, "unexpected generated query route")
		decoded := reflect.New(reflect.TypeOf(expected).Elem()).Interface().(proto.Message)
		require.NoError(t, proto.Unmarshal(wire.Data, decoded))
		require.True(t, proto.Equal(expected, decoded), "query protobuf changed")
		data, err := proto.Marshal(responses[wire.Path])
		require.NoError(t, err)
		return abci.ResponseQuery{
			Height: 42,
			Value:  data,
		}
	})
	call := func(path string, f func(context.Context, ...grpc.CallOption) (proto.Message, error)) {
		t.Helper()
		var header metadata.MD
		ctx := metadata.AppendToOutgoingContext(t.Context(), grpctypes.GRPCBlockHeightHeader, "42")
		result, err := f(ctx, grpc.Header(&header))
		require.NoError(t, err)
		require.Equal(t, []string{"42"}, header.Get(grpctypes.GRPCBlockHeightHeader))
		expectedWire, err := proto.Marshal(responses[path])
		require.NoError(t, err)
		actualWire, err := proto.Marshal(result)
		require.NoError(t, err)
		require.True(t, bytes.Equal(expectedWire, actualWire), "response protobuf changed")
	}

	call(pathState, func(ctx context.Context, o ...grpc.CallOption) (proto.Message, error) {
		return c.sink.State(ctx, requests[pathState].(*sinkv1.QueryStateRequest), o...)
	})
	call(pathParams, func(ctx context.Context, o ...grpc.CallOption) (proto.Message, error) {
		return c.sink.Params(ctx, requests[pathParams].(*sinkv1.QueryParamsRequest), o...)
	})
	call(pathReporters, func(ctx context.Context, o ...grpc.CallOption) (proto.Message, error) {
		return c.sink.Reporters(ctx, requests[pathReporters].(*sinkv1.QueryReportersRequest), o...)
	})
	call(pathTransfer, func(ctx context.Context, o ...grpc.CallOption) (proto.Message, error) {
		return c.sink.Transfer(ctx, requests[pathTransfer].(*sinkv1.QueryTransferRequest), o...)
	})
	call("/akash.sink.v1.Query/Transfers", func(ctx context.Context, o ...grpc.CallOption) (proto.Message, error) {
		return c.sink.Transfers(ctx, requests["/akash.sink.v1.Query/Transfers"].(*sinkv1.QueryTransfersRequest), o...)
	})
	call("/akash.sink.v1.Query/Disputes", func(ctx context.Context, o ...grpc.CallOption) (proto.Message, error) {
		return c.sink.Disputes(ctx, requests["/akash.sink.v1.Query/Disputes"].(*sinkv1.QueryDisputesRequest), o...)
	})
	require.Equal(t, int64(6), calls.Load())
}

func TestABCIAccountConsensusAndSimulation(t *testing.T) {
	address := sdk.AccAddress(bytes.Repeat([]byte{7}, 20))
	account := authtypes.NewBaseAccount(address, nil, 73, 0)
	accountAny, err := codectypes.NewAnyWithValue(account)
	require.NoError(t, err)
	timeout := time.Unix(200, 0)
	body, err := proto.Marshal(&txtypes.TxBody{
		Unordered:        true,
		TimeoutTimestamp: &timeout,
	})
	require.NoError(t, err)
	raw, err := proto.Marshal(&txtypes.TxRaw{
		BodyBytes:     body,
		AuthInfoBytes: []byte{18, 0},
		Signatures:    [][]byte{make([]byte, 64)},
	})
	require.NoError(t, err)
	var simulationFailure atomic.Bool
	var calls atomic.Int64
	c := mockABCI(t, func(_ *http.Request, wire abciWireRequest) abci.ResponseQuery {
		calls.Add(1)
		require.Zero(t, wire.Height, "latest query request height")
		var response proto.Message
		switch wire.Path {
		case "/cosmos.auth.v1beta1.Query/Account":
			var request authtypes.QueryAccountRequest
			require.NoError(t, proto.Unmarshal(wire.Data, &request))
			require.Equal(t, address.String(), request.Address)
			response = &authtypes.QueryAccountResponse{Account: accountAny}
		case "/cosmos.consensus.v1.Query/Params":
			response = &consensustypes.QueryParamsResponse{
				Params: &cmtproto.ConsensusParams{
					Block: &cmtproto.BlockParams{
						MaxBytes: 1000000,
						MaxGas:   1300000,
					},
				},
			}
		case "/app/simulate":
			require.True(t, bytes.Equal(raw, wire.Data), "simulation tx bytes changed")
			if simulationFailure.Load() {
				return abci.ResponseQuery{
					Code:      24,
					Codespace: "sink",
					Log:       "failed to simulate tx: failed to execute message; message index: 0: stale reporter policy or round",
				}
			}

			return abci.ResponseQuery{
				Height: 42,
				Value:  []byte(`{"gas_info":{"gas_wanted":"18446744073709551615","gas_used":"143201"}}`),
			}
		default:
			t.Fatalf("unexpected generated route %s", wire.Path)
		}

		data, err := proto.Marshal(response)
		require.NoError(t, err)
		return abci.ResponseQuery{
			Height: 42,
			Value:  data,
		}
	})
	number, err := c.AccountNumber(t.Context(), address)
	require.NoError(t, err)
	require.Equal(t, uint64(73), number)
	gas, err := c.BlockMaxGas(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(1300000), gas)
	used, err := c.Simulate(t.Context(), raw)
	require.NoError(t, err)
	require.Equal(t, uint64(143201), used)
	simulationFailure.Store(true)
	_, err = c.Simulate(t.Context(), raw)
	require.Equal(t, codes.Unknown, status.Code(err))
	require.ErrorContains(t, err, "message index: 0")
	require.Equal(t, int64(4), calls.Load())
}

func TestABCIHeightNotFoundAndUnavailable(t *testing.T) {
	var mode atomic.Int64
	c := mockABCI(t, func(_ *http.Request, _ abciWireRequest) abci.ResponseQuery {
		response := sdkerrors.QueryResult(sdkerrors.ErrKeyNotFound.Wrap("missing"), false)
		response.Height = 42
		switch mode.Load() {
		case 1:
			response.Height = 41
		case 2:
			response.Codespace = "different-app"
		case 4:
			response.Height = 0
			response.Code = sdkerrors.ErrUnknownRequest.ABCICode()
		case 3:
			response.Code = 0
			response.Value = []byte{255}
		}

		return *response
	})
	id := sinkid.ID{Owner: "akash1fixture"}
	_, err := c.Transfer(t.Context(), 42, id)
	require.ErrorIs(t, err, ErrRecordNotFound)
	for _, m := range []int64{1, 2, 3, 4} {
		mode.Store(m)
		_, err = c.Transfer(t.Context(), 42, id)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrRecordNotFound)
		require.NotEqual(t, codes.NotFound, status.Code(err))
	}
}

func TestABCICancellationAndPermit(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	defer close(release)
	c := mockABCI(t, func(r *http.Request, _ abciWireRequest) abci.ResponseQuery {
		started <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-release:
		}

		return abci.ResponseQuery{Height: 42}
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := c.sink.State(ctx, &sinkv1.QueryStateRequest{})
		done <- err
	}()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.NoError(t, c.reads.Acquire(t.Context(), 2))
	stop, cancelStop := context.WithCancel(t.Context())
	cancelStop()
	_, err := c.sink.State(WithStop(t.Context(), stop), &sinkv1.QueryStateRequest{})
	require.ErrorIs(t, err, context.Canceled)
	c.reads.Release(2)
	select {
	case <-started:
		t.Fatal("queued cancellation reached HTTP")
	default:
	}
}

func TestABCILimitsAndCredentialErrors(t *testing.T) {
	body := &boundedRPCBody{
		ReadCloser: io.NopCloser(strings.NewReader("1234")),
		remaining:  3,
	}

	_, err := io.ReadAll(body)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	for _, url := range []string{"http://user:SECRET@127.0.0.1", "http://SECRET:%Q/"} {
		_, err := Dial(t.Context(), Options{
			ABCI: true,
			Node: url,
		})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "SECRET")
	}

	require.ErrorIs(t, sanitizeLegacyHTTP(context.Canceled), context.Canceled)
}

func TestABCIRedirectTargetsAndErrors(t *testing.T) {
	for _, scenario := range []string{"relative", "cross-host", "userinfo-same", "userinfo-cross", "hop20", "hop21", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			var mu sync.Mutex
			var request rpctypes.RPCRequest
			var targets, referers []string
			respond := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				targets = append(targets, r.RequestURI)
				referers = append(referers, r.Referer())
				data, err := json.Marshal(rpctypes.NewRPCSuccessResponse(request.ID, &ctypes.ResultStatus{}))
				require.NoError(t, err)
				_, _ = w.Write(data)
			})
			cross := httptest.NewUnstartedServer(respond)
			listener, err := net.Listen("tcp", "[::1]:0")
			require.NoError(t, err)
			cross.Listener = listener
			cross.Start()
			defer cross.Close()
			first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/final/path" {
					respond.ServeHTTP(w, r)
					return
				}

				mu.Lock()
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				targets = append(targets, r.RequestURI)
				referers = append(referers, r.Referer())
				mu.Unlock()
				location := "/final%2Fpath?keep=ok"
				switch scenario {
				case "relative":
					location = "../final%2Fpath?keep=ok"
				case "cross-host":
					location = cross.URL + location
				case "userinfo-same":
					location = "http://user:SECRET@" + r.Host + location
				case "userinfo-cross":
					location = strings.Replace(cross.URL, "http://", "http://user:SECRET@", 1) + location
				case "invalid":
					location = "http://SECRET:%Q/"
				case "hop20", "hop21":
					hops, err := strconv.Atoi(r.URL.Query().Get("left"))
					require.NoError(t, err)
					if hops == 1 {
						location = "/final%2Fpath?keep=ok"
					} else {
						location = "/hop?left=" + strconv.Itoa(hops-1)
					}
				}

				w.Header().Set("Location", location)
				w.WriteHeader(307)
			}))
			defer first.Close()
			initial := "/nested/initial%2Fpath?token=SOURCE"
			if scenario == "hop20" {
				initial = "/hop?left=20"
			}

			if scenario == "hop21" {
				initial = "/hop?left=21"
			}

			c, err := Dial(t.Context(), Options{
				ABCI:           true,
				Node:           first.URL + initial,
				RequestTimeout: time.Second,
			})
			success := scenario == "relative" || scenario == "cross-host" || scenario == "hop20"
			if success {
				require.NoError(t, err)
				require.NoError(t, c.Close())
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "SECRET")
				require.NotContains(t, err.Error(), "SOURCE")
			}

			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, initial, targets[0])
			for _, ref := range referers {
				require.Empty(t, ref)
			}

			if scenario == "relative" || scenario == "cross-host" {
				require.Equal(t, []string{initial, "/final%2Fpath?keep=ok"}, targets)
			}

			if strings.HasPrefix(scenario, "userinfo") {
				require.Len(t, targets, 1)
			}

			if scenario == "hop20" || scenario == "hop21" {
				require.Len(t, targets, 21)
			}
		})
	}
}

func TestABCIRequestTimeoutAndMetadata(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	var calls atomic.Int64
	c := mockABCI(t, func(r *http.Request, _ abciWireRequest) abci.ResponseQuery {
		calls.Add(1)
		select {
		case <-r.Context().Done():
		case <-release:
		}
		return abci.ResponseQuery{Height: 42}
	})
	for _, heights := range [][]string{{"-1"}, {"0"}, {"garbage"}, {"42", "42"}} {
		ctx := metadata.NewOutgoingContext(t.Context(), metadata.MD{grpctypes.GRPCBlockHeightHeader: heights})
		_, err := c.sink.State(ctx, &sinkv1.QueryStateRequest{})
		require.Error(t, err)
	}
	require.Zero(t, calls.Load())
	_, err := c.sink.State(t.Context(), &sinkv1.QueryStateRequest{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, int64(1), calls.Load())
}
