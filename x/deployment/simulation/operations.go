package simulation

import (
	"math/rand"

	"github.com/pkg/errors"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/simapp/helpers"
	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	simappparams "github.com/ovrclk/akash/app/params"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
)

// Simulation operation weights constants
const (
	OpWeightMsgCreateDeployment = "op_weight_msg_create_deployment"
	OpWeightMsgUpdateDeployment = "op_weight_msg_update_deployment"
	OpWeightMsgCloseDeployment  = "op_weight_msg_close_deployment"
	OpWeightMsgCloseGroup       = "op_weight_msg_close_group"
)

// WeightedOperations returns all the operations from the module with their respective weights
func WeightedOperations(
	appParams simulation.AppParams, cdc *codec.Codec,
	ak govtypes.AccountKeeper, k keeper.Keeper) simulation.WeightedOperations {
	var (
		weightMsgCreateDeployment int
		weightMsgUpdateDeployment int
		weightMsgCloseDeployment  int
		weightMsgCloseGroup       int
	)

	appParams.GetOrGenerate(
		cdc, OpWeightMsgCreateDeployment, &weightMsgCreateDeployment, nil, func(r *rand.Rand) {
			weightMsgCreateDeployment = simappparams.DefaultWeightMsgCreateDeployment
		},
	)

	appParams.GetOrGenerate(
		cdc, OpWeightMsgUpdateDeployment, &weightMsgUpdateDeployment, nil, func(r *rand.Rand) {
			weightMsgUpdateDeployment = simappparams.DefaultWeightMsgUpdateDeployment
		},
	)

	appParams.GetOrGenerate(
		cdc, OpWeightMsgCloseDeployment, &weightMsgCloseDeployment, nil, func(r *rand.Rand) {
			weightMsgCloseDeployment = simappparams.DefaultWeightMsgCloseDeployment
		},
	)

	appParams.GetOrGenerate(
		cdc, OpWeightMsgCloseGroup, &weightMsgCloseGroup, nil, func(r *rand.Rand) {
			weightMsgCloseGroup = simappparams.DefaultWeightMsgCloseGroup
		},
	)

	return simulation.WeightedOperations{
		simulation.NewWeightedOperation(
			weightMsgCreateDeployment,
			SimulateMsgCreateDeployment(ak, k),
		),
		simulation.NewWeightedOperation(
			weightMsgUpdateDeployment,
			SimulateMsgUpdateDeployment(ak, k),
		),
		simulation.NewWeightedOperation(
			weightMsgCloseDeployment,
			SimulateMsgCloseDeployment(ak, k),
		),
		simulation.NewWeightedOperation(
			weightMsgCloseGroup,
			SimulateMsgCloseGroup(ak, k),
		),
	}
}

// SimulateMsgCreateDeployment generates a MsgCreate with random values
func SimulateMsgCreateDeployment(ak govtypes.AccountKeeper, k keeper.Keeper) simulation.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simulation.Account,
		chainID string) (simulation.OperationMsg, []simulation.FutureOperation, error) {
		simAccount, _ := simulation.RandomAcc(r, accounts)

		dID := types.DeploymentID{
			Owner: simAccount.Address,
			DSeq:  uint64(ctx.BlockHeight()),
		}

		// ensure the provider doesn't exist already
		_, found := k.GetDeployment(ctx, dID)
		if found {
			return simulation.NoOpMsg(types.ModuleName), nil, nil
		}

		sdl, readError := sdl.ReadFile("../x/deployment/testdata/deployment.yaml")
		if readError != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, readError
		}

		groupSpecs, groupErr := sdl.DeploymentGroups()
		if groupErr != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, groupErr
		}

		account := ak.GetAccount(ctx, simAccount.Address)

		fees, err := simulation.RandomFees(r, ctx, account.SpendableCoins(ctx.BlockTime()))
		if err != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, err
		}

		msg := types.MsgCreateDeployment{
			ID:     dID,
			Groups: make([]types.GroupSpec, 0, len(groupSpecs)),
		}

		for _, spec := range groupSpecs {
			msg.Groups = append(msg.Groups, *spec)
		}

		tx := helpers.GenTx(
			[]sdk.Msg{msg},
			fees,
			helpers.DefaultGenTxGas,
			chainID,
			[]uint64{account.GetAccountNumber()},
			[]uint64{account.GetSequence()},
			simAccount.PrivKey,
		)

		_, _, err = app.Deliver(tx)
		if err != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, err
		}

		return simulation.NewOperationMsg(msg, true, ""), nil, nil
	}
}

// SimulateMsgUpdateDeployment generates a MsgUpdate with random values
func SimulateMsgUpdateDeployment(ak govtypes.AccountKeeper, k keeper.Keeper) simulation.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simulation.Account,
		chainID string) (simulation.OperationMsg, []simulation.FutureOperation, error) {
		var deployments []types.Deployment

		k.WithDeployments(ctx, func(deployment types.Deployment) bool {
			deployments = append(deployments, deployment)

			return false
		})

		if len(deployments) == 0 {
			return simulation.NoOpMsg(types.ModuleName), nil, nil
		}

		// Get random deployment
		i := r.Intn(len(deployments))
		deployment := deployments[i]

		simAccount, found := simulation.FindAccount(accounts, deployment.ID().Owner)
		if !found {
			return simulation.NoOpMsg(types.ModuleName), nil, errors.Errorf("deployment with %s not found", deployment.ID().Owner)
		}

		account := ak.GetAccount(ctx, simAccount.Address)

		fees, err := simulation.RandomFees(r, ctx, account.SpendableCoins(ctx.BlockTime()))
		if err != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, err
		}

		msg := types.MsgUpdateDeployment{
			ID:      deployment.ID(),
			Version: simAccount.Address,
		}

		tx := helpers.GenTx(
			[]sdk.Msg{msg},
			fees,
			helpers.DefaultGenTxGas,
			chainID,
			[]uint64{account.GetAccountNumber()},
			[]uint64{account.GetSequence()},
			simAccount.PrivKey,
		)

		_, _, err = app.Deliver(tx)
		if err != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, err
		}

		return simulation.NewOperationMsg(msg, true, ""), nil, nil
	}
}

// SimulateMsgCloseDeployment generates a MsgClose with random values
func SimulateMsgCloseDeployment(ak govtypes.AccountKeeper, k keeper.Keeper) simulation.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simulation.Account,
		chainID string) (simulation.OperationMsg, []simulation.FutureOperation, error) {
		var deployments []types.Deployment

		k.WithDeployments(ctx, func(deployment types.Deployment) bool {
			if deployment.State == types.DeploymentActive {
				deployments = append(deployments, deployment)
			}

			return false
		})

		if len(deployments) == 0 {
			return simulation.NoOpMsg(types.ModuleName), nil, nil
		}

		// Get random deployment
		i := r.Intn(len(deployments))
		deployment := deployments[i]

		simAccount, found := simulation.FindAccount(accounts, deployment.ID().Owner)
		if !found {
			return simulation.NoOpMsg(types.ModuleName), nil, errors.Errorf("deployment with %s not found", deployment.ID().Owner)
		}

		account := ak.GetAccount(ctx, simAccount.Address)

		fees, err := simulation.RandomFees(r, ctx, account.SpendableCoins(ctx.BlockTime()))
		if err != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, err
		}

		msg := types.MsgCloseDeployment{
			ID: deployment.ID(),
		}

		tx := helpers.GenTx(
			[]sdk.Msg{msg},
			fees,
			helpers.DefaultGenTxGas,
			chainID,
			[]uint64{account.GetAccountNumber()},
			[]uint64{account.GetSequence()},
			simAccount.PrivKey,
		)

		_, _, err = app.Deliver(tx)
		if err != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, err
		}

		return simulation.NewOperationMsg(msg, true, ""), nil, nil
	}
}

// SimulateMsgCloseGroup generates a MsgCloseGroup for a random deployment
func SimulateMsgCloseGroup(ak govtypes.AccountKeeper, k keeper.Keeper) simulation.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simulation.Account,
		chainID string) (simulation.OperationMsg, []simulation.FutureOperation, error) {
		var deployments []types.Deployment

		k.WithDeployments(ctx, func(deployment types.Deployment) bool {
			if deployment.State == types.DeploymentActive {
				deployments = append(deployments, deployment)
			}

			return false
		})

		if len(deployments) == 0 {
			return simulation.NoOpMsg(types.ModuleName), nil, nil
		}

		// Get random deployment
		i := r.Intn(len(deployments))
		deployment := deployments[i]

		simAccount, found := simulation.FindAccount(accounts, deployment.ID().Owner)
		if !found {
			err := errors.Errorf("deployment with %s not found", deployment.ID().Owner)
			return simulation.NoOpMsg(err.Error()), nil, err
		}

		account := ak.GetAccount(ctx, simAccount.Address)

		fees, err := simulation.RandomFees(r, ctx, account.SpendableCoins(ctx.BlockTime()))
		if err != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, err
		}

		// Select Group to close
		groups := k.GetGroups(ctx, deployment.ID())
		if len(groups) < 1 {
			// No groups to close
			err := errors.Errorf("no groups for deployment ID: %v", deployment.ID())
			return simulation.NoOpMsg(err.Error()), nil, err
		}
		i = r.Intn(len(groups))
		group := groups[i]
		if group.State == types.GroupClosed {
			return simulation.NoOpMsg("group already closed"), nil, nil
		}

		msg := types.MsgCloseGroup{
			ID: group.ID(),
		}
		err = msg.ValidateBasic()
		if err != nil {
			return simulation.NoOpMsg(errors.Wrapf(err, "%q close-group msg validation failure", types.ModuleName).Error()), nil, err
		}

		tx := helpers.GenTx(
			[]sdk.Msg{msg},
			fees,
			helpers.DefaultGenTxGas,
			chainID,
			[]uint64{account.GetAccountNumber()},
			[]uint64{account.GetSequence()},
			simAccount.PrivKey,
		)

		_, _, err = app.Deliver(tx)
		if err != nil {
			err = errors.Wrapf(err, "%s: msg delivery error closing group: %v", types.ModuleName, group.ID())
			return simulation.NoOpMsg(err.Error()), nil, err
		}
		return simulation.NewOperationMsg(msg, true, "submitting MsgCloseGroup"), nil, nil
	}
}
