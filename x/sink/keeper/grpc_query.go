package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	types "pkg.akt.dev/go/node/sink/v1"
)

type Querier struct {
	*keeper
}

var _ types.QueryServer = Querier{}

func (q Querier) Params(ctx context.Context, req *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params, err := q.GetParams(sdkCtx)

	if err != nil {
		return nil, err
	}

	return &types.QueryParamsResponse{Params: params}, nil
}

func (q Querier) Reporters(ctx context.Context, request *types.QueryReportersRequest) (*types.QueryReportersResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (q Querier) Transfer(ctx context.Context, req *types.QueryTransferRequest) (*types.QueryTransferResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "missing request")
	}

	if err := req.ValidateBasic(); err != nil {
		return nil, err
	}

	t, err := q.GetTransfer(ctx, req.ID)
	if err != nil {
		return nil, err
	}

	return &types.QueryTransferResponse{Transfer: t}, nil
}

func (q Querier) Account(ctx context.Context, req *types.QueryAccountRequest) (*types.QueryAccountResponse, error) {
	//if req == nil {
	//	return nil, status.Error(codes.InvalidArgument, "missing request")
	//}
	//
	//if err := req.ValidateBasic(); err != nil {
	//	return nil, err
	//}
	//
	//owner, err := sdk.AccAddressFromBech32(req.Owner)
	//if err != nil {
	//	return nil, err
	//}
	//
	//epoch, err := q.k.quotaEpoch(sdk.UnwrapSDKContext(ctx))
	//if err != nil {
	//	return nil, err
	//}
	//
	//u, err := q.GetAccountUsage(ctx, owner)
	//if err != nil {
	//	return nil, err
	//}
	//
	//u, err = u.At(epoch)
	//if err != nil {
	//	return nil, err
	//}

	return &types.QueryAccountResponse{
		//Usage: u,
		//Epoch: epoch,
	}, nil
}

func (q Querier) State(ctx context.Context, _ *types.QueryStateRequest) (*types.QueryStateResponse, error) {
	result := &types.QueryStateResponse{}
	//var err error
	//
	//sdkCtx := sdk.UnwrapSDKContext(ctx)
	//
	//result.Totals, err = q.totals.Get(ctx)
	//if err != nil {
	//	return nil, err
	//}
	//
	//result.Epoch, err = q.quotaEpoch(cctx)
	//if err != nil {
	//	return nil, err
	//}
	//
	//send, err := q.sendUsage.Get(ctx)
	//if err != nil {
	//	return nil, err
	//}
	//
	//refund, err := q.refundUsage.Get(ctx)
	//if err != nil {
	//	return nil, err
	//}
	//
	//result.SendUsage, err = send.At(result.Epoch)
	//if err != nil {
	//	return nil, err
	//}
	//
	//result.RefundUsage, err = refund.At(result.Epoch)
	//if err != nil {
	//	return nil, err
	//}
	//
	//head, ok, err := q.refundHead(ctx, result.Policy.Version)
	//if err != nil {
	//	return nil, err
	//}
	//
	//if ok {
	//	result.RefundHead = &head
	//}

	return result, nil
}

func (q Querier) Transfers(ctx context.Context, req *types.QueryTransfersRequest) (*types.QueryTransfersResponse, error) {
	//if req == nil {
	//	req = &types.QueryTransfersRequest{}
	//}
	//
	//if err := req.ValidateBasic(); err != nil {
	//	return nil, err
	//}
	//
	//if p := req.Pagination; p != nil && (p.Offset != 0 || p.CountTotal || p.Limit > maxPageLimit) {
	//	return nil, types.ErrInvalidPagination.Wrapf("use key, reverse and limit up to %d; offset and count_total are unsupported", maxPageLimit)
	//}
	//
	//if req.Owner != "" && req.Status == types.StatusInvalid {
	//	return q.ownerTransfers(ctx, req)
	//}
	//
	//var err error
	//domain := HistoryKey.Bytes()
	//var filter []byte
	//decode := exactID(chronoIDKey)
	//if req.Status != types.StatusInvalid {
	//	domain = SettlementsKey.Bytes()
	//	filter, err = SettlementIDKey.ToPrefix(types.SettlementID{
	//		Status: req.Status,
	//		Owner:  req.Owner,
	//	})
	//	if err != nil {
	//		return nil, err
	//	}
	//
	//	decode = func(key []byte) (types.ID, error) {
	//		n, sid, err := SettlementIDKey.Decode(key)
	//		if err == nil && (n != len(key) || sid.Status != req.Status) {
	//			err = status.Error(codes.InvalidArgument, "cursor outside status filter")
	//		}
	//
	//		return *sid.GetID(), err
	//	}
	//}
	//
	//transfers, page, err := q.page(ctx, domain, filter, req.Pagination, decode)
	//if err != nil {
	//	return nil, err
	//}

	return &types.QueryTransfersResponse{
		//Transfers:  transfers,
		//Pagination: page,
	}, nil
}

func (q Querier) Disputes(ctx context.Context, req *types.QueryDisputesRequest) (*types.QueryDisputesResponse, error) {
	//if req == nil {
	//	req = &types.QueryDisputesRequest{}
	//}
	//
	//if err := req.ValidateBasic(); err != nil {
	//	return nil, err
	//}
	//
	//if p := req.Pagination; p != nil && (p.Offset != 0 || p.CountTotal || p.Limit > maxPageLimit) {
	//	return nil, types.ErrInvalidPagination.Wrapf("use key, reverse and limit up to %d; offset and count_total are unsupported", maxPageLimit)
	//}
	//
	//transfers, page, err := q.page(ctx, DisputesKey.Bytes(), nil, req.Pagination, exactID(chronoIDKey))
	//if err != nil {
	//	return nil, err
	//}

	return &types.QueryDisputesResponse{
		//Transfers:  transfers,
		//Pagination: page,
	}, nil
}

func (q Querier) page(ctx context.Context, domain, filter []byte, request *sdkquery.PageRequest, decode func([]byte) (types.ID, error)) ([]types.TransferState, *sdkquery.PageResponse, error) {
	//page := sdkquery.PageRequest{Limit: defaultPageLimit}
	//if request != nil {
	//	page = *request
	//}
	//
	//if page.Limit == 0 {
	//	page.Limit = defaultPageLimit
	//}
	//
	//if len(page.Key) > 0 {
	//	if _, err := decode(page.Key); err != nil {
	//		return nil, nil, status.Error(codes.InvalidArgument, err.Error())
	//	}
	//
	//	if !bytes.HasPrefix(page.Key, filter) {
	//		return nil, nil, status.Error(codes.InvalidArgument, "cursor outside owner or status filter")
	//	}
	//
	//	page.Key = page.Key[len(filter):]
	//}
	//
	//store := prefix.NewStore(sdk.UnwrapSDKContext(ctx).KVStore(q.k.skey), append(bytes.Clone(domain), filter...))
	//if len(page.Key) > 0 && !store.Has(page.Key) {
	//	return nil, nil, status.Error(codes.InvalidArgument, "cursor is absent from the selected index")
	//}
	//
	transfers := make([]types.TransferState, 0)
	//result, err := sdkquery.Paginate(store, &page, func(key, value []byte) error {
	//	id, err := decode(append(bytes.Clone(filter), key...))
	//	if err != nil {
	//		return err
	//	}
	//
	//	t, err := q.k.GetTransfer(ctx, id)
	//	if err != nil {
	//		return err
	//	}
	//
	//	transfers = append(transfers, t)
	//
	//	return nil
	//})
	//if err != nil {
	//	return nil, nil, err
	//}
	//
	//if len(result.NextKey) > 0 {
	//	result.NextKey = append(bytes.Clone(filter), result.NextKey...)
	//}

	return transfers, nil, nil
}

//func exactID(codec collcodec.KeyCodec[types.ID]) func([]byte) (types.ID, error) {
//	return func(key []byte) (types.ID, error) {
//		n, id, err := codec.Decode(key)
//		if err == nil && n != len(key) {
//			err = status.Error(codes.InvalidArgument, "trailing cursor bytes")
//		}
//
//		return id, err
//	}
//}
//
//func (q Querier) ownerTransfers(ctx context.Context, req *types.QueryTransfersRequest) (response *types.QueryTransfersResponse, err error) {
//	page := sdkquery.PageRequest{Limit: defaultPageLimit}
//	if req.Pagination != nil {
//		page = *req.Pagination
//	}
//
//	if page.Limit == 0 {
//		page.Limit = defaultPageLimit
//	}
//
//	ownerPrefix, err := RecordIDKey.ToPrefix(types.ID{Owner: req.Owner})
//	if err != nil {
//		return nil, err
//	}
//
//	var cursor []byte
//	if len(page.Key) > 0 {
//		id, decodeErr := exactID(RecordIDKey)(page.Key)
//		if decodeErr != nil || !bytes.HasPrefix(page.Key, ownerPrefix) {
//			return nil, status.Error(codes.InvalidArgument, "cursor outside owner filter or malformed")
//		}
//
//		if _, lookupErr := q.GetTransfer(ctx, id); lookupErr != nil {
//			if errors.Is(lookupErr, types.ErrTransferNotFound) {
//				return nil, status.Error(codes.InvalidArgument, "cursor is absent from owner settlements")
//			}
//
//			return nil, lookupErr
//		}
//
//		cursor = page.Key[len(ownerPrefix):]
//	}
//
//	iterators := make([]storetypes.Iterator, 0, 5)
//	defer func() {
//		for _, it := range iterators {
//			err = errors.Join(err, it.Close())
//		}
//	}()
//
//	for financial := types.StatusTransferPending; financial <= types.StatusRefunded; financial++ {
//		filter, prefixErr := SettlementIDKey.ToPrefix(types.SettlementID{
//			Status: financial,
//			Owner:  req.Owner,
//		})
//		if prefixErr != nil {
//			return nil, prefixErr
//		}
//
//		domain := append(bytes.Clone(SettlementsKey.Bytes()), filter...)
//		store := prefix.NewStore(sdk.UnwrapSDKContext(ctx).KVStore(q.k.skey), domain)
//		var it storetypes.Iterator
//		if page.Reverse {
//			var end []byte
//			if len(cursor) > 0 {
//				end = append(bytes.Clone(cursor), 0)
//			}
//
//			it = store.ReverseIterator(nil, end)
//		} else {
//			it = store.Iterator(cursor, nil)
//		}
//
//		iterators = append(iterators, it)
//	}
//
//	response = &types.QueryTransfersResponse{
//		Transfers:  make([]types.TransferState, 0),
//		Pagination: &sdkquery.PageResponse{},
//	}
//
//	for {
//		selected := -1
//		for i, it := range iterators {
//			if !it.Valid() {
//				continue
//			}
//
//			if selected == -1 {
//				selected = i
//				continue
//			}
//
//			comparison := bytes.Compare(it.Key(), iterators[selected].Key())
//			if comparison == 0 {
//				return nil, types.ErrInternal.Wrap("multiple canonical owner settlements")
//			}
//
//			if !page.Reverse && comparison < 0 || page.Reverse && comparison > 0 {
//				selected = i
//			}
//		}
//
//		if selected == -1 {
//			return response, nil
//		}
//
//		it := iterators[selected]
//		key := append(bytes.Clone(ownerPrefix), it.Key()...)
//		id, decodeErr := exactID(RecordIDKey)(key)
//		if decodeErr != nil {
//			return nil, decodeErr
//		}
//
//		if uint64(len(response.Transfers)) == page.Limit {
//			response.Pagination.NextKey = key
//			return response, nil
//		}
//
//		transfer, lookupErr := q.k.GetTransfer(ctx, id)
//		if lookupErr != nil {
//			return nil, lookupErr
//		}
//
//		response.Transfers = append(response.Transfers, transfer)
//		it.Next()
//	}
//}
