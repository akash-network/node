package handler

import (
	"bytes"
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"

	"github.com/akash-network/node/x/deployment/keeper"
)

var _ types.MsgServer = msgServer{}

type msgServer struct {
	deployment  keeper.IKeeper
	market      MarketKeeper
	escrow      EscrowKeeper
	authzKeeper AuthzKeeper
}

// NewServer returns an implementation of the deployment MsgServer interface
// for the provided Keeper.
func NewServer(k keeper.IKeeper, mkeeper MarketKeeper, ekeeper EscrowKeeper, authzKeeper AuthzKeeper) types.MsgServer {
	return &msgServer{deployment: k, market: mkeeper, escrow: ekeeper, authzKeeper: authzKeeper}
}

func (ms msgServer) CreateDeployment(goCtx context.Context, msg *types.MsgCreateDeployment) (*types.MsgCreateDeploymentResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if _, found := ms.deployment.GetDeployment(ctx, msg.ID); found {
		return nil, types.ErrDeploymentExists
	}

	params := ms.deployment.GetParams(ctx)
	if err := params.ValidateDeposit(msg.Deposit); err != nil {
		return nil, err
	}

	deployment := types.Deployment{
		DeploymentID: msg.ID,
		State:        types.DeploymentActive,
		Version:      msg.Version,
		CreatedAt:    ctx.BlockHeight(),
	}

	if err := types.ValidateDeploymentGroups(msg.Groups); err != nil {
		return nil, fmt.Errorf("%w: %s", types.ErrInvalidGroups, err.Error())
	}

	owner, err := sdk.AccAddressFromBech32(msg.ID.Owner)
	if err != nil {
		return &types.MsgCreateDeploymentResponse{}, err
	}

	depositor, err := sdk.AccAddressFromBech32(msg.Depositor)
	if err != nil {
		return &types.MsgCreateDeploymentResponse{}, err
	}

	if err = ms.authorizeDeposit(ctx, owner, depositor, msg.Deposit); err != nil {
		return nil, err
	}

	groups := make([]types.Group, 0, len(msg.Groups))

	for idx, spec := range msg.Groups {
		groups = append(groups, types.Group{
			GroupID:   types.MakeGroupID(deployment.ID(), uint32(idx+1)),
			State:     types.GroupOpen,
			GroupSpec: spec,
			CreatedAt: ctx.BlockHeight(),
		})
	}

	if err := ms.deployment.Create(ctx, deployment, groups); err != nil {
		return nil, fmt.Errorf("%w: %s", types.ErrInternal, err.Error())
	}

	// create orders
	for _, group := range groups {
		if _, err := ms.market.CreateOrder(ctx, group.ID(), group.GroupSpec); err != nil {
			return &types.MsgCreateDeploymentResponse{}, err
		}
	}

	if err := ms.escrow.AccountCreate(ctx,
		types.EscrowAccountForDeployment(deployment.ID()),
		owner,
		depositor,
		msg.Deposit,
	); err != nil {
		return &types.MsgCreateDeploymentResponse{}, err
	}

	return &types.MsgCreateDeploymentResponse{}, nil
}

func (ms msgServer) authorizeDeposit(ctx sdk.Context, owner, depositor sdk.AccAddress, deposit sdk.Coin) error {
	// if owner is the depositor, then no need to check authorization
	if owner.Equals(depositor) {
		return nil
	}

	// find the DepositDeploymentAuthorization given to the owner by the depositor and check
	// acceptance
	msg := &types.MsgDepositDeployment{Amount: deposit}
	authorization, expiration := ms.authzKeeper.GetCleanAuthorization(ctx, owner, depositor, sdk.MsgTypeURL(msg))
	if authorization == nil {
		return sdkerrors.ErrUnauthorized.Wrap("authorization not found")
	}
	resp, err := authorization.Accept(ctx, msg)
	if err != nil {
		return err
	}
	if resp.Delete {
		err = ms.authzKeeper.DeleteGrant(ctx, owner, depositor, sdk.MsgTypeURL(msg))
	} else if resp.Updated != nil {
		err = ms.authzKeeper.SaveGrant(ctx, owner, depositor, resp.Updated, expiration)
	}
	if err != nil {
		return err
	}
	if !resp.Accept {
		return sdkerrors.ErrUnauthorized
	}

	return nil
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

	owner, err := sdk.AccAddressFromBech32(deployment.ID().Owner)
	if err != nil {
		return &types.MsgDepositDeploymentResponse{}, err
	}

	depositor, err := sdk.AccAddressFromBech32(msg.Depositor)
	if err != nil {
		return &types.MsgDepositDeploymentResponse{}, err
	}

	if err = ms.authorizeDeposit(ctx, owner, depositor, msg.Amount); err != nil {
		return nil, err
	}

	if err := ms.escrow.AccountDeposit(ctx,
		types.EscrowAccountForDeployment(msg.ID),
		depositor,
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

	// If the deployment is not active, do not allow it to be updated
	if deployment.State != types.DeploymentActive {
		return &types.MsgUpdateDeploymentResponse{}, types.ErrDeploymentClosed
	}

	// If the version is not identical do not allow the update, there is nothing to change in this transaction
	if bytes.Equal(msg.Version, deployment.Version) {
		return &types.MsgUpdateDeploymentResponse{}, types.ErrInvalidVersion
	}

	deployment.Version = msg.Version

	if err := ms.deployment.UpdateDeployment(ctx, deployment); err != nil {
		return &types.MsgUpdateDeploymentResponse{}, fmt.Errorf("%w: %s", types.ErrInternal, err.Error())
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
