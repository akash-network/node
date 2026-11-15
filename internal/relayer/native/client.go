// Package native reads height-bound Sink state through CometBFT HTTP.
package native

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	cmthttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/gogoproto/proto"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	sink "pkg.akt.dev/go/node/sink/v1"
)

const maxSafeHeight int64 = 1<<53 - 1

var (
	ErrNotFound = errors.New("Sink record not found")
	ErrPruned   = errors.New("Sink query state pruned")
)

// QueryError preserves ABCI failure details; only Transfer classifies sdk/22 and sdk/38.
type QueryError struct {
	Code      uint32
	Codespace string
	Log       string
	Height    int64
	cause     error
}

func (e *QueryError) Error() string {
	return fmt.Sprintf("ABCI query failed (%s/%d) at %d: %s", e.Codespace, e.Code, e.Height, e.Log)
}

func (e *QueryError) Unwrap() error { return e.cause }

type Client struct {
	rpc     *cmthttp.HTTP
	chainID string
	maxAge  time.Duration
	now     func() time.Time
}

type Snapshot struct {
	Height int64
	Time   time.Time
	State  sink.QueryStateResponse
	Params sink.Params
	Policy sink.ReporterPolicy
}

func New(rpcURL, chainID string, maxAge time.Duration) (*Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil

	return NewWithHTTP(rpcURL, chainID, maxAge, &http.Client{
		Transport:     transport,
		Timeout:       15 * time.Second,
		CheckRedirect: RPCCheckRedirect,
	})
}

func NewWithHTTP(rpcURL, chainID string, maxAge time.Duration, httpClient *http.Client) (*Client, error) {
	if chainID == "" || maxAge < 0 || httpClient == nil {
		return nil, errors.New("invalid native client configuration")
	}

	endpoint, httpClient, err := rpcEndpointHTTP(rpcURL, httpClient, true)
	if err != nil {
		return nil, err
	}

	client, err := cmthttp.NewWithClient(endpoint, "/websocket", httpClient)
	if err != nil {
		return nil, err
	}

	return &Client{
		rpc:     client,
		chainID: chainID,
		maxAge:  maxAge,
		now:     time.Now,
	}, nil
}

func queryHeight(height int64) error {
	if height <= 0 || height > maxSafeHeight {
		return errors.New("invalid native query height")
	}
	return nil
}

func (c *Client) query(ctx context.Context, path string, data []byte, height int64, prove bool) (*abci.ResponseQuery, error) {
	if err := queryHeight(height); err != nil {
		return nil, err
	}

	reply, err := c.rpc.ABCIQueryWithOptions(ctx, path, data, rpcclient.ABCIQueryOptions{
		Height: height,
		Prove:  prove,
	})
	if err != nil {
		return nil, err
	}

	value := &reply.Response
	if value.Code != 0 {
		failure := &QueryError{
			Code:      value.Code,
			Codespace: value.Codespace,
			Log:       value.Log,
			Height:    value.Height,
		}
		if path == "/akash.sink.v1.Query/Transfer" && value.Codespace == "sdk" {
			switch value.Code {
			case 22:
				failure.cause = ErrNotFound
			case 38:
				failure.cause = ErrPruned
			}
		}
		return nil, failure
	}
	if value.Height != height {
		return nil, fmt.Errorf("Akash query height mismatch: requested %d, received %d", height, value.Height)
	}

	return value, nil
}

type queryConnection struct {
	client *Client
	height int64
}

func (q queryConnection) Invoke(ctx context.Context, method string, args, reply any, _ ...grpc.CallOption) error {
	request, ok := args.(proto.Message)
	if !ok {
		return errors.New("query request is not protobuf")
	}
	response, ok := reply.(proto.Message)
	if !ok {
		return errors.New("query response is not protobuf")
	}

	raw, err := proto.Marshal(request)
	if err != nil {
		return err
	}
	result, err := q.client.query(ctx, method, raw, q.height, false)
	if err != nil {
		return err
	}
	if err := responseShape(method, result.Value); err != nil {
		return err
	}

	return proto.Unmarshal(result.Value, response)
}

func (q queryConnection) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, errors.New("ABCI transport supports unary queries only")
}

func (c *Client) queries(height int64) sink.QueryClient {
	return sink.NewQueryClient(queryConnection{
		client: c,
		height: height,
	})
}

func (c *Client) fresh(t time.Time) bool {
	age := float64(c.now().UnixMilli())/1000 - float64(t.Unix())
	return age >= -10 && age <= c.maxAge.Seconds()
}

func (c *Client) Snapshot(ctx context.Context) (Snapshot, error) {
	status, err := c.rpc.Status(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if status.NodeInfo.Network != c.chainID || status.SyncInfo.CatchingUp || !c.fresh(status.SyncInfo.LatestBlockTime) {
		return Snapshot{}, errors.New("wrong, syncing, or stale Akash node")
	}

	info, err := c.rpc.ABCIInfo(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	height := min(status.SyncInfo.LatestBlockHeight, info.Response.LastBlockHeight)
	if height <= 1 || height > maxSafeHeight {
		return Snapshot{}, errors.New("Akash node has no queryable committed state")
	}

	block, err := c.rpc.Block(ctx, &height)
	if err != nil {
		return Snapshot{}, err
	}
	if block.Block == nil || block.Block.Height != height || block.Block.ChainID != c.chainID || !c.fresh(block.Block.Time) {
		return Snapshot{}, errors.New("wrong or stale Akash committed block")
	}

	query := c.queries(height)
	var state *sink.QueryStateResponse
	var params *sink.QueryParamsResponse
	var reporters *sink.QueryReportersResponse

	group, queryCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		state, err = query.State(queryCtx, &sink.QueryStateRequest{})
		return err
	})

	group.Go(func() error {
		var err error
		params, err = query.Params(queryCtx, &sink.QueryParamsRequest{})
		return err
	})

	group.Go(func() error {
		var err error
		reporters, err = query.Reporters(queryCtx, &sink.QueryReportersRequest{})
		return err
	})

	if err := group.Wait(); err != nil {
		return Snapshot{}, err
	}

	if len(reporters.Reporters) != 1 || reporters.Reporters[0].Version == 0 || state.Epoch > math.MaxInt64 {
		return Snapshot{}, errors.New("missing reporter policy or invalid quota epoch")
	}
	for _, usage := range []sink.EpochUsage{state.SendUsage, state.RefundUsage} {
		if usage.Epoch != state.Epoch || validCoin(usage.Amount, false) != nil || (state.Epoch == 0 && !usage.Amount.Amount.IsZero()) {
			return Snapshot{}, errors.New("invalid quota usage")
		}
	}

	return Snapshot{
		Height: height,
		Time:   block.Block.Time.Truncate(time.Second),
		State:  *state,
		Params: params.Params,
		Policy: reporters.Reporters[0],
	}, nil
}

func (c *Client) Transfer(ctx context.Context, id sink.ID, height int64) (sink.TransferState, error) {
	if _, err := ParseID(id.String()); err != nil {
		return sink.TransferState{}, err
	}

	reply, err := c.queries(height).Transfer(ctx, &sink.QueryTransferRequest{ID: id})
	if err != nil {
		return sink.TransferState{}, err
	}

	if err := validTransfer(reply.Transfer); err != nil {
		return sink.TransferState{}, err
	}
	if reply.Transfer.ID != id {
		return sink.TransferState{}, errors.New("Akash transfer ID mismatch")
	}

	return reply.Transfer, nil
}
