package handler

import (
	"bytes"
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/validation"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
)

type msgServer struct {
	deployment keeper.Keeper
	market     MarketKeeper
}

// NewMsgServerImpl returns an implementation of the deployment MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(k keeper.Keeper, mkeeper MarketKeeper) types.MsgServer {
	return &msgServer{deployment: k, market: mkeeper}
}

var _ types.MsgServer = msgServer{}

func (ms msgServer) CreateDeployment(goCtx context.Context, msg *types.MsgCreateDeployment) (*types.MsgCreateDeploymentResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if _, found := ms.deployment.GetDeployment(ctx, msg.ID); found {
		return nil, types.ErrDeploymentExists
	}

	deployment := types.Deployment{
		DeploymentID: msg.ID,
		State:        types.DeploymentActive,
		Version:      msg.Version,
	}

	if err := validation.ValidateDeploymentGroups(msg.Groups); err != nil {
		return nil, errors.Wrap(types.ErrInvalidGroups, err.Error())
	}

	groups := make([]types.Group, 0, len(msg.Groups))

	for idx, spec := range msg.Groups {
		groups = append(groups, types.Group{
			GroupID:   types.MakeGroupID(deployment.ID(), uint32(idx+1)),
			State:     types.GroupOpen,
			GroupSpec: spec,
		})
	}

	if err := ms.deployment.Create(ctx, deployment, groups); err != nil {
		return nil, errors.Wrap(types.ErrInternal, err.Error())
	}

	return &types.MsgCreateDeploymentResponse{}, nil
}

func (ms msgServer) UpdateDeployment(goCtx context.Context, msg *types.MsgUpdateDeployment) (*types.MsgUpdateDeploymentResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	deployment, found := ms.deployment.GetDeployment(ctx, msg.ID)
	if !found {
		return nil, types.ErrDeploymentNotFound
	}

	if !bytes.Equal(msg.Version, deployment.Version) {
		deployment.Version = msg.Version
	}

	if err := ms.deployment.UpdateDeployment(ctx, deployment); err != nil {
		return nil, errors.Wrap(types.ErrInternal, err.Error())
	}

	return &types.MsgUpdateDeploymentResponse{}, nil
}

func (ms msgServer) CloseDeployment(goCtx context.Context, msg *types.MsgCloseDeployment) (*types.MsgCloseDeploymentResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	deployment, found := ms.deployment.GetDeployment(ctx, msg.ID)
	if !found {
		return nil, types.ErrDeploymentNotFound
	}

	if deployment.State == types.DeploymentClosed {
		return nil, types.ErrDeploymentClosed
	}

	deployment.State = types.DeploymentClosed

	if err := ms.deployment.UpdateDeployment(ctx, deployment); err != nil {
		return nil, errors.Wrap(types.ErrInternal, err.Error())
	}

	for _, group := range ms.deployment.GetGroups(ctx, deployment.ID()) {
		ms.deployment.OnDeploymentClosed(ctx, group)
		ms.market.OnGroupClosed(ctx, group.ID())
	}

	return &types.MsgCloseDeploymentResponse{}, nil
}

func (ms msgServer) CloseGroup(goCtx context.Context, msg *types.MsgCloseGroup) (*types.MsgCloseGroupResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	group, found := ms.deployment.GetGroup(ctx, msg.ID)
	if !found {
		return nil, types.ErrGroupNotFound
	}

	// if Group already closed; return the validation error
	err := group.ValidateClosable()
	if err != nil {
		return nil, err
	}

	// Update the Group's state
	err = ms.deployment.OnCloseGroup(ctx, group)
	if err != nil {
		return nil, err
	}
	ms.market.OnGroupClosed(ctx, group.ID())

	return &types.MsgCloseGroupResponse{}, nil
}
