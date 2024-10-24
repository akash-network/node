package handler

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/audit/v1"

	"pkg.akt.dev/node/x/audit/keeper"
)

type msgServer struct {
	keeper keeper.Keeper
}

// NewMsgServerImpl returns an implementation of the market MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(k keeper.Keeper) types.MsgServer {
	return &msgServer{keeper: k}
}

var _ types.MsgServer = msgServer{}

// SignProviderAttributes defines a method that signs provider attributes
func (ms msgServer) SignProviderAttributes(goCtx context.Context, msg *types.MsgSignProviderAttributes) (*types.MsgSignProviderAttributesResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	auditor, err := sdk.AccAddressFromBech32(msg.Auditor)
	if err != nil {
		return nil, err
	}

	var owner sdk.AccAddress
	if owner, err = sdk.AccAddressFromBech32(msg.Owner); err != nil {
		return nil, err
	}

	provID := types.ProviderID{
		Owner:   owner,
		Auditor: auditor,
	}

	if err = ms.keeper.CreateOrUpdateProviderAttributes(ctx, provID, msg.Attributes); err != nil {
		return nil, err
	}

	return &types.MsgSignProviderAttributesResponse{}, nil
}

// DeleteProviderAttributes defines a method that deletes provider attributes
func (ms msgServer) DeleteProviderAttributes(goCtx context.Context, msg *types.MsgDeleteProviderAttributes) (*types.MsgDeleteProviderAttributesResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	auditor, err := sdk.AccAddressFromBech32(msg.Auditor)
	if err != nil {
		return nil, err
	}

	var owner sdk.AccAddress
	if owner, err = sdk.AccAddressFromBech32(msg.Owner); err != nil {
		return nil, err
	}

	provID := types.ProviderID{
		Owner:   owner,
		Auditor: auditor,
	}

	if err = ms.keeper.DeleteProviderAttributes(ctx, provID, msg.Keys); err != nil {
		return nil, err
	}

	return &types.MsgDeleteProviderAttributesResponse{}, nil
}
