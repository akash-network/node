package simnode

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"math"
	"net"
	"slices"
	"sort"
	"strconv"
	"time"

	errorsmod "cosmossdk.io/errors"
	p2pproto "github.com/cometbft/cometbft/proto/tendermint/p2p"
	"github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	grpctypes "github.com/cosmos/cosmos-sdk/types/grpc"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	gogoproto "github.com/cosmos/gogoproto/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"
)

// view is the height and block time a query reads.
type view struct {
	height int64
	time   time.Time
}

type viewKey struct{}

func withView(ctx context.Context, v view) context.Context {
	return context.WithValue(ctx, viewKey{}, v)
}

func viewOf(ctx context.Context) view {
	v, _ := ctx.Value(viewKey{}).(view)
	return v
}

// queryRouter maps gRPC full method names to handlers that take and return protobuf bytes, as BaseApp's
// GRPCQueryRouter serves abci_query. It satisfies gogoproto's grpc.Server, so generated Register functions fill it.
type queryRouter map[string]func(ctx context.Context, req []byte) ([]byte, error)

func (r queryRouter) RegisterService(sd *grpc.ServiceDesc, ss any) {
	for _, m := range sd.Methods {
		handler := m.Handler
		r["/"+sd.ServiceName+"/"+m.MethodName] = func(ctx context.Context, req []byte) ([]byte, error) {
			res, err := handler(ss, ctx, func(in any) error { return gogoproto.Unmarshal(req, in.(gogoproto.Message)) }, nil)
			if err != nil {
				return nil, err
			}

			return gogoproto.Marshal(res.(gogoproto.Message))
		}
	}
}

// registerQueries registers the services that both gRPC and abci_query serve.
func (n *Node) registerQueries(s gogoServer) {
	sinkv1.RegisterQueryServer(s, &sinkQuery{n: n})
	authtypes.RegisterQueryServer(s, &authQuery{n: n})
}

type gogoServer interface {
	RegisterService(sd *grpc.ServiceDesc, ss any)
}

// serveGRPC serves the gRPC services on a new loopback listener with the SDK's gogoproto codec.
func (n *Node) serveGRPC(ctx context.Context) (string, error) {
	var lc net.ListenConfig
	l, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	srv := grpc.NewServer(
		grpc.ForceServerCodec(codec.NewProtoCodec(encoding().InterfaceRegistry).GRPCCodec()),
		grpc.UnaryInterceptor(n.intercept),
	)
	n.registerQueries(srv)
	txtypes.RegisterServiceServer(srv, &txService{n: n})
	cmtservice.RegisterServiceServer(srv, &nodeService{n: n})

	n.grpcServer = srv
	go func() { _ = srv.Serve(l) }() // returns once Close stops the server

	return l.Addr().String(), nil
}

// intercept applies the latency and the x-cosmos-block-height semantics of baseapp/grpcserver.go:50-77: a request is
// answered at its header's height, or at the latest height without one, and the response header repeats that height.
func (n *Node) intercept(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	n.count(info.FullMethod)
	if err := delay(ctx, n.opts.AkashLatency); err != nil {
		return nil, status.FromContextError(err).Err()
	}

	var height int64
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get(grpctypes.GRPCBlockHeightHeader); len(values) == 1 {
			var err error
			if height, err = strconv.ParseInt(values[0], 10, 64); err != nil {
				return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest,
					"Baseapp.RegisterGRPCServer: invalid height header %q: %v", grpctypes.GRPCBlockHeightHeader, err)
			}
		}
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	h, err := n.queryHeight(height)
	if err != nil {
		return nil, err
	}

	echo := strconv.FormatInt(h+n.sc.Akash.EchoOffset, 10)
	if err := grpc.SetHeader(ctx, metadata.Pairs(grpctypes.GRPCBlockHeightHeader, echo)); err != nil {
		return nil, err
	}

	t, _ := n.akashBlockTime(h)

	return handler(withView(ctx, view{height: h, time: t}), req)
}

// grpcErrorToSDK maps a query error to an SDK error as baseapp/abci.go gRPCErrorToSDKError does.
func grpcErrorToSDK(err error) error {
	st, ok := status.FromError(err)
	if !ok {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, err.Error())
	}

	switch st.Code() {
	case codes.NotFound:
		return errorsmod.Wrap(sdkerrors.ErrKeyNotFound, err.Error())
	case codes.InvalidArgument, codes.FailedPrecondition:
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, err.Error())
	case codes.Unauthenticated:
		return errorsmod.Wrap(sdkerrors.ErrUnauthorized, err.Error())
	default:
		return errorsmod.Wrap(sdkerrors.ErrUnknownRequest, err.Error())
	}
}

// sinkQuery is x/sink/keeper/grpc_query.go over the versioned state at the request's height. Callers hold n.mu for
// reading.
type sinkQuery struct {
	sinkv1.UnimplementedQueryServer
	n *Node
}

func (q *sinkQuery) Params(context.Context, *sinkv1.QueryParamsRequest) (*sinkv1.QueryParamsResponse, error) {
	return &sinkv1.QueryParamsResponse{Params: q.n.sc.Akash.Params}, nil
}

func (q *sinkQuery) Reporters(_ context.Context, req *sinkv1.QueryReportersRequest) (*sinkv1.QueryReportersResponse, error) {
	policies := slices.SortedFunc(slices.Values(q.n.sc.Akash.Policies), func(a, b sinkv1.ReporterPolicy) int {
		return cmp.Compare(a.Version, b.Version)
	})

	version := uint64(0)
	if req != nil {
		version = req.Version
	}

	switch version {
	case 0:
		if len(policies) == 0 {
			return &sinkv1.QueryReportersResponse{Reporters: []sinkv1.ReporterPolicy{}}, nil
		}

		return &sinkv1.QueryReportersResponse{Reporters: policies[len(policies)-1:]}, nil
	case math.MaxUint64:
		return &sinkv1.QueryReportersResponse{Reporters: policies}, nil
	}

	for _, p := range policies {
		if p.Version == version {
			return &sinkv1.QueryReportersResponse{Reporters: []sinkv1.ReporterPolicy{p}}, nil
		}
	}

	return nil, status.Errorf(codes.NotFound, "reporter policy version %d not found", version)
}

func (q *sinkQuery) Transfer(ctx context.Context, req *sinkv1.QueryTransferRequest) (*sinkv1.QueryTransferResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	if err := req.ValidateBasic(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	v := viewOf(ctx)
	t, _, err := q.n.newStage(v.height, v.time).get(req.ID)
	if err != nil {
		return nil, err
	}

	return &sinkv1.QueryTransferResponse{Transfer: *t}, nil
}

func (q *sinkQuery) Account(_ context.Context, req *sinkv1.QueryAccountRequest) (*sinkv1.QueryAccountResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	if err := req.ValidateBasic(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	a := q.n.sc.Akash
	usage, ok := a.Usage[req.Owner]
	if !ok {
		zero := sdk.NewInt64Coin("uakt", 0)
		usage = sinkv1.AccountUsage{TotalSent: zero, EpochSent: zero}
	}

	return &sinkv1.QueryAccountResponse{Usage: usage, Epoch: a.State.Epoch}, nil
}

func (q *sinkQuery) State(ctx context.Context, _ *sinkv1.QueryStateRequest) (*sinkv1.QueryStateResponse, error) {
	v := viewOf(ctx)
	s := q.n.sc.Akash.State

	return &sinkv1.QueryStateResponse{
		Totals:      s.Totals,
		Epoch:       s.Epoch,
		SendUsage:   s.SendUsage,
		RefundUsage: s.RefundUsage,
		RefundHead:  q.n.newStage(v.height, v.time).refundHead(),
	}, nil
}

// Transfers walks one owner's transfers or one status bucket in record-key order, or the history chronologically.
// The cursor carries the walk, so a resumed page ignores the request's filters.
func (q *sinkQuery) Transfers(ctx context.Context, req *sinkv1.QueryTransfersRequest) (*sinkv1.QueryTransfersResponse, error) {
	if req == nil {
		req = &sinkv1.QueryTransfersRequest{}
	}
	if err := req.ValidateBasic(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	var page sdkquery.PageRequest
	if req.Pagination != nil {
		page = *req.Pagination
	}
	if page.Offset > 0 && req.Status == sinkv1.StatusInvalid {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, status must be provided")
	}

	w, resume, err := transfersWalk(req, page.Key)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if w.path == pathHistory {
		walk := int(q.n.historyWalks.Add(1))
		if slices.Contains(q.n.sc.Akash.FailHistory, walk) {
			return nil, status.Errorf(codes.Unavailable, "simnode: history walk %d fails", walk)
		}
	}

	transfers, next := q.n.walk(viewOf(ctx).height, w, resume, page)

	return &sinkv1.QueryTransfersResponse{
		Transfers:  transfers,
		Pagination: &sdkquery.PageResponse{NextKey: next, Total: uint64(len(transfers))},
	}, nil
}

// Disputes walks every held transfer chronologically.
func (q *sinkQuery) Disputes(ctx context.Context, req *sinkv1.QueryDisputesRequest) (*sinkv1.QueryDisputesResponse, error) {
	if req == nil {
		req = &sinkv1.QueryDisputesRequest{}
	}
	if err := req.ValidateBasic(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	var page sdkquery.PageRequest
	if req.Pagination != nil {
		page = *req.Pagination
	}
	if page.Offset > 0 && len(page.Key) > 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. offset and key are mutually exclusive")
	}

	w, resume := walk{path: pathDisputes}, []byte(nil)
	if len(page.Key) > 0 {
		var err error
		if w, resume, err = decodeCursor(page.Key); err != nil || w.path != pathDisputes {
			return nil, status.Error(codes.InvalidArgument, errInvalidCursor.Error())
		}
	}

	transfers, next := q.n.walk(viewOf(ctx).height, w, resume, page)

	return &sinkv1.QueryDisputesResponse{Transfers: transfers, Pagination: &sdkquery.PageResponse{NextKey: next}}, nil
}

// Walks over the transfer index; their cursors are this simulator's own, opaque as the keeper's are.
const (
	pathOwner byte = iota + 1
	pathHistory
	pathBucket
	pathDisputes
)

var errInvalidCursor = errors.New("invalid transfers pagination key")

type walk struct {
	path   byte
	status sinkv1.Status
	// owner is the record-key prefix of the owner walk: len(owner) || owner.
	owner []byte
}

func transfersWalk(req *sinkv1.QueryTransfersRequest, cursor []byte) (walk, []byte, error) {
	if len(cursor) > 0 {
		w, resume, err := decodeCursor(cursor)
		if err != nil || w.path == pathDisputes {
			return walk{}, nil, errInvalidCursor
		}

		return w, resume, nil
	}

	switch {
	case req.Owner != "":
		_, raw, err := bech32.DecodeAndConvert(req.Owner)
		if err != nil || len(raw) == 0 || len(raw) > math.MaxUint8 {
			return walk{}, nil, errors.New("invalid owner")
		}

		owner := append([]byte{byte(len(raw))}, raw...) //nolint:gosec // checked above: at most 255 bytes

		return walk{path: pathOwner, status: req.Status, owner: owner}, nil, nil
	case req.Status != sinkv1.StatusInvalid:
		return walk{path: pathBucket, status: req.Status}, nil, nil
	default:
		return walk{path: pathHistory}, nil, nil
	}
}

// encodeCursor is path, status, the owner prefix's length and bytes, then the record key to resume at.
func encodeCursor(w walk, key []byte) []byte {
	out := []byte{w.path, byte(w.status), byte(len(w.owner))} //nolint:gosec // statuses and owner prefixes fit a byte
	out = append(out, w.owner...)

	return append(out, key...)
}

func decodeCursor(b []byte) (walk, []byte, error) {
	if len(b) < 3 || b[0] < pathOwner || b[0] > pathDisputes || len(b) < 3+int(b[2]) {
		return walk{}, nil, errInvalidCursor
	}

	w := walk{path: b[0], status: sinkv1.Status(b[1])}
	if b[2] > 0 {
		w.owner = bytes.Clone(b[3 : 3+int(b[2])])
	}

	resume := b[3+int(b[2]):]
	if len(resume) == 0 {
		return walk{}, nil, errInvalidCursor
	}

	return w, bytes.Clone(resume), nil
}

func (w walk) chronological() bool { return w.path == pathHistory || w.path == pathDisputes }

func (w walk) matches(key []byte, t *sinkv1.TransferState) bool {
	switch w.path {
	case pathOwner:
		return bytes.HasPrefix(key, w.owner) && (w.status == sinkv1.StatusInvalid || t.Status == w.status)
	case pathBucket:
		return t.Status == w.status
	case pathDisputes:
		return t.Settlement.Hold != sinkv1.HoldNone
	default:
		return true
	}
}

// walk returns up to the page limit of matching transfers at height h after skipping the offset, starting at resume
// inclusive, and the cursor of the first match that did not fit, as the keeper's collect does.
func (n *Node) walk(h int64, w walk, resume []byte, page sdkquery.PageRequest) ([]sinkv1.TransferState, []byte) {
	order, compare := n.ak.index.record, bytes.Compare
	if w.chronological() {
		order, compare = n.ak.index.chrono, compareChrono
	}

	limit := page.Limit
	if limit == 0 {
		limit = sdkquery.DefaultLimit
	}

	i, step := 0, 1
	switch {
	case page.Reverse && resume != nil:
		i, step = sort.Search(len(order), func(j int) bool { return compare(order[j].key, resume) > 0 })-1, -1
	case page.Reverse:
		i, step = len(order)-1, -1
	case resume != nil:
		i = sort.Search(len(order), func(j int) bool { return compare(order[j].key, resume) >= 0 })
	}

	out := make([]sinkv1.TransferState, 0)
	skip := page.Offset
	var last []byte
	for ; i >= 0 && i < len(order); i += step {
		e := order[i]
		t := e.at(h)
		if t == nil || !w.matches(e.key, t) {
			continue
		}

		if skip > 0 {
			skip--
			continue
		}

		if uint64(len(out)) == limit {
			if n.sc.Akash.PageOverlap && last != nil {
				return out, encodeCursor(w, last)
			}

			return out, encodeCursor(w, e.key)
		}
		out = append(out, *t)
		last = e.key
	}

	return out, nil
}

// authQuery serves accounts, which do not change with height.
type authQuery struct {
	authtypes.UnimplementedQueryServer
	n *Node
}

func (q *authQuery) Account(_ context.Context, req *authtypes.QueryAccountRequest) (*authtypes.QueryAccountResponse, error) {
	if req == nil || req.Address == "" {
		return nil, status.Error(codes.InvalidArgument, "Address cannot be empty")
	}
	if _, _, err := bech32.DecodeAndConvert(req.Address); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	for _, a := range q.n.sc.Akash.Accounts {
		if a.Address != req.Address {
			continue
		}

		acc := &authtypes.BaseAccount{Address: a.Address, AccountNumber: a.Number}
		if len(a.PubKey) > 0 {
			pk, err := codectypes.NewAnyWithValue(&secp256k1.PubKey{Key: a.PubKey})
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			acc.PubKey = pk
		}

		packed, err := codectypes.NewAnyWithValue(acc)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		return &authtypes.QueryAccountResponse{Account: packed}, nil
	}

	return nil, status.Errorf(codes.NotFound, "account %s not found", req.Address)
}

// txService simulates against the latest state, without the replay-key check (x/auth/ante/sigverify.go).
type txService struct {
	txtypes.UnimplementedServiceServer
	n *Node
}

func (s *txService) Simulate(ctx context.Context, req *txtypes.SimulateRequest) (*txtypes.SimulateResponse, error) {
	if req == nil || req.TxBytes == nil {
		return nil, status.Error(codes.InvalidArgument, "empty txBytes is not allowed")
	}

	v := viewOf(ctx)
	tx := decodeTx(req.TxBytes)
	if err := s.n.anteError(tx, v.time); err != nil {
		return nil, status.Errorf(codes.Unknown, "%v with gas used: '%d'", err, gasPerTx)
	}

	used := gasFor(len(tx.msgs))
	if _, err := s.n.newStage(v.height, v.time).run(tx.msgs, s.n.simFailure); err != nil {
		return nil, status.Errorf(codes.Unknown, "%v with gas used: '%d'", err, used)
	}

	return &txtypes.SimulateResponse{
		GasInfo: &sdk.GasInfo{GasWanted: tx.gas, GasUsed: uint64(used)}, //nolint:gosec // positive
		Result:  &sdk.Result{},
	}, nil
}

// simFailure returns the injected simulation failure of a message, if any.
func (n *Node) simFailure(_ int, msg sdk.Msg) error {
	var id sinkv1.ID
	switch m := msg.(type) {
	case *sinkv1.MsgReportOutcome:
		id = m.ID
	case *sinkv1.MsgVetoVoid:
		id = m.ID
	case *sinkv1.MsgFinalizeRefund:
		id = m.ID
	default:
		return nil
	}

	for _, f := range n.sc.Akash.SimFailures {
		if f.ID == idText(id) && (f.TypeURL == "" || f.TypeURL == sdk.MsgTypeURL(msg)) {
			return errors.New(f.Error)
		}
	}

	return nil
}

// nodeService answers GetNodeInfo with the node ID that status reports.
type nodeService struct {
	cmtservice.UnimplementedServiceServer
	n *Node
}

func (s *nodeService) GetNodeInfo(context.Context, *cmtservice.GetNodeInfoRequest) (*cmtservice.GetNodeInfoResponse, error) {
	info := s.n.nodeInfo()

	return &cmtservice.GetNodeInfoResponse{
		DefaultNodeInfo: &p2pproto.DefaultNodeInfo{
			ProtocolVersion: p2pproto.ProtocolVersion{P2P: info.ProtocolVersion.P2P, Block: info.ProtocolVersion.Block, App: info.ProtocolVersion.App},
			DefaultNodeID:   string(info.DefaultNodeID),
			ListenAddr:      info.ListenAddr,
			Network:         info.Network,
			Version:         info.Version,
			Channels:        info.Channels,
			Moniker:         info.Moniker,
			Other:           p2pproto.DefaultNodeInfoOther{TxIndex: info.Other.TxIndex, RPCAddress: info.Other.RPCAddress},
		},
		ApplicationVersion: &cmtservice.VersionInfo{Name: appName, AppName: appName, Version: cometVersion},
	}, nil
}
