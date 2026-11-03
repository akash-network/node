package handler

import (
	"context"
	"slices"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

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
	if msg == nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("nil request")
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	sctx := sdk.UnwrapSDKContext(ctx)

	id, err := s.keeper.Deposit(sctx, msg)
	if err != nil {
		return nil, err
	}

	return &types.MsgSinkResponse{ID: id}, nil
}

func (s msgServer) UpdateParams(ctx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if msg == nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("nil request")
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	if msg.Authority != s.keeper.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrap("sink authority must be governance")
	}

	sctx := sdk.UnwrapSDKContext(ctx)

	if err := s.keeper.SetParams(sctx, msg); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, nil
}

func (s msgServer) RequestVoid(ctx context.Context, msg *types.MsgRequestVoid) (*types.MsgRequestVoidResponse, error) {
	if msg == nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("nil request")
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	if msg.Authority != s.keeper.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrap("sink authority must be governance")
	}

	sctx := sdk.UnwrapSDKContext(ctx)

	if err := s.keeper.RequestVoid(sctx, msg); err != nil {
		return nil, err
	}

	return &types.MsgRequestVoidResponse{}, nil
}

func (s msgServer) ReportOutcome(ctx context.Context, msg *types.MsgReportOutcome) (*types.MsgReportOutcomeResponse, error) {
	if msg == nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("nil report")
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	sctx := sdk.UnwrapSDKContext(ctx)

	policy, err := s.keeper.GetCurrentPolicy(sctx)
	if err != nil {
		return nil, err
	}

	if !slices.Contains(policy.Members, msg.Reporter) {
		return nil, types.ErrNotReporter
	}

	if err := s.keeper.ReportOutcome(sctx, msg); err != nil {
		return nil, err
	}

	return &types.MsgReportOutcomeResponse{}, nil
}

func (s msgServer) VetoVoid(ctx context.Context, msg *types.MsgVetoVoid) (*types.MsgVetoVoidResponse, error) {
	if msg == nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("nil veto")
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	sctx := sdk.UnwrapSDKContext(ctx)

	policy, err := s.keeper.GetCurrentPolicy(sctx)
	if err != nil {
		return nil, err
	}

	if msg.Signer != s.keeper.GetAuthority() && !slices.Contains(policy.Members, msg.Signer) {
		return nil, types.ErrNotReporter
	}

	if err := s.keeper.VetoVoid(sctx, msg); err != nil {
		return nil, err
	}

	return &types.MsgVetoVoidResponse{}, nil
}

func (s msgServer) RestartTransfer(ctx context.Context, msg *types.MsgRestartTransfer) (*types.MsgRestartTransferResponse, error) {
	if msg == nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("nil restart")
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	if msg.Authority != s.keeper.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrap("sink authority must be governance")
	}

	sctx := sdk.UnwrapSDKContext(ctx)

	if err := s.keeper.Resolve(sctx, msg); err != nil {
		return nil, err
	}

	return &types.MsgRestartTransferResponse{}, nil
}

func (s msgServer) ConfirmMint(ctx context.Context, msg *types.MsgConfirmMint) (*types.MsgConfirmMintResponse, error) {
	if msg == nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("nil confirmation")
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	if msg.Authority != s.keeper.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrap("sink authority must be governance")
	}

	sctx := sdk.UnwrapSDKContext(ctx)

	if err := s.keeper.Resolve(sctx, msg); err != nil {
		return nil, err
	}

	return &types.MsgConfirmMintResponse{}, nil
}

func (s msgServer) ConfirmVoid(ctx context.Context, msg *types.MsgConfirmVoid) (*types.MsgConfirmVoidResponse, error) {
	if msg == nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("nil confirmation")
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	if msg.Authority != s.keeper.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrap("sink authority must be governance")
	}

	sctx := sdk.UnwrapSDKContext(ctx)

	if err := s.keeper.Resolve(sctx, msg); err != nil {
		return nil, err
	}

	return &types.MsgConfirmVoidResponse{}, nil
}

func (s msgServer) FinalizeRefund(ctx context.Context, msg *types.MsgFinalizeRefund) (*types.MsgFinalizeRefundResponse, error) {
	if msg == nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("nil refund")
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	sctx := sdk.UnwrapSDKContext(ctx)

	if err := s.keeper.FinalizeRefund(sctx, msg); err != nil {
		return nil, err
	}

	return &types.MsgFinalizeRefundResponse{}, nil
}

func (s msgServer) GuardianPause(ctx context.Context, msg *types.MsgGuardianPause) (*types.MsgGuardianPauseResponse, error) {
	if msg == nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("nil request")
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	sctx := sdk.UnwrapSDKContext(ctx)

	p, err := s.keeper.GetParams(sctx)
	if err != nil {
		return nil, err
	}

	if p.PauseGuardian == "" || msg.Guardian != p.PauseGuardian {
		return nil, types.ErrNotGuardian
	}

	if err := s.keeper.GuardianPause(sctx, msg); err != nil {
		return nil, err
	}

	return &types.MsgGuardianPauseResponse{}, nil
}
