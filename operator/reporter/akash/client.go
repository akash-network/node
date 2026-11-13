// Package akash is the reporter's only path to Akash: height-pinned gRPC state reads, the height tracker, listing and
// the audit cursor, and the CometBFT calls that broadcast and look up transactions (spec §5.3).
package akash

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/std"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	grpctypes "github.com/cosmos/cosmos-sdk/types/grpc"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/operator/reporter/internal/clock"
	"pkg.akt.dev/node/v2/operator/reporter/metrics"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

const (
	metricsChain      = "akash"
	maxRecvBytes      = 64 << 20
	defaultMinRefresh = time.Second
	txIndexOn         = "on"
	// errInCache is CometBFT's mempool error for bytes already pending or included; broadcast_tx_sync returns it as a
	// JSON-RPC error, not a CheckTx code (spec §6.5).
	errInCache = "tx already exists in cache"
)

var (
	// ErrRecordNotFound reports a transfer the node does not have at the read height.
	ErrRecordNotFound = errors.New("Sink record not found") //nolint:staticcheck // verbatim TS text
	// ErrAccountNotFound reports a reporter address that has no account yet.
	ErrAccountNotFound = errors.New("reporter account not found")

	errDifferentNodes   = errors.New("Akash RPC and gRPC endpoints are different nodes") //nolint:staticcheck // verbatim spec text
	errTransferHeight   = errors.New("Akash transfer query height mismatch")             //nolint:staticcheck // verbatim TS text
	errTransferMissing  = errors.New("missing sink transfer")
	errTransferMismatch = errors.New("Akash transfer ID mismatch") //nolint:staticcheck // verbatim TS text
)

// Options configures a Client.
type Options struct {
	// Node is the CometBFT RPC URL.
	Node string
	// GRPCAddr is the gRPC host:port of the same node.
	GRPCAddr string
	// GRPCInsecure uses plaintext gRPC for a non-loopback address; loopback always uses plaintext.
	GRPCInsecure bool
	ChainID      string
	// MaxAge is the oldest block age a snapshot accepts.
	MaxAge time.Duration
	// RequestTimeout bounds each request; zero means no bound.
	RequestTimeout time.Duration
	// Concurrency limits concurrent gRPC calls and sizes the CometBFT connection pool.
	Concurrency int
	// MinRefresh is the shortest interval between tracker refreshes; zero means one second.
	MinRefresh time.Duration
	// Clock judges block freshness and refresh intervals; nil means the system clock.
	Clock   clock.Clock
	Metrics *metrics.Metrics
}

// Client reads one Akash node through gRPC and CometBFT RPC. It is safe for concurrent use.
type Client struct {
	chainID    string
	maxAge     time.Duration
	timeout    time.Duration
	minRefresh time.Duration
	clock      clock.Clock
	metrics    *metrics.Metrics

	transport *http.Transport
	rpc       *rpchttp.HTTP
	conn      *grpc.ClientConn
	registry  codectypes.InterfaceRegistry
	sink      sinkv1.QueryClient
	auth      authtypes.QueryClient
	txs       txtypes.ServiceClient
	reads     *semaphore.Weighted
	txIndex   bool

	track tracker
}

// BroadcastResult is a broadcast_tx_sync answer. InCache means identical bytes were already pending or included;
// such a result has no CheckTx outcome, and Hash is computed locally.
type BroadcastResult struct {
	Code      uint32
	Codespace string
	Log       string
	Hash      []byte
	InCache   bool
}

// TxResult is the delivery result of an included transaction.
type TxResult struct {
	Height    int64
	Code      uint32
	Codespace string
	Log       string
}

// Dial connects to the node's CometBFT RPC and gRPC endpoints and refuses them unless they are one node.
func Dial(ctx context.Context, opts Options) (*Client, error) {
	concurrency := max(opts.Concurrency, 1)

	c := &Client{
		chainID:    opts.ChainID,
		maxAge:     opts.MaxAge,
		timeout:    opts.RequestTimeout,
		minRefresh: opts.MinRefresh,
		clock:      opts.Clock,
		metrics:    opts.Metrics,
		registry:   interfaceRegistry(),
		reads:      semaphore.NewWeighted(int64(concurrency)),
	}
	if c.minRefresh <= 0 {
		c.minRefresh = defaultMinRefresh
	}
	if c.clock == nil {
		c.clock = clock.Real()
	}

	c.transport = http.DefaultTransport.(*http.Transport).Clone()
	c.transport.MaxIdleConns = concurrency
	c.transport.MaxIdleConnsPerHost = concurrency

	var err error
	if c.rpc, err = rpchttp.NewWithClient(opts.Node, "/websocket", &http.Client{Transport: c.transport}); err != nil {
		return nil, err
	}

	creds := credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	if plaintext(opts.GRPCAddr, opts.GRPCInsecure) {
		creds = insecure.NewCredentials()
	}

	c.conn, err = grpc.NewClient(opts.GRPCAddr,
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(
			grpc.ForceCodec(codec.NewProtoCodec(c.registry).GRPCCodec()),
			grpc.MaxCallRecvMsgSize(maxRecvBytes),
		),
		grpc.WithUnaryInterceptor(c.intercept),
	)
	if err != nil {
		return nil, err
	}

	c.sink = sinkv1.NewQueryClient(c.conn)
	c.auth = authtypes.NewQueryClient(c.conn)
	c.txs = txtypes.NewServiceClient(c.conn)

	if err := c.sameNode(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}

	return c, nil
}

// Close releases the gRPC connection and idle CometBFT connections.
func (c *Client) Close() error {
	c.transport.CloseIdleConnections()
	return c.conn.Close()
}

// sameNode requires CometBFT's node ID to equal gRPC's, so that every height exists on both, and records whether the
// node indexes transactions.
func (c *Client) sameNode(ctx context.Context) error {
	st, err := c.status(ctx)
	if err != nil {
		return err
	}

	info, err := cmtservice.NewServiceClient(c.conn).GetNodeInfo(ctx, &cmtservice.GetNodeInfoRequest{})
	if err != nil {
		return err
	}

	if info.DefaultNodeInfo == nil || info.DefaultNodeInfo.DefaultNodeID != string(st.NodeInfo.DefaultNodeID) {
		return errDifferentNodes
	}

	c.txIndex = st.NodeInfo.Other.TxIndex == txIndexOn

	return nil
}

// plaintext reports whether addr is reached without TLS: loopback hosts always, others only when insecure is set.
func plaintext(addr string, insecure bool) bool {
	if insecure {
		return true
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}

	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

// interfaceRegistry resolves the account and key types in auth answers.
func interfaceRegistry() codectypes.InterfaceRegistry {
	reg := codectypes.NewInterfaceRegistry()
	std.RegisterInterfaces(reg)
	authtypes.RegisterInterfaces(reg)
	vestingtypes.RegisterInterfaces(reg)

	return reg
}

func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.timeout <= 0 {
		return context.WithCancel(ctx)
	}

	return context.WithTimeout(ctx, c.timeout)
}

// intercept bounds every gRPC call by the concurrency limit and the request timeout, and records it.
func (c *Client) intercept(
	ctx context.Context,
	method string,
	req, reply any,
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	if err := c.reads.Acquire(ctx, 1); err != nil {
		return err
	}
	defer c.reads.Release(1)

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	began := time.Now()
	err := invoker(ctx, method, req, reply, cc, opts...)
	c.metrics.RecordRPC(metricsChain, method, time.Since(began), err)

	return err
}

// comet runs one CometBFT call under the request timeout and records it.
func (c *Client) comet(ctx context.Context, method string, call func(context.Context) error) error {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	began := time.Now()
	err := call(ctx)
	c.metrics.RecordRPC(metricsChain, method, time.Since(began), err)

	return err
}

// pinned runs a gRPC read at height and requires the response header to repeat it.
func (c *Client) pinned(ctx context.Context, height int64, mismatch error, call func(context.Context, ...grpc.CallOption) error) error {
	want := strconv.FormatInt(height, 10)

	var header metadata.MD
	if err := call(metadata.AppendToOutgoingContext(ctx, grpctypes.GRPCBlockHeightHeader, want), grpc.Header(&header)); err != nil {
		return err
	}

	if got := header.Get(grpctypes.GRPCBlockHeightHeader); len(got) != 1 || got[0] != want {
		return mismatch
	}

	return nil
}

// Transfer reads one transfer at height. A transfer the node does not have is ErrRecordNotFound; any other failure
// means the node is unavailable.
func (c *Client) Transfer(ctx context.Context, height int64, id sinkid.ID) (sinkv1.TransferState, error) {
	var res *sinkv1.QueryTransferResponse
	err := c.pinned(ctx, height, errTransferHeight, func(ctx context.Context, opts ...grpc.CallOption) (err error) {
		res, err = c.sink.Transfer(ctx, &sinkv1.QueryTransferRequest{ID: id.Proto()}, opts...)
		return err
	})

	switch {
	case status.Code(err) == codes.NotFound:
		return sinkv1.TransferState{}, ErrRecordNotFound
	case errors.Is(err, errTransferHeight):
		return sinkv1.TransferState{}, err
	case err != nil:
		return sinkv1.TransferState{}, fmt.Errorf("Transfer query failed: %w", err) //nolint:staticcheck // TS text
	}

	t := res.Transfer
	if isZero(t.Settlement) {
		return sinkv1.TransferState{}, errTransferMissing
	}

	rec, err := sinkid.DecodeRecord(t)
	if err != nil {
		return sinkv1.TransferState{}, err
	}

	if sinkid.Compare(rec.ID, id) != 0 {
		return sinkv1.TransferState{}, errTransferMismatch
	}

	return t, nil
}

// AccountNumber reads the account number of addr at the node's latest state.
func (c *Client) AccountNumber(ctx context.Context, addr sdk.AccAddress) (uint64, error) {
	address, err := bech32.ConvertAndEncode(sdkutil.Bech32PrefixAccAddr, addr)
	if err != nil {
		return 0, err
	}

	res, err := c.auth.Account(ctx, &authtypes.QueryAccountRequest{Address: address})
	if status.Code(err) == codes.NotFound {
		return 0, ErrAccountNotFound
	}
	if err != nil {
		return 0, err
	}

	var acc sdk.AccountI
	if err := c.registry.UnpackAny(res.Account, &acc); err != nil {
		return 0, err
	}

	return acc.GetAccountNumber(), nil
}

// Simulate runs txBytes against the node's latest state and returns the gas it used. A failure keeps the node's
// message, which names the failing message index.
func (c *Client) Simulate(ctx context.Context, txBytes []byte) (uint64, error) {
	res, err := c.txs.Simulate(ctx, &txtypes.SimulateRequest{TxBytes: txBytes})
	if err != nil {
		return 0, err
	}

	if res.GasInfo == nil {
		return 0, errors.New("simulation returned no gas info")
	}

	return res.GasInfo.GasUsed, nil
}

// BroadcastSync submits txBytes with broadcast_tx_sync. A CheckTx rejection is a result with a non-zero Code; a
// transport or other JSON-RPC failure is an error.
func (c *Client) BroadcastSync(ctx context.Context, txBytes []byte) (BroadcastResult, error) {
	var out BroadcastResult
	err := c.comet(ctx, "broadcast_tx_sync", func(ctx context.Context) error {
		res, err := c.rpc.BroadcastTxSync(ctx, txBytes)
		if err != nil {
			if strings.Contains(err.Error(), errInCache) {
				sum := sha256.Sum256(txBytes)
				out = BroadcastResult{Hash: sum[:], InCache: true}

				return nil
			}

			return err
		}

		out = BroadcastResult{Code: res.Code, Codespace: res.Codespace, Log: res.Log, Hash: res.Hash}

		return nil
	})

	return out, err
}

// Tx looks up an included transaction by hash; it returns nil and no error when the node does not have it.
func (c *Client) Tx(ctx context.Context, hash []byte) (*TxResult, error) {
	var out *TxResult
	err := c.comet(ctx, "tx", func(ctx context.Context) error {
		res, err := c.rpc.Tx(ctx, hash, false)
		if err != nil {
			if strings.Contains(err.Error(), fmt.Sprintf("tx (%X) not found", hash)) {
				return nil
			}

			return err
		}

		out = &TxResult{Height: res.Height, Code: res.TxResult.Code, Codespace: res.TxResult.Codespace, Log: res.TxResult.Log}

		return nil
	})

	return out, err
}

// TxIndexEnabled reports whether the node indexed transactions when Dial read its status.
func (c *Client) TxIndexEnabled() bool { return c.txIndex }
