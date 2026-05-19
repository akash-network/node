package handler

import (
	"context"

	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	types "pkg.akt.dev/go/node/provider/v1beta4"

	mkeeper "pkg.akt.dev/node/v2/x/market/keeper"
	"pkg.akt.dev/node/v2/x/provider/keeper"
)

var (
	// ErrInternal defines registered error code for internal error
	ErrInternal = errorsmod.Register(types.ModuleName, 10, "internal error")
)

type msgServer struct {
	provider keeper.IKeeper
	market   mkeeper.IKeeper
}

// NewMsgServerImpl returns an implementation of the market MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(k keeper.IKeeper, mk mkeeper.IKeeper) types.MsgServer {
	return &msgServer{provider: k, market: mk}
}

var _ types.MsgServer = msgServer{}

func (ms msgServer) CreateProvider(goCtx context.Context, msg *types.MsgCreateProvider) (*types.MsgCreateProviderResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	owner, _ := sdk.AccAddressFromBech32(msg.Owner)

	if _, ok := ms.provider.Get(ctx, owner); ok {
		return nil, types.ErrProviderExists.Wrapf("id: %s", msg.Owner)
	}

	if err := ms.provider.Create(ctx, types.Provider(*msg)); err != nil {
		return nil, ErrInternal.Wrapf("err: %v", err)
	}

	return &types.MsgCreateProviderResponse{}, nil
}

func (ms msgServer) UpdateProvider(goCtx context.Context, msg *types.MsgUpdateProvider) (*types.MsgUpdateProviderResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	err := msg.ValidateBasic()
	if err != nil {
		return nil, err
	}

	owner, _ := sdk.AccAddressFromBech32(msg.Owner)
	_, found := ms.provider.Get(ctx, owner)
	if !found {
		return nil, types.ErrProviderNotFound.Wrapf("id: %s", msg.Owner)
	}

	if err := ms.provider.Update(ctx, types.Provider(*msg)); err != nil {
		return nil, errorsmod.Wrapf(ErrInternal, "err: %v", err)
	}

	return &types.MsgUpdateProviderResponse{}, nil
}

func (ms msgServer) DeleteProvider(goCtx context.Context, msg *types.MsgDeleteProvider) (*types.MsgDeleteProviderResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	owner, err := sdk.AccAddressFromBech32(msg.Owner)
	if err != nil {
		return nil, err
	}

	if _, ok := ms.provider.Get(ctx, owner); !ok {
		return nil, types.ErrProviderNotFound
	}

	// TODO: cancel leases
	return nil, ErrInternal.Wrap("NOTIMPLEMENTED")
}

func (ms msgServer) OpenProviderMaintenance(goCtx context.Context, msg *types.MsgOpenProviderMaintenance) (*types.MsgOpenProviderMaintenanceResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	provider, err := sdk.AccAddressFromBech32(msg.Provider)
	if err != nil {
		return nil, types.ErrInvalidAddress.Wrap(err.Error())
	}

	if _, ok := ms.provider.Get(ctx, provider); !ok {
		return nil, types.ErrProviderNotFound.Wrapf("id: %s", msg.Provider)
	}

	if err := validateOpenProviderMaintenance(ctx, ms.provider, provider, msg); err != nil {
		return nil, err
	}

	id := ms.provider.AllocateMaintenanceID(ctx)
	record := types.ProviderMaintenanceRecord{
		ID:              id,
		Provider:        msg.Provider,
		MaintenanceType: msg.MaintenanceType,
		StartsAt:        msg.StartsAt,
		ExpectedEndsAt:  msg.ExpectedEndsAt,
		OpenedAt:        ctx.BlockTime(),
		MetadataHash:    msg.MetadataHash,
	}

	if err := ms.provider.SetMaintenance(ctx, record); err != nil {
		return nil, ErrInternal.Wrapf("err: %v", err)
	}

	ms.provider.SetActiveMaintenanceID(ctx, provider, id)

	err = ctx.EventManager().EmitTypedEvent(&types.EventProviderMaintenanceOpened{
		MaintenanceID:   id,
		Provider:        msg.Provider,
		MaintenanceType: msg.MaintenanceType,
		StartsAt:        msg.StartsAt,
		ExpectedEndsAt:  msg.ExpectedEndsAt,
		MetadataHash:    msg.MetadataHash,
	})
	if err != nil {
		return nil, err
	}

	return &types.MsgOpenProviderMaintenanceResponse{MaintenanceID: id}, nil
}

func (ms msgServer) CloseProviderMaintenance(goCtx context.Context, msg *types.MsgCloseProviderMaintenance) (*types.MsgCloseProviderMaintenanceResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	provider, err := sdk.AccAddressFromBech32(msg.Provider)
	if err != nil {
		return nil, types.ErrInvalidAddress.Wrap(err.Error())
	}

	if _, ok := ms.provider.Get(ctx, provider); !ok {
		return nil, types.ErrProviderNotFound.Wrapf("id: %s", msg.Provider)
	}

	record, found := ms.provider.GetMaintenance(ctx, msg.MaintenanceID)
	if !found {
		return nil, sdkerrors.ErrNotFound.Wrap("maintenance record not found")
	}

	if record.Provider != msg.Provider {
		return nil, sdkerrors.ErrUnauthorized.Wrap("maintenance record belongs to another provider")
	}

	if record.ClosedAt != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("maintenance record already closed")
	}

	closedAt := ctx.BlockTime()
	record.ClosedAt = &closedAt

	if err := ms.provider.SetMaintenance(ctx, record); err != nil {
		return nil, ErrInternal.Wrapf("err: %v", err)
	}

	if activeID, ok := ms.provider.GetActiveMaintenanceID(ctx, provider); ok && activeID == msg.MaintenanceID {
		ms.provider.DeleteActiveMaintenanceID(ctx, provider)
	}

	err = ctx.EventManager().EmitTypedEvent(&types.EventProviderMaintenanceClosed{
		MaintenanceID: msg.MaintenanceID,
		Provider:      msg.Provider,
		ClosedAt:      closedAt,
	})
	if err != nil {
		return nil, err
	}

	return &types.MsgCloseProviderMaintenanceResponse{}, nil
}

func (ms msgServer) UpdateParams(goCtx context.Context, req *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if ms.provider.GetAuthority() != req.Authority {
		return nil, govtypes.ErrInvalidSigner.Wrapf("invalid authority; expected %s, got %s", ms.provider.GetAuthority(), req.Authority)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := ms.provider.SetParams(ctx, req.Params); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, nil
}

func validateOpenProviderMaintenance(ctx sdk.Context, k keeper.IKeeper, provider sdk.Address, msg *types.MsgOpenProviderMaintenance) error {
	if msg.MaintenanceType == types.ProviderMaintenanceType_provider_maintenance_type_unspecified {
		return sdkerrors.ErrInvalidRequest.Wrap("maintenance type must be specified")
	}

	if msg.StartsAt.IsZero() {
		return sdkerrors.ErrInvalidRequest.Wrap("starts_at must be set")
	}

	if msg.ExpectedEndsAt.IsZero() {
		return sdkerrors.ErrInvalidRequest.Wrap("expected_ends_at must be set")
	}

	if !msg.ExpectedEndsAt.After(msg.StartsAt) {
		return sdkerrors.ErrInvalidRequest.Wrap("expected_ends_at must be after starts_at")
	}

	params := k.GetParams(ctx)
	if msg.ExpectedEndsAt.Sub(msg.StartsAt) > params.MaintenanceMaxDuration {
		return sdkerrors.ErrInvalidRequest.Wrap("maintenance duration exceeds max duration")
	}

	if msg.StartsAt.After(ctx.BlockTime().Add(params.MaintenanceMaxLookahead)) {
		return sdkerrors.ErrInvalidRequest.Wrap("maintenance starts_at exceeds max lookahead")
	}

	if activeID, ok := k.GetActiveMaintenanceID(ctx, provider); ok {
		record, found := k.GetMaintenance(ctx, activeID)
		if found {
			status := keeper.MaintenanceStatus(ctx.BlockTime(), record)
			if status != types.ProviderMaintenanceStatus_provider_maintenance_status_elapsed &&
				status != types.ProviderMaintenanceStatus_provider_maintenance_status_closed {
				return sdkerrors.ErrConflict.Wrap("provider already has scheduled or active maintenance")
			}
		}

		k.DeleteActiveMaintenanceID(ctx, provider)
	}

	return nil
}
