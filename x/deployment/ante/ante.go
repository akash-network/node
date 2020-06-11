package ante

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
)

type deploymentDecorator struct {
	keeper keeper.Keeper
}

// NewDecorator returns a decorator for "deployment" type messages
func NewDecorator(keeper keeper.Keeper) sdk.AnteDecorator {
	return deploymentDecorator{
		keeper: keeper,
	}
}

func (dd deploymentDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	for _, msg := range tx.GetMsgs() {
		var err error
		switch msg := msg.(type) {
		case types.MsgCreateDeployment:
			err = handleMsgCreate(ctx, dd.keeper, msg)
		case types.MsgUpdateDeployment:
			err = handleMsgUpdate(ctx, dd.keeper, msg)
		case types.MsgCloseDeployment:
			err = handleMsgClose(ctx, dd.keeper, msg)
		default:
			continue
		}

		if err != nil {
			return ctx, err
		}
	}
	return next(ctx, tx, simulate)
}

func handleMsgCreate(ctx sdk.Context, keeper keeper.Keeper, msg types.MsgCreateDeployment) error {
	if _, found := keeper.GetDeployment(ctx, msg.ID); found {
		return types.ErrDeploymentExists
	}

	return nil
}

func handleMsgUpdate(ctx sdk.Context, keeper keeper.Keeper, msg types.MsgUpdateDeployment) error {
	if _, found := keeper.GetDeployment(ctx, msg.ID); !found {
		return types.ErrDeploymentNotFound
	}

	return nil
}

func handleMsgClose(ctx sdk.Context, keeper keeper.Keeper, msg types.MsgCloseDeployment) error {
	deployment, found := keeper.GetDeployment(ctx, msg.ID)
	if !found {
		return types.ErrDeploymentNotFound
	}

	if deployment.State == types.DeploymentClosed {
		return types.ErrDeploymentClosed
	}

	return nil
}
