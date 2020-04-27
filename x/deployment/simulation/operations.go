package simulation

import (
	"fmt"
	"math/rand"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/simapp/helpers"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
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
)

// WeightedOperations returns all the operations from the module with their respective weights
func WeightedOperations(
	appParams simtypes.AppParams, cdc *codec.Codec,
	ak govtypes.AccountKeeper, bk govtypes.BankKeeper, k keeper.Keeper) simulation.WeightedOperations {
	var (
		weightMsgCreateDeployment int
		weightMsgUpdateDeployment int
		weightMsgCloseDeployment  int
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

	return simulation.WeightedOperations{
		simulation.NewWeightedOperation(
			weightMsgCreateDeployment,
			SimulateMsgCreateDeployment(ak, bk, k),
		),
		simulation.NewWeightedOperation(
			weightMsgUpdateDeployment,
			SimulateMsgUpdateDeployment(ak, bk, k),
		),
		simulation.NewWeightedOperation(
			weightMsgCloseDeployment,
			SimulateMsgCloseDeployment(ak, bk, k),
		),
	}
}

// SimulateMsgCreateDeployment generates a MsgCreate with random values
func SimulateMsgCreateDeployment(ak govtypes.AccountKeeper, bk govtypes.BankKeeper, k keeper.Keeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accounts)

		dID := types.DeploymentID{
			Owner: simAccount.Address,
			DSeq:  uint64(ctx.BlockHeight()),
		}

		// ensure the provider doesn't exist already
		_, found := k.GetDeployment(ctx, dID)
		if found {
			return simtypes.NoOpMsg(types.ModuleName), nil, nil
		}

		sdl, readError := sdl.ReadFile("../x/deployment/testdata/deployment.yml")
		if readError != nil {
			return simtypes.NoOpMsg(types.ModuleName), nil, readError
		}

		groupSpecs, groupErr := sdl.DeploymentGroups()
		if groupErr != nil {
			return simtypes.NoOpMsg(types.ModuleName), nil, groupErr
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName), nil, err
		}

		msg := types.MsgCreate{
			Owner:  simAccount.Address,
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
			return simtypes.NoOpMsg(types.ModuleName), nil, err
		}

		return simtypes.NewOperationMsg(msg, true, ""), nil, nil
	}
}

// SimulateMsgUpdateDeployment generates a MsgUpdate with random values
func SimulateMsgUpdateDeployment(ak govtypes.AccountKeeper, bk govtypes.BankKeeper, k keeper.Keeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		var deployments []types.Deployment

		k.WithDeployments(ctx, func(deployment types.Deployment) bool {
			deployments = append(deployments, deployment)

			return false
		})

		if len(deployments) == 0 {
			return simtypes.NoOpMsg(types.ModuleName), nil, nil
		}

		// Get random deployment
		i := r.Intn(len(deployments))
		deployment := deployments[i]

		simAccount, found := simtypes.FindAccount(accounts, deployment.ID().Owner)
		if !found {
			return simtypes.NoOpMsg(types.ModuleName), nil, fmt.Errorf("deployment with %s not found", deployment.ID().Owner)
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName), nil, err
		}

		msg := types.MsgUpdate{
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
			return simtypes.NoOpMsg(types.ModuleName), nil, err
		}

		return simtypes.NewOperationMsg(msg, true, ""), nil, nil
	}
}

// SimulateMsgCloseDeployment generates a MsgClose with random values
func SimulateMsgCloseDeployment(ak govtypes.AccountKeeper, bk govtypes.BankKeeper, k keeper.Keeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		var deployments []types.Deployment

		k.WithDeployments(ctx, func(deployment types.Deployment) bool {
			if deployment.State == types.DeploymentActive {
				deployments = append(deployments, deployment)
			}

			return false
		})

		if len(deployments) == 0 {
			return simtypes.NoOpMsg(types.ModuleName), nil, nil
		}

		// Get random deployment
		i := r.Intn(len(deployments))
		deployment := deployments[i]

		simAccount, found := simtypes.FindAccount(accounts, deployment.ID().Owner)
		if !found {
			return simtypes.NoOpMsg(types.ModuleName), nil, fmt.Errorf("deployment with %s not found", deployment.ID().Owner)
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName), nil, err
		}

		msg := types.MsgClose{
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
			return simtypes.NoOpMsg(types.ModuleName), nil, err
		}

		return simtypes.NewOperationMsg(msg, true, ""), nil, nil
	}
}
