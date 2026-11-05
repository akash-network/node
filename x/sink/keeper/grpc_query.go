package keeper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"

	"cosmossdk.io/collections"
	collcodec "cosmossdk.io/collections/codec"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	types "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/util/query"
)

// Transfers cursors use the util/query encoding. Its unsolicited byte marks the
// walk; a single status bucket walk carries no marker.
const (
	transfersOwnerPath   byte = 1
	transfersHistoryPath byte = 2
)

// allStatuses lists every financial status in key order.
var allStatuses = []byte{
	byte(types.StatusTransferPending),
	byte(types.StatusMinted),
	byte(types.StatusVoided),
	byte(types.StatusRefundPending),
	byte(types.StatusRefunded),
}

var errInvalidTransfersCursor = errors.New("invalid transfers pagination key")

type Querier struct {
	*keeper
}

var _ types.QueryServer = Querier{}

func (q Querier) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	params, err := q.GetParams(ctx)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "unable to retrieve params: %s", err.Error())
	}

	return &types.QueryParamsResponse{Params: params}, nil
}

// Reporters returns the current policy for version 0, every retained policy for
// math.MaxUint64 and otherwise the requested version.
func (q Querier) Reporters(ctx context.Context, req *types.QueryReportersRequest) (*types.QueryReportersResponse, error) {
	if req == nil {
		req = &types.QueryReportersRequest{}
	}

	switch req.Version {
	case 0:
		current, err := q.policySequence.Peek(ctx)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		// No policy exists before the first reporter configuration.
		if current == 0 {
			return &types.QueryReportersResponse{Reporters: []types.ReporterPolicy{}}, nil
		}

		policy, err := q.GetCurrentPolicy(ctx)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		return &types.QueryReportersResponse{Reporters: []types.ReporterPolicy{policy}}, nil
	case math.MaxUint64:
		policies, err := q.allPolicies(ctx)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		return &types.QueryReportersResponse{Reporters: policies}, nil
	}

	policy, err := q.policies.Get(ctx, req.Version)
	if errors.Is(err, collections.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "reporter policy version %d not found", req.Version)
	}

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if policy.Version != req.Version {
		return nil, status.Error(codes.Internal, "reporter policy key mismatch")
	}

	return &types.QueryReportersResponse{Reporters: []types.ReporterPolicy{policy}}, nil
}

func (q Querier) Transfer(ctx context.Context, req *types.QueryTransferRequest) (*types.QueryTransferResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if err := req.ValidateBasic(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	t, err := q.GetTransfer(ctx, req.ID)
	if errors.Is(err, types.ErrTransferNotFound) {
		return nil, err
	}

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryTransferResponse{Transfer: t}, nil
}

// Account returns the owner's usage normalized to the current native epoch.
func (q Querier) Account(ctx context.Context, req *types.QueryAccountRequest) (*types.QueryAccountResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if err := req.ValidateBasic(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	owner, err := sdk.AccAddressFromBech32(req.Owner)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	sctx := sdk.UnwrapSDKContext(ctx)

	epoch, err := q.quotaEpoch(sctx)
	if err != nil {
		return nil, err
	}

	usage, err := q.GetAccountUsage(sctx, owner)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	usage, err = usage.At(epoch)
	if err != nil {
		return nil, err
	}

	return &types.QueryAccountResponse{
		Usage: usage,
		Epoch: epoch,
	}, nil
}

// State returns totals, usage normalized to the current native epoch and the
// oldest unheld refund candidate under governance or the current policy.
// Store read failures are Internal; epoch and usage clock errors keep their
// module errors.
func (q Querier) State(ctx context.Context, _ *types.QueryStateRequest) (*types.QueryStateResponse, error) {
	sctx := sdk.UnwrapSDKContext(ctx)

	totals, err := q.totals.Get(sctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	epoch, err := q.quotaEpoch(sctx)
	if err != nil {
		return nil, err
	}

	send, err := q.sendUsage.Get(sctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if send, err = send.At(epoch); err != nil {
		return nil, err
	}

	refund, err := q.refundUsage.Get(sctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if refund, err = refund.At(epoch); err != nil {
		return nil, err
	}

	// The sequence names the current version even before any policy exists,
	// when only the governance scope can hold refunds.
	current, err := q.policySequence.Peek(sctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	head, found, err := q.refundHead(sctx, current)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	res := &types.QueryStateResponse{
		Totals:      totals,
		Epoch:       epoch,
		SendUsage:   send,
		RefundUsage: refund,
	}

	if found {
		res.RefundHead = &head
	}

	return res, nil
}

// Disputes pages through held transfers in chronological order with native
// pagination keys.
func (q Querier) Disputes(ctx context.Context, req *types.QueryDisputesRequest) (*types.QueryDisputesResponse, error) {
	if req == nil {
		req = &types.QueryDisputesRequest{}
	}

	if err := req.ValidateBasic(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if p := req.Pagination; p != nil && p.Offset > 0 && len(p.Key) > 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. offset and key are mutually exclusive")
	}

	transfers, page, err := sdkquery.CollectionPaginate(ctx, q.disputes, req.Pagination, func(id types.ID, _ collections.NoValue) (types.TransferState, error) {
		return q.GetTransfer(ctx, id)
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryDisputesResponse{
		Transfers:  transfers,
		Pagination: page,
	}, nil
}

// Transfers lists transfers as the deployment and market queries do. An owner
// filter walks that owner's transfers in (height, sequence) order, a status
// filter alone walks its settlement bucket, and no filter walks the
// chronological history. The cursor carries the walk, so request filters are
// ignored on resume. Offset requires a status filter.
func (q Querier) Transfers(ctx context.Context, req *types.QueryTransfersRequest) (*types.QueryTransfersResponse, error) {
	if req == nil {
		req = &types.QueryTransfersRequest{}
	}

	if err := req.ValidateBasic(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	var page sdkquery.PageRequest
	if req.Pagination != nil {
		page = *req.Pagination
	}

	if page.Offset > 0 && req.Status == types.StatusInvalid {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, status must be provided")
	}

	if page.Limit == 0 {
		page.Limit = sdkquery.DefaultLimit
	}

	walk, err := newTransfersWalk(req, page.Key)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	transfers, next, err := q.walkTransfers(ctx, walk, page)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryTransfersResponse{
		Transfers: transfers,
		Pagination: &sdkquery.PageResponse{
			NextKey: next,
			Total:   uint64(len(transfers)),
		},
	}, nil
}

// transfersWalk is one Transfers iteration, resolved from the request on the
// first page and from the cursor afterwards.
type transfersWalk struct {
	// path is transfersOwnerPath, transfersHistoryPath or zero for one status bucket.
	path byte
	// states are the returned statuses: every status or one for the owner path,
	// one for a status bucket and a single zero for the history.
	states []byte
	// owner is the RecordIDKey owner prefix of the owner path.
	owner []byte
	// resume is the primary key of the first transfer to return, when resuming.
	resume []byte
}

func newTransfersWalk(req *types.QueryTransfersRequest, cursor []byte) (transfersWalk, error) {
	if len(cursor) > 0 {
		return decodeTransfersCursor(cursor)
	}

	wanted := byte(req.Status) //nolint:gosec // ValidateBasic admits only statuses 0 through 5.

	switch {
	case req.Owner != "":
		owner, err := RecordIDKey.ToPrefix(types.ID{Owner: req.Owner})
		if err != nil {
			return transfersWalk{}, err
		}

		states := allStatuses
		if req.Status != types.StatusInvalid {
			states = []byte{wanted}
		}

		return transfersWalk{
			path:   transfersOwnerPath,
			states: states,
			owner:  owner,
		}, nil
	case req.Status != types.StatusInvalid:
		return transfersWalk{states: []byte{wanted}}, nil
	default:
		return transfersWalk{
			path:   transfersHistoryPath,
			states: []byte{byte(types.StatusInvalid)},
		}, nil
	}
}

// decodeTransfersCursor accepts only cursors that Transfers itself issues.
func decodeTransfersCursor(cursor []byte) (transfersWalk, error) {
	states, prefix, pk, unsolicited, err := query.DecodePaginationKey(cursor)
	if err != nil {
		return transfersWalk{}, err
	}

	if len(states) == 0 || !bytes.Equal(prefix, states[:1]) || len(unsolicited) > 1 || bytes.Equal(unsolicited, []byte{0}) {
		return transfersWalk{}, errInvalidTransfersCursor
	}

	w := transfersWalk{
		states: states,
		resume: pk,
	}

	if len(unsolicited) == 1 {
		w.path = unsolicited[0]
	}

	single := len(states) == 1 && types.Status(states[0]).IsValid()

	switch w.path {
	case transfersHistoryPath:
		if !bytes.Equal(states, []byte{byte(types.StatusInvalid)}) {
			return transfersWalk{}, errInvalidTransfersCursor
		}

		if _, err = decodeExact(chronoIDKey, pk); err != nil {
			return transfersWalk{}, fmt.Errorf("%w: %w", errInvalidTransfersCursor, err)
		}
	case transfersOwnerPath:
		if !single && !bytes.Equal(states, allStatuses) {
			return transfersWalk{}, errInvalidTransfersCursor
		}

		var id types.ID
		if id, err = decodeExact(RecordIDKey, pk); err != nil {
			return transfersWalk{}, fmt.Errorf("%w: %w", errInvalidTransfersCursor, err)
		}

		if w.owner, err = RecordIDKey.ToPrefix(types.ID{Owner: id.Owner}); err != nil {
			return transfersWalk{}, fmt.Errorf("%w: %w", errInvalidTransfersCursor, err)
		}
	case 0:
		if !single {
			return transfersWalk{}, errInvalidTransfersCursor
		}

		if _, err = decodeExact(RecordIDKey, pk); err != nil {
			return transfersWalk{}, fmt.Errorf("%w: %w", errInvalidTransfersCursor, err)
		}
	default:
		return transfersWalk{}, errInvalidTransfersCursor
	}

	return w, nil
}

func (q Querier) walkTransfers(ctx context.Context, w transfersWalk, page sdkquery.PageRequest) ([]types.TransferState, []byte, error) {
	order := collections.OrderAscending
	if page.Reverse {
		order = collections.OrderDescending
	}

	cursor := func(codec collcodec.KeyCodec[types.ID], id types.ID) ([]byte, error) {
		pk, err := encodeKey(codec, id)
		if err != nil {
			return nil, err
		}

		return transfersCursor(w.states, w.path, pk)
	}

	switch {
	case w.path == transfersHistoryPath:
		start, end := pageRange(nil, w.resume, page.Reverse)
		it, err := q.history.IterateRaw(ctx, start, end, order)
		if err != nil {
			return nil, nil, err
		}

		return collect(it, page.Offset, page.Limit,
			func(id types.ID, _ collections.NoValue) (types.TransferState, error) { return q.GetTransfer(ctx, id) },
			func(id types.ID) ([]byte, error) { return cursor(chronoIDKey, id) },
		)
	case w.path == transfersOwnerPath && len(w.states) > 1:
		start, end := pageRange(w.owner, w.resume, page.Reverse)
		it, err := q.transfers.IterateRaw(ctx, start, end, order)
		if err != nil {
			return nil, nil, err
		}

		return collect(it, page.Offset, page.Limit,
			func(id types.ID, _ types.RecordData) (types.TransferState, error) { return q.GetTransfer(ctx, id) },
			func(id types.ID) ([]byte, error) { return cursor(RecordIDKey, id) },
		)
	default:
		// One settlement bucket, restricted to the owner on the owner path. Within
		// an owner it is ordered by height and sequence.
		bucket := append([]byte{w.states[0]}, w.owner...)

		var resume []byte
		if w.resume != nil {
			resume = append([]byte{w.states[0]}, w.resume...)
		}

		start, end := pageRange(bucket, resume, page.Reverse)
		it, err := q.settlements.IterateRaw(ctx, start, end, order)
		if err != nil {
			return nil, nil, err
		}

		return collect(it, page.Offset, page.Limit,
			func(id types.SettlementID, s types.SettlementData) (types.TransferState, error) {
				return q.settledTransfer(ctx, id, s)
			},
			func(id types.SettlementID) ([]byte, error) { return cursor(RecordIDKey, *id.GetID()) },
		)
	}
}

// settledTransfer joins one canonical settlement with its immutable record.
func (q Querier) settledTransfer(ctx context.Context, id types.SettlementID, s types.SettlementData) (types.TransferState, error) {
	tid := *id.GetID()

	data, err := q.transfers.Get(ctx, tid)
	if err != nil {
		return types.TransferState{}, fmt.Errorf("record of settlement %s: %w", tid.String(), err)
	}

	return types.TransferState{
		ID:         tid,
		Data:       data,
		Settlement: s,
		Status:     id.Status,
	}, nil
}

// collect returns up to limit transfers from it after skipping offset entries,
// and the cursor of the first entry that did not fit.
func collect[K, V any](
	it collections.Iterator[K, V],
	offset, limit uint64,
	load func(K, V) (types.TransferState, error),
	cursor func(K) ([]byte, error),
) (transfers []types.TransferState, next []byte, err error) {
	defer func() {
		err = errors.Join(err, it.Close())
	}()

	transfers = make([]types.TransferState, 0)
	for ; it.Valid(); it.Next() {
		var kv collections.KeyValue[K, V]
		if kv, err = it.KeyValue(); err != nil {
			return nil, nil, err
		}

		if offset > 0 {
			offset--
			continue
		}

		if uint64(len(transfers)) == limit {
			next, err = cursor(kv.Key)
			return transfers, next, err
		}

		var t types.TransferState
		if t, err = load(kv.Key, kv.Value); err != nil {
			return nil, nil, err
		}

		transfers = append(transfers, t)
	}

	return transfers, nil, nil
}

// pageRange bounds a raw iteration to prefix and, when resuming, to the
// inclusive resume key.
func pageRange(prefix, resume []byte, reverse bool) (start, end []byte) {
	if len(prefix) > 0 {
		start, end = prefix, storetypes.PrefixEndBytes(prefix)
	}

	switch {
	case resume == nil:
		return start, end
	case reverse:
		// Sink keys are length-delimited, so no stored key extends resume and
		// the exclusive end resume||0x00 keeps exactly resume.
		return start, append(bytes.Clone(resume), 0)
	default:
		return resume, end
	}
}

// transfersCursor encodes the cursor that resumes the walk at pk.
func transfersCursor(states []byte, path byte, pk []byte) ([]byte, error) {
	// util/query prefixes each part with a single length byte.
	if len(pk) > math.MaxUint8 {
		return nil, fmt.Errorf("transfer key of %d bytes exceeds the pagination key limit", len(pk))
	}

	var unsolicited []byte
	if path != 0 {
		unsolicited = []byte{path}
	}

	return query.EncodePaginationKey(states, states[:1], pk, unsolicited)
}

func encodeKey[K any](codec collcodec.KeyCodec[K], key K) ([]byte, error) {
	buf := make([]byte, codec.Size(key))

	n, err := codec.Encode(buf, key)
	if err != nil {
		return nil, err
	}

	return buf[:n], nil
}

func decodeExact[K any](codec collcodec.KeyCodec[K], key []byte) (K, error) {
	n, value, err := codec.Decode(key)
	if err == nil && n != len(key) {
		err = errors.New("trailing key bytes")
	}

	return value, err
}
