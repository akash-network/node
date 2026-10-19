package keeper

import (
	"context"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	types "pkg.akt.dev/go/node/sink/v1"
)

// MsgServer is separate from Keeper to keep transaction authority checks at
// the message boundary while native upgrade code can initialize parameters.
type MsgServer struct{ Keeper }

func (s MsgServer) Sink(ctx context.Context, msg *types.MsgSink) (*types.MsgSinkResponse, error) {
	id, err := s.Keeper.Sink(sdk.UnwrapSDKContext(ctx), msg)
	if err != nil {
		return nil, err
	}
	return &types.MsgSinkResponse{Id: strconv.FormatUint(id, 10)}, nil
}

func (s MsgServer) UpdateParams(ctx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if msg == nil || msg.Authority != s.authority {
		return nil, sdkerrors.ErrUnauthorized.Wrap("sink authority must be governance")
	}
	if err := s.SetParams(sdk.UnwrapSDKContext(ctx), msg.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}

func (s MsgServer) RequestVoid(ctx context.Context, msg *types.MsgRequestVoid) (*types.MsgRequestVoidResponse, error) {
	if msg == nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("nil request")
	}
	if err := s.Keeper.RequestVoid(sdk.UnwrapSDKContext(ctx), msg.Authority, msg.Id); err != nil {
		return nil, err
	}
	return &types.MsgRequestVoidResponse{}, nil
}

func (s MsgServer) GuardianPause(ctx context.Context, msg *types.MsgGuardianPause) (*types.MsgGuardianPauseResponse, error) {
	if err := s.Keeper.GuardianPause(sdk.UnwrapSDKContext(ctx), msg); err != nil {
		return nil, err
	}
	return &types.MsgGuardianPauseResponse{}, nil
}

type QueryServer struct{ Keeper }

func (s MsgServer) ReportOutcome(ctx context.Context, msg *types.MsgReportOutcome) (*types.MsgReportOutcomeResponse, error) {
	if err := s.Keeper.ReportOutcome(sdk.UnwrapSDKContext(ctx), msg); err != nil {
		return nil, err
	}
	return &types.MsgReportOutcomeResponse{}, nil
}
func (s MsgServer) VetoVoid(ctx context.Context, msg *types.MsgVetoVoid) (*types.MsgVetoVoidResponse, error) {
	if err := s.Keeper.VetoVoid(sdk.UnwrapSDKContext(ctx), msg); err != nil {
		return nil, err
	}
	return &types.MsgVetoVoidResponse{}, nil
}
func (s MsgServer) ResolveTransfer(ctx context.Context, msg *types.MsgResolveTransfer) (*types.MsgResolveTransferResponse, error) {
	if err := s.Keeper.ResolveTransfer(sdk.UnwrapSDKContext(ctx), msg); err != nil {
		return nil, err
	}
	return &types.MsgResolveTransferResponse{}, nil
}
func (s MsgServer) FinalizeRefund(ctx context.Context, msg *types.MsgFinalizeRefund) (*types.MsgFinalizeRefundResponse, error) {
	if msg == nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("nil refund")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Executor); err != nil {
		return nil, err
	}
	if err := s.Keeper.FinalizeRefund(sdk.UnwrapSDKContext(ctx), msg.Id); err != nil {
		return nil, err
	}
	return &types.MsgFinalizeRefundResponse{}, nil
}

func (s QueryServer) Sink(ctx context.Context, req *types.QuerySink) (*types.QuerySinkResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "canonical positive transfer ID required")
	}
	id, err := strconv.ParseUint(req.Id, 10, 64)
	if err != nil || id == 0 || strconv.FormatUint(id, 10) != req.Id {
		return nil, status.Error(codes.InvalidArgument, "canonical positive transfer ID required")
	}
	t, ok := s.GetTransfer(sdk.UnwrapSDKContext(ctx), id)
	if !ok {
		return nil, status.Error(codes.NotFound, "sink transfer")
	}
	var public types.QuerySinkResponse_SinkStatus
	switch t.Settlement.Status {
	case types.Status_STATUS_PENDING:
		public = types.QuerySinkResponse_transfer_pending
	case types.Status_STATUS_MINTED:
		public = types.QuerySinkResponse_completed
	case types.Status_STATUS_VOIDED, types.Status_STATUS_REFUNDED:
		public = types.QuerySinkResponse_void
	default:
		return nil, status.Error(codes.Internal, "unknown sink settlement status")
	}
	return &types.QuerySinkResponse{Status: public, Transfer: t}, nil
}

func (s QueryServer) State(ctx context.Context, _ *types.QueryStateRequest) (*types.QueryStateResponse, error) {
	c := sdk.UnwrapSDKContext(ctx)
	return &types.QueryStateResponse{Params: s.GetParams(c), Totals: s.GetTotals(c)}, nil
}

func (s QueryServer) Transfer(ctx context.Context, req *types.QueryTransferRequest) (*types.QueryTransferResponse, error) {
	if req == nil || req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "positive transfer ID required")
	}
	t, ok := s.GetTransfer(sdk.UnwrapSDKContext(ctx), req.Id)
	if !ok {
		return nil, status.Error(codes.NotFound, "sink transfer")
	}
	return &types.QueryTransferResponse{Transfer: t}, nil
}

func (s QueryServer) Account(ctx context.Context, req *types.QueryAccountRequest) (*types.QueryAccountResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "sender required")
	}
	addr, err := sdk.AccAddressFromBech32(req.Sender)
	if err != nil || len(addr) != 20 {
		return nil, status.Error(codes.InvalidArgument, "invalid sender")
	}
	return &types.QueryAccountResponse{Usage: s.GetAccountUsage(sdk.UnwrapSDKContext(ctx), addr)}, nil
}

func (s QueryServer) OpenTransfers(ctx context.Context, req *types.QueryOpenTransfersRequest) (*types.QueryOpenTransfersResponse, error) {
	if req == nil {
		req = &types.QueryOpenTransfersRequest{}
	}
	transfers, next, err := s.Keeper.OpenTransfers(sdk.UnwrapSDKContext(ctx), req.StartAfter, req.Limit)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryOpenTransfersResponse{Transfers: transfers, NextStartAfter: next}, nil
}
