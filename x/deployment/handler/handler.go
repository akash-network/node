package handler

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/validation"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
)

func NewHandler(keeper keeper.Keeper, mkeeper MarketKeeper) sdk.Handler {
	return func(ctx sdk.Context, msg sdk.Msg) sdk.Result {
		switch msg := msg.(type) {
		case types.MsgCreate:
			return handleMsgCreate(ctx, keeper, mkeeper, msg)
		case types.MsgUpdate:
			return handleMsgUpdate(ctx, keeper, mkeeper, msg)
		case types.MsgClose:
			return handleMsgClose(ctx, keeper, mkeeper, msg)
		default:
			errMsg := fmt.Sprintf("Unrecognized nameservice Msg type: %v", msg.Type())
			return sdk.ErrUnknownRequest(errMsg).Result()
		}
	}
}

func handleMsgCreate(ctx sdk.Context, keeper keeper.Keeper, mkeeper MarketKeeper, msg types.MsgCreate) sdk.Result {

	deployment := types.Deployment{
		DeploymentID: types.DeploymentID{
			Owner: msg.Owner,
			DSeq:  uint64(ctx.BlockHeight()),
		},
		State: types.DeploymentActive,
		// TODO: version
		// Version: sdk.Address.Bytes(),
	}

	if _, found := keeper.GetDeployment(ctx, deployment.ID()); found {
		return types.ErrDeploymentExists().Result()
	}

	if err := validation.ValidateDeploymentGroups(msg.Groups); err != nil {
		return sdk.ErrInternal(err.Error()).Result()
	}

	groups := make([]types.Group, 0, len(msg.Groups))

	for idx, spec := range msg.Groups {
		groups = append(groups, types.Group{
			GroupID:   types.MakeGroupID(deployment.ID(), uint32(idx+1)),
			State:     types.GroupOpen,
			GroupSpec: spec,
		})
	}

	keeper.Create(ctx, deployment, groups)

	return sdk.Result{
		Events: ctx.EventManager().Events(),
	}
}

func handleMsgUpdate(ctx sdk.Context, keeper keeper.Keeper, mkeeper MarketKeeper, msg types.MsgUpdate) sdk.Result {
	deployment, found := keeper.GetDeployment(ctx, msg.ID)
	if !found {
		return types.ErrDeploymentNotFound().Result()
	}

	// TODO: version
	// deployment.Version = msg.Version

	keeper.UpdateDeployment(ctx, deployment)

	return sdk.Result{
		Events: ctx.EventManager().Events(),
	}
}

func handleMsgClose(ctx sdk.Context, keeper keeper.Keeper, mkeeper MarketKeeper, msg types.MsgClose) sdk.Result {

	deployment, found := keeper.GetDeployment(ctx, msg.ID)
	if !found {
		return types.ErrDeploymentNotFound().Result()
	}

	if deployment.State == types.DeploymentClosed {
		return types.ErrDeploymentClosed().Result()
	}

	deployment.State = types.DeploymentClosed
	keeper.UpdateDeployment(ctx, deployment)

	for _, group := range keeper.GetGroups(ctx, deployment.ID()) {
		keeper.OnDeploymentClosed(ctx, group)
		mkeeper.OnGroupClosed(ctx, group.ID())
	}

	return sdk.Result{
		Events: ctx.EventManager().Events(),
	}
}
