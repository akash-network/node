package handler

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/x/sink/keeper"
)

type msgServer struct {
	keeper keeper.Keeper
}

var _ types.MsgServer = msgServer{}

func NewServer(k keeper.Keeper) types.MsgServer {
	return msgServer{keeper: k}
}

func (s msgServer) Sink(ctx context.Context, msg *types.MsgSink) (*types.MsgSinkResponse, error) {
	id, err := s.keeper.Deposit(sdk.UnwrapSDKContext(ctx), msg)
	if err != nil {
		return nil, err
	}

	return &types.MsgSinkResponse{ID: id}, nil
}

func (s msgServer) UpdateParams(ctx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := s.keeper.UpdateParams(sdk.UnwrapSDKContext(ctx), msg); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, nil
}

func (s msgServer) RequestVoid(ctx context.Context, msg *types.MsgRequestVoid) (*types.MsgRequestVoidResponse, error) {
	if err := s.keeper.RequestVoid(sdk.UnwrapSDKContext(ctx), msg); err != nil {
		return nil, err
	}

	return &types.MsgRequestVoidResponse{}, nil
}

func (s msgServer) ReportOutcome(ctx context.Context, msg *types.MsgReportOutcome) (*types.MsgReportOutcomeResponse, error) {
	if err := s.keeper.ReportOutcome(sdk.UnwrapSDKContext(ctx), msg); err != nil {
		return nil, err
	}

	return &types.MsgReportOutcomeResponse{}, nil
}

func (s msgServer) VetoVoid(ctx context.Context, msg *types.MsgVetoVoid) (*types.MsgVetoVoidResponse, error) {
	if err := s.keeper.VetoVoid(sdk.UnwrapSDKContext(ctx), msg); err != nil {
		return nil, err
	}

	return &types.MsgVetoVoidResponse{}, nil
}

func (s msgServer) RestartTransfer(ctx context.Context, msg *types.MsgRestartTransfer) (*types.MsgRestartTransferResponse, error) {
	if err := s.keeper.Resolve(sdk.UnwrapSDKContext(ctx), msg); err != nil {
		return nil, err
	}

	return &types.MsgRestartTransferResponse{}, nil
}

func (s msgServer) ConfirmMint(ctx context.Context, msg *types.MsgConfirmMint) (*types.MsgConfirmMintResponse, error) {
	if err := s.keeper.Resolve(sdk.UnwrapSDKContext(ctx), msg); err != nil {
		return nil, err
	}

	return &types.MsgConfirmMintResponse{}, nil
}

func (s msgServer) ConfirmVoid(ctx context.Context, msg *types.MsgConfirmVoid) (*types.MsgConfirmVoidResponse, error) {
	if err := s.keeper.Resolve(sdk.UnwrapSDKContext(ctx), msg); err != nil {
		return nil, err
	}

	return &types.MsgConfirmVoidResponse{}, nil
}

func (s msgServer) FinalizeRefund(ctx context.Context, msg *types.MsgFinalizeRefund) (*types.MsgFinalizeRefundResponse, error) {
	if err := s.keeper.FinalizeRefund(sdk.UnwrapSDKContext(ctx), msg); err != nil {
		return nil, err
	}

	return &types.MsgFinalizeRefundResponse{}, nil
}

func (s msgServer) GuardianPause(ctx context.Context, msg *types.MsgGuardianPause) (*types.MsgGuardianPauseResponse, error) {
	if err := s.keeper.GuardianPause(sdk.UnwrapSDKContext(ctx), msg); err != nil {
		return nil, err
	}

	return &types.MsgGuardianPauseResponse{}, nil
}
