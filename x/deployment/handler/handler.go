package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/validation"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
)

// NewHandler returns a handler for "deployment" type messages
func NewHandler(keeper keeper.Keeper, mkeeper MarketKeeper) sdk.Handler {
	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		switch msg := msg.(type) {
		case types.MsgCreateDeployment:
			return handleMsgCreate(ctx, keeper, mkeeper, msg)
		case types.MsgUpdateDeployment:
			return handleMsgUpdate(ctx, keeper, mkeeper, msg)
		case types.MsgCloseDeployment:
			return handleMsgCloseDeployment(ctx, keeper, mkeeper, msg)
		case types.MsgCloseGroup:
			return handleMsgCloseGroup(ctx, keeper, mkeeper, msg)
		default:
			return nil, sdkerrors.ErrUnknownRequest
		}
	}
}

func handleMsgCreate(ctx sdk.Context, keeper keeper.Keeper, _ MarketKeeper, msg types.MsgCreateDeployment) (*sdk.Result, error) {
	if _, found := keeper.GetDeployment(ctx, msg.ID); found {
		return nil, types.ErrDeploymentExists
	}

	deployment := types.Deployment{
		DeploymentID: msg.ID,
		State:        types.DeploymentActive,
		// TODO: version
		// Version: sdk.Address.Bytes(),
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

	if err := keeper.Create(ctx, deployment, groups); err != nil {
		return nil, errors.Wrap(types.ErrInternal, err.Error())
	}

	return &sdk.Result{
		Events: ctx.EventManager().Events(),
	}, nil
}

func handleMsgUpdate(ctx sdk.Context, keeper keeper.Keeper, _ MarketKeeper, msg types.MsgUpdateDeployment) (*sdk.Result, error) {
	deployment, found := keeper.GetDeployment(ctx, msg.ID)
	if !found {
		return nil, types.ErrDeploymentNotFound
	}

	// TODO: version
	// deployment.Version = msg.Version

	if err := keeper.UpdateDeployment(ctx, deployment); err != nil {
		return nil, errors.Wrap(types.ErrInternal, err.Error())
	}

	return &sdk.Result{
		Events: ctx.EventManager().Events(),
	}, nil
}

func handleMsgCloseDeployment(ctx sdk.Context, keeper keeper.Keeper, mkeeper MarketKeeper, msg types.MsgCloseDeployment) (*sdk.Result, error) {

	deployment, found := keeper.GetDeployment(ctx, msg.ID)
	if !found {
		return nil, types.ErrDeploymentNotFound
	}

	if deployment.State == types.DeploymentClosed {
		return nil, types.ErrDeploymentClosed
	}

	deployment.State = types.DeploymentClosed

	if err := keeper.UpdateDeployment(ctx, deployment); err != nil {
		return nil, errors.Wrap(types.ErrInternal, err.Error())
	}

	for _, group := range keeper.GetGroups(ctx, deployment.ID()) {
		keeper.OnDeploymentClosed(ctx, group)
		mkeeper.OnGroupClosed(ctx, group.ID())
	}

	return &sdk.Result{
		Events: ctx.EventManager().Events(),
	}, nil
}

func handleMsgCloseGroup(ctx sdk.Context, keeper keeper.Keeper, mkeeper MarketKeeper, msg types.MsgCloseGroup) (*sdk.Result, error) {
	group, found := keeper.GetGroup(ctx, msg.ID)
	if !found {
		return nil, types.ErrGroupNotFound
	}

	// if Group already closed; return the validation error
	err := group.ValidateClosable()
	if err != nil {
		return nil, err
	}

	// Update the Group's state
	err = keeper.OnCloseGroup(ctx, group)
	if err != nil {
		return nil, err
	}
	mkeeper.OnGroupClosed(ctx, group.ID())

	return &sdk.Result{
		Events: ctx.EventManager().Events(),
	}, nil
}
