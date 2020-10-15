package simulation

import (
	"math/rand"

	"github.com/pkg/errors"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/simapp/helpers"
	simappparams "github.com/cosmos/cosmos-sdk/simapp/params"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	appparams "github.com/ovrclk/akash/app/params"
	sdlv1 "github.com/ovrclk/akash/sdl"
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
	appParams simtypes.AppParams, cdc codec.JSONMarshaler, ak govtypes.AccountKeeper,
	bk bankkeeper.Keeper, k keeper.Keeper) simulation.WeightedOperations {
	var (
		weightMsgCreateDeployment int
		weightMsgUpdateDeployment int
		weightMsgCloseDeployment  int
		weightMsgCloseGroup       int
	)

	appParams.GetOrGenerate(
		cdc, OpWeightMsgCreateDeployment, &weightMsgCreateDeployment, nil, func(r *rand.Rand) {
			weightMsgCreateDeployment = appparams.DefaultWeightMsgCreateDeployment
		},
	)

	appParams.GetOrGenerate(
		cdc, OpWeightMsgUpdateDeployment, &weightMsgUpdateDeployment, nil, func(r *rand.Rand) {
			weightMsgUpdateDeployment = appparams.DefaultWeightMsgUpdateDeployment
		},
	)

	appParams.GetOrGenerate(
		cdc, OpWeightMsgCloseDeployment, &weightMsgCloseDeployment, nil, func(r *rand.Rand) {
			weightMsgCloseDeployment = appparams.DefaultWeightMsgCloseDeployment
		},
	)

	appParams.GetOrGenerate(
		cdc, OpWeightMsgCloseGroup, &weightMsgCloseGroup, nil, func(r *rand.Rand) {
			weightMsgCloseGroup = appparams.DefaultWeightMsgCloseGroup
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
		simulation.NewWeightedOperation(
			weightMsgCloseGroup,
			SimulateMsgCloseGroup(ak, bk, k),
		),
	}
}

// SimulateMsgCreateDeployment generates a MsgCreate with random values
func SimulateMsgCreateDeployment(ak govtypes.AccountKeeper, bk bankkeeper.Keeper, k keeper.Keeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accounts)

		dID := types.DeploymentID{
			Owner: simAccount.Address.String(),
			DSeq:  uint64(ctx.BlockHeight()),
		}

		_, found := k.GetDeployment(ctx, dID)
		if found {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCreateDeployment, "no deployment found"), nil, nil
		}

		sdl, readError := sdlv1.ReadFile("../x/deployment/testdata/deployment.yaml")
		if readError != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCreateDeployment, "unable to read config file"),
				nil, readError
		}

		groupSpecs, groupErr := sdl.DeploymentGroups()
		if groupErr != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCreateDeployment, "unable to read groups"), nil, groupErr
		}
		sdlSum, err := sdlv1.Version(sdl)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCreateDeployment, "error parsing deployment version sum"),
				nil, err
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCreateDeployment, "unable to generate fees"), nil, err
		}

		msg := types.NewMsgCreateDeployment(dID, make([]types.GroupSpec, 0, len(groupSpecs)), sdlSum)

		for _, spec := range groupSpecs {
			msg.Groups = append(msg.Groups, *spec)
		}

		txGen := simappparams.MakeEncodingConfig().TxConfig
		tx, err := helpers.GenTx(
			txGen,
			[]sdk.Msg{msg},
			fees,
			helpers.DefaultGenTxGas,
			chainID,
			[]uint64{account.GetAccountNumber()},
			[]uint64{account.GetSequence()},
			simAccount.PrivKey,
		)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, msg.Type(), "unable to generate mock tx"), nil, err
		}

		_, _, err = app.Deliver(tx)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, msg.Type(), "unable to deliver mock tx"), nil, err
		}

		return simtypes.NewOperationMsg(msg, true, ""), nil, nil
	}
}

// SimulateMsgUpdateDeployment generates a MsgUpdate with random values
func SimulateMsgUpdateDeployment(ak govtypes.AccountKeeper, bk bankkeeper.Keeper, k keeper.Keeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		var deployments []types.Deployment

		k.WithDeployments(ctx, func(deployment types.Deployment) bool {
			deployments = append(deployments, deployment)

			return false
		})

		if len(deployments) == 0 {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeUpdateDeployment, "no deployments found"), nil, nil
		}

		// Get random deployment
		i := r.Intn(len(deployments))
		deployment := deployments[i]

		owner, convertErr := sdk.AccAddressFromBech32(deployment.ID().Owner)
		if convertErr != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeUpdateDeployment, "error while converting address"), nil, convertErr
		}

		simAccount, found := simtypes.FindAccount(accounts, owner)
		if !found {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeUpdateDeployment, "unable to find deployment with given id"),
				nil, errors.Errorf("deployment with %s not found", deployment.ID().Owner)
		}

		sdl, readError := sdlv1.ReadFile("../x/deployment/testdata/deployment-v2.yaml")
		if readError != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeUpdateDeployment, "unable to read config file"), nil, readError
		}

		groupSpecs, groupErr := sdl.DeploymentGroups()
		if groupErr != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeUpdateDeployment, "unable to parse groups"), nil, groupErr
		}

		sdlSum, err := sdlv1.Version(sdl)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeUpdateDeployment, "error parsing deployment version sum"),
				nil, err
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeUpdateDeployment, "unable to generate fees"), nil, err
		}

		msg := types.NewMsgUpdateDeployment(deployment.ID(), make([]types.GroupSpec, 0, len(groupSpecs)), sdlSum)

		for _, spec := range groupSpecs {
			msg.Groups = append(msg.Groups, *spec)
		}

		txGen := simappparams.MakeEncodingConfig().TxConfig
		tx, err := helpers.GenTx(
			txGen,
			[]sdk.Msg{msg},
			fees,
			helpers.DefaultGenTxGas,
			chainID,
			[]uint64{account.GetAccountNumber()},
			[]uint64{account.GetSequence()},
			simAccount.PrivKey,
		)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, msg.Type(), "unable to generate mock tx"), nil, err
		}

		_, _, err = app.Deliver(tx)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, msg.Type(), "unable to deliver mock tx"), nil, err
		}

		return simtypes.NewOperationMsg(msg, true, ""), nil, nil
	}
}

// SimulateMsgCloseDeployment generates a MsgClose with random values
func SimulateMsgCloseDeployment(ak govtypes.AccountKeeper, bk bankkeeper.Keeper, k keeper.Keeper) simtypes.Operation {
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
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCloseDeployment, "no deployments found"), nil, nil
		}

		// Get random deployment
		i := r.Intn(len(deployments))
		deployment := deployments[i]

		owner, convertErr := sdk.AccAddressFromBech32(deployment.ID().Owner)
		if convertErr != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCloseDeployment, "error while converting address"), nil, convertErr
		}

		simAccount, found := simtypes.FindAccount(accounts, owner)
		if !found {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCloseDeployment, "unable to find deployment"), nil,
				errors.Errorf("deployment with %s not found", deployment.ID().Owner)
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCloseDeployment, "unable to generate fees"), nil, err
		}

		msg := types.NewMsgCloseDeployment(deployment.ID())

		txGen := simappparams.MakeEncodingConfig().TxConfig
		tx, err := helpers.GenTx(
			txGen,
			[]sdk.Msg{msg},
			fees,
			helpers.DefaultGenTxGas,
			chainID,
			[]uint64{account.GetAccountNumber()},
			[]uint64{account.GetSequence()},
			simAccount.PrivKey,
		)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, msg.Type(), "unable to generate mock tx"), nil, err
		}

		_, _, err = app.Deliver(tx)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, msg.Type(), "unable to deliver mock tx"), nil, err
		}

		return simtypes.NewOperationMsg(msg, true, ""), nil, nil
	}
}

// SimulateMsgCloseGroup generates a MsgCloseGroup for a random deployment
func SimulateMsgCloseGroup(ak govtypes.AccountKeeper, bk bankkeeper.Keeper, k keeper.Keeper) simtypes.Operation {
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
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCloseGroup, "no deplyments found"), nil, nil
		}

		// Get random deployment
		i := r.Intn(len(deployments))
		deployment := deployments[i]

		owner, convertErr := sdk.AccAddressFromBech32(deployment.ID().Owner)
		if convertErr != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCloseGroup, "error while converting address"), nil, convertErr
		}

		simAccount, found := simtypes.FindAccount(accounts, owner)
		if !found {
			err := errors.Errorf("deployment with %s not found", deployment.ID().Owner)
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCloseGroup, err.Error()), nil, err
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCloseGroup, "unable to generate fees"), nil, err
		}

		// Select Group to close
		groups := k.GetGroups(ctx, deployment.ID())
		if len(groups) < 1 {
			// No groups to close
			err := errors.Errorf("no groups for deployment ID: %v", deployment.ID())
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCloseGroup, err.Error()), nil, err
		}
		i = r.Intn(len(groups))
		group := groups[i]
		if group.State == types.GroupClosed {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCloseGroup, "group already closed"), nil, nil
		}

		msg := types.NewMsgCloseGroup(group.ID())

		err = msg.ValidateBasic()
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, types.MsgTypeCloseGroup, "msg validation failure"), nil, err
		}

		txGen := simappparams.MakeEncodingConfig().TxConfig
		tx, err := helpers.GenTx(
			txGen,
			[]sdk.Msg{msg},
			fees,
			helpers.DefaultGenTxGas,
			chainID,
			[]uint64{account.GetAccountNumber()},
			[]uint64{account.GetSequence()},
			simAccount.PrivKey,
		)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName, msg.Type(), "unable to generate mock tx"), nil, err
		}

		_, _, err = app.Deliver(tx)
		if err != nil {
			err = errors.Wrapf(err, "%s: msg delivery error closing group: %v", types.ModuleName, group.ID())
			return simtypes.NoOpMsg(types.ModuleName, msg.Type(), err.Error()), nil, err
		}
		return simtypes.NewOperationMsg(msg, true, "submitting MsgCloseGroup"), nil, nil
	}
}
