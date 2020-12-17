package handler

import (
	"bytes"
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
)

var _ types.MsgServer = msgServer{}

type msgServer struct {
	deployment keeper.IKeeper
	market     MarketKeeper
	escrow     EscrowKeeper
}

// NewServer returns an implementation of the deployment MsgServer interface
// for the provided Keeper.
func NewServer(k keeper.IKeeper, mkeeper MarketKeeper, ekeeper EscrowKeeper) types.MsgServer {
	return &msgServer{deployment: k, market: mkeeper, escrow: ekeeper}
}

func (ms msgServer) CreateDeployment(goCtx context.Context, msg *types.MsgCreateDeployment) (*types.MsgCreateDeploymentResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if _, found := ms.deployment.GetDeployment(ctx, msg.ID); found {
		return nil, types.ErrDeploymentExists
	}

	minDeposit := ms.deployment.GetParams(ctx).DeploymentMinDeposit

	if minDeposit.Denom != msg.Deposit.Denom {
		return nil, errors.Wrapf(types.ErrInvalidDeposit, "mininum:%v received:%v", minDeposit, msg.Deposit)
	}
	if minDeposit.Amount.GT(msg.Deposit.Amount) {
		return nil, errors.Wrapf(types.ErrInvalidDeposit, "mininum:%v received:%v", minDeposit, msg.Deposit)
	}

	deployment := types.Deployment{
		DeploymentID: msg.ID,
		State:        types.DeploymentActive,
		Version:      msg.Version,
	}

	if err := types.ValidateDeploymentGroups(msg.Groups); err != nil {
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

	// create orders
	for _, group := range groups {
		if _, err := ms.market.CreateOrder(ctx, group.ID(), group.GroupSpec); err != nil {
			return &types.MsgCreateDeploymentResponse{}, err
		}
	}

	owner, err := sdk.AccAddressFromBech32(deployment.ID().Owner)
	if err != nil {
		return &types.MsgCreateDeploymentResponse{}, err
	}

	if err := ms.escrow.AccountCreate(ctx,
		types.EscrowAccountForDeployment(deployment.ID()),
		owner,
		msg.Deposit,
	); err != nil {
		return &types.MsgCreateDeploymentResponse{}, err
	}

	return &types.MsgCreateDeploymentResponse{}, nil
}

func (ms msgServer) DepositDeployment(goCtx context.Context, msg *types.MsgDepositDeployment) (*types.MsgDepositDeploymentResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	deployment, found := ms.deployment.GetDeployment(ctx, msg.ID)
	if !found {
		return &types.MsgDepositDeploymentResponse{}, types.ErrDeploymentNotFound
	}

	if deployment.State != types.DeploymentActive {
		return &types.MsgDepositDeploymentResponse{}, types.ErrDeploymentClosed
	}

	if err := ms.escrow.AccountDeposit(ctx,
		types.EscrowAccountForDeployment(msg.ID),
		msg.Amount); err != nil {
		return &types.MsgDepositDeploymentResponse{}, err
	}

	return &types.MsgDepositDeploymentResponse{}, nil
}

func (ms msgServer) UpdateDeployment(goCtx context.Context, msg *types.MsgUpdateDeployment) (*types.MsgUpdateDeploymentResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	deployment, found := ms.deployment.GetDeployment(ctx, msg.ID)
	if !found {
		return nil, types.ErrDeploymentNotFound
	}

	if deployment.State != types.DeploymentActive {
		return &types.MsgUpdateDeploymentResponse{}, types.ErrDeploymentClosed
	}

	if !bytes.Equal(msg.Version, deployment.Version) {
		deployment.Version = msg.Version
	}

	if err := ms.deployment.UpdateDeployment(ctx, deployment); err != nil {
		return &types.MsgUpdateDeploymentResponse{}, errors.Wrap(types.ErrInternal, err.Error())
	}

	return &types.MsgUpdateDeploymentResponse{}, nil
}

func (ms msgServer) CloseDeployment(goCtx context.Context, msg *types.MsgCloseDeployment) (*types.MsgCloseDeploymentResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	deployment, found := ms.deployment.GetDeployment(ctx, msg.ID)
	if !found {
		return &types.MsgCloseDeploymentResponse{}, types.ErrDeploymentNotFound
	}

	if deployment.State != types.DeploymentActive {
		return &types.MsgCloseDeploymentResponse{}, types.ErrDeploymentClosed
	}

	if err := ms.escrow.AccountClose(ctx,
		types.EscrowAccountForDeployment(deployment.ID()),
	); err != nil {
		return &types.MsgCloseDeploymentResponse{}, err
	}

	// Update state via escrow hooks.

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
	err = ms.deployment.OnCloseGroup(ctx, group, types.GroupClosed)
	if err != nil {
		return nil, err
	}
	ms.market.OnGroupClosed(ctx, group.ID())

	return &types.MsgCloseGroupResponse{}, nil
}

func (ms msgServer) PauseGroup(goCtx context.Context, msg *types.MsgPauseGroup) (*types.MsgPauseGroupResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	group, found := ms.deployment.GetGroup(ctx, msg.ID)
	if !found {
		return nil, types.ErrGroupNotFound
	}

	// if Group already closed; return the validation error
	err := group.ValidatePausable()
	if err != nil {
		return nil, err
	}

	// Update the Group's state
	err = ms.deployment.OnPauseGroup(ctx, group)
	if err != nil {
		return nil, err
	}
	ms.market.OnGroupClosed(ctx, group.ID())

	return &types.MsgPauseGroupResponse{}, nil
}

func (ms msgServer) StartGroup(goCtx context.Context, msg *types.MsgStartGroup) (*types.MsgStartGroupResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	group, found := ms.deployment.GetGroup(ctx, msg.ID)
	if !found {
		return &types.MsgStartGroupResponse{}, types.ErrGroupNotFound
	}

	err := group.ValidateStartable()
	if err != nil {
		return &types.MsgStartGroupResponse{}, err
	}

	err = ms.deployment.OnStartGroup(ctx, group)
	if err != nil {
		return &types.MsgStartGroupResponse{}, err
	}
	if _, err := ms.market.CreateOrder(ctx, group.ID(), group.GroupSpec); err != nil {
		return &types.MsgStartGroupResponse{}, err
	}

	return &types.MsgStartGroupResponse{}, nil
}
