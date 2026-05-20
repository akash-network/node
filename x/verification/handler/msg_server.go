package handler

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	"pkg.akt.dev/node/v2/x/verification/keeper"
)

type msgServer struct {
	vtypes.UnimplementedMsgServer
	keeper keeper.Keeper
}

func NewMsgServerImpl(k keeper.Keeper) vtypes.MsgServer {
	return &msgServer{keeper: k}
}

var _ vtypes.MsgServer = (*msgServer)(nil)

func (ms msgServer) RegisterAuditor(goCtx context.Context, msg *vtypes.MsgRegisterAuditor) (*vtypes.MsgRegisterAuditorResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	auditor, err := sdk.AccAddressFromBech32(msg.Auditor)
	if err != nil {
		return nil, err
	}

	err = ms.keeper.RegisterAuditor(ctx, msg.Authority, auditor, msg.MaxAttestationTier, msg.MetadataHash)
	if err != nil {
		return nil, err
	}

	return &vtypes.MsgRegisterAuditorResponse{}, nil
}

func (ms msgServer) UpdateParams(goCtx context.Context, msg *vtypes.MsgUpdateParams) (*vtypes.MsgUpdateParamsResponse, error) {
	if ms.keeper.GetAuthority() != msg.Authority {
		return nil, govtypes.ErrInvalidSigner.Wrapf("invalid authority; expected %s, got %s", ms.keeper.GetAuthority(), msg.Authority)
	}
	if err := keeper.ValidateParams(msg.Params); err != nil {
		return nil, err
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	ms.keeper.SetParams(ctx, msg.Params)

	return &vtypes.MsgUpdateParamsResponse{}, nil
}

func (ms msgServer) PostAuditorBond(goCtx context.Context, msg *vtypes.MsgPostAuditorBond) (*vtypes.MsgPostAuditorBondResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	auditor, err := sdk.AccAddressFromBech32(msg.Auditor)
	if err != nil {
		return nil, err
	}

	if err = ms.keeper.PostAuditorBond(ctx, auditor, msg.Amount); err != nil {
		return nil, err
	}

	return &vtypes.MsgPostAuditorBondResponse{}, nil
}

func (ms msgServer) PostProviderBond(goCtx context.Context, msg *vtypes.MsgPostProviderBond) (*vtypes.MsgPostProviderBondResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	provider, err := sdk.AccAddressFromBech32(msg.Provider)
	if err != nil {
		return nil, err
	}

	if err = ms.keeper.PostProviderBond(ctx, provider, msg.Amount); err != nil {
		return nil, err
	}

	return &vtypes.MsgPostProviderBondResponse{}, nil
}

func (ms msgServer) PostSnapshotHash(goCtx context.Context, msg *vtypes.MsgPostSnapshotHash) (*vtypes.MsgPostSnapshotHashResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	provider, err := sdk.AccAddressFromBech32(msg.Provider)
	if err != nil {
		return nil, err
	}

	err = ms.keeper.PostSnapshotHash(ctx, provider, msg.SnapshotHash, msg.ResourceSummary, msg.SnapshotTimestamp)
	if err != nil {
		return nil, err
	}

	return &vtypes.MsgPostSnapshotHashResponse{}, nil
}

func (ms msgServer) OpenAuditEscrow(goCtx context.Context, msg *vtypes.MsgOpenAuditEscrow) (*vtypes.MsgOpenAuditEscrowResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	provider, err := sdk.AccAddressFromBech32(msg.Provider)
	if err != nil {
		return nil, err
	}

	id, err := ms.keeper.OpenAuditEscrow(
		ctx,
		provider,
		msg.RequestedTier,
		msg.RequestedCapabilities,
		msg.Fee,
		msg.ProviderDeposit,
		msg.ExpiresAt,
		msg.MetadataHash,
	)
	if err != nil {
		return nil, err
	}

	return &vtypes.MsgOpenAuditEscrowResponse{AuditEscrowID: id}, nil
}

func (ms msgServer) CancelAuditEscrow(goCtx context.Context, msg *vtypes.MsgCancelAuditEscrow) (*vtypes.MsgCancelAuditEscrowResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	provider, err := sdk.AccAddressFromBech32(msg.Provider)
	if err != nil {
		return nil, err
	}

	if err = ms.keeper.CancelAuditEscrow(ctx, provider, msg.AuditEscrowID); err != nil {
		return nil, err
	}

	return &vtypes.MsgCancelAuditEscrowResponse{}, nil
}

func (ms msgServer) SettleAuditEscrow(goCtx context.Context, msg *vtypes.MsgSettleAuditEscrow) (*vtypes.MsgSettleAuditEscrowResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	err := ms.keeper.SettleAuditEscrow(ctx, msg.Authority, msg.AuditEscrowID, msg.Reason, msg.FaultAttribution, msg.EvidenceHash)
	if err != nil {
		return nil, err
	}

	return &vtypes.MsgSettleAuditEscrowResponse{}, nil
}

func (ms msgServer) SubmitAttestation(goCtx context.Context, msg *vtypes.MsgSubmitAttestation) (*vtypes.MsgSubmitAttestationResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	provider, auditor, err := parseProviderAuditor(msg.Provider, msg.Auditor)
	if err != nil {
		return nil, err
	}

	err = ms.keeper.SubmitAttestation(
		ctx,
		provider,
		auditor,
		msg.Tier,
		msg.Capabilities,
		msg.EvidenceHash,
		msg.Fee,
		msg.Deposit,
		msg.AuditEscrowID,
	)
	if err != nil {
		return nil, err
	}

	return &vtypes.MsgSubmitAttestationResponse{}, nil
}

func (ms msgServer) ResolveDiscrepancy(goCtx context.Context, msg *vtypes.MsgResolveDiscrepancy) (*vtypes.MsgResolveDiscrepancyResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	err := ms.keeper.ResolveDiscrepancy(
		ctx,
		msg.Authority,
		msg.DiscrepancyID,
		msg.VindicatedAuditor,
		msg.SlashAuditorA,
		msg.SlashAuditorB,
		msg.Reason,
		msg.FaultAttribution,
		msg.EvidenceHash,
	)
	if err != nil {
		return nil, err
	}

	return &vtypes.MsgResolveDiscrepancyResponse{}, nil
}

func parseProviderAuditor(provider, auditor string) (sdk.AccAddress, sdk.AccAddress, error) {
	providerAddr, err := sdk.AccAddressFromBech32(provider)
	if err != nil {
		return nil, nil, err
	}
	auditorAddr, err := sdk.AccAddressFromBech32(auditor)
	if err != nil {
		return nil, nil, err
	}
	return providerAddr, auditorAddr, nil
}
