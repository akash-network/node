package simulation

import (
	"bytes"
	"fmt"
	"math/rand"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"
	deposit "pkg.akt.dev/go/node/types/deposit/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/node/deployment/v1beta4"

	sdlv1 "pkg.akt.dev/go/sdl"

	appparams "pkg.akt.dev/node/app/params"
	testsim "pkg.akt.dev/node/testutil/sim"
	"pkg.akt.dev/node/x/deployment/keeper"
)

// Simulation operation weights constants
const (
	OpWeightMsgCreateDeployment = "op_weight_msg_create_deployment" // nolint gosec
	OpWeightMsgUpdateDeployment = "op_weight_msg_update_deployment" // nolint gosec
	OpWeightMsgCloseDeployment  = "op_weight_msg_close_deployment"  // nolint gosec
	OpWeightMsgCloseGroup       = "op_weight_msg_close_group"       // nolint gosec
)

// WeightedOperations returns all the operations from the module with their respective weights
func WeightedOperations(
	appParams simtypes.AppParams, _ codec.JSONCodec, ak govtypes.AccountKeeper,
	bk bankkeeper.Keeper, k keeper.IKeeper) simulation.WeightedOperations {
	var (
		weightMsgCreateDeployment int
		weightMsgUpdateDeployment int
		weightMsgCloseDeployment  int
		weightMsgCloseGroup       int
	)

	appParams.GetOrGenerate(
		OpWeightMsgCreateDeployment, &weightMsgCreateDeployment, nil, func(_ *rand.Rand) {
			weightMsgCreateDeployment = appparams.DefaultWeightMsgCreateDeployment
		},
	)

	appParams.GetOrGenerate(
		OpWeightMsgUpdateDeployment, &weightMsgUpdateDeployment, nil, func(_ *rand.Rand) {
			weightMsgUpdateDeployment = appparams.DefaultWeightMsgUpdateDeployment
		},
	)

	appParams.GetOrGenerate(
		OpWeightMsgCloseDeployment, &weightMsgCloseDeployment, nil, func(_ *rand.Rand) {
			weightMsgCloseDeployment = appparams.DefaultWeightMsgCloseDeployment
		},
	)

	appParams.GetOrGenerate(
		OpWeightMsgCloseGroup, &weightMsgCloseGroup, nil, func(_ *rand.Rand) {
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
func SimulateMsgCreateDeployment(ak govtypes.AccountKeeper, bk bankkeeper.Keeper, k keeper.IKeeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accounts)

		params := k.GetParams(ctx)

		dID := v1.DeploymentID{
			Owner: simAccount.Address.String(),
			DSeq:  uint64(ctx.BlockHeight()), // nolint gosec
		}

		_, found := k.GetDeployment(ctx, dID)
		if found {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCreateDeployment{}).Type(), "no deployment found"), nil, nil
		}

		sdl, readError := sdlv1.ReadFile("../x/deployment/testdata/deployment.yaml")
		if readError != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCreateDeployment{}).Type(), "unable to read config file"),
				nil, readError
		}

		groupSpecs, groupErr := sdl.DeploymentGroups()
		if groupErr != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCreateDeployment{}).Type(), "unable to read groups"), nil, groupErr
		}
		sdlSum, err := sdl.Version()
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCreateDeployment{}).Type(), "error parsing deployment version sum"),
				nil, err
		}

		depositAmount := params.MinDeposits[0]
		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		if spendable.AmountOf(depositAmount.Denom).LT(depositAmount.Amount.MulRaw(2)) {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCreateDeployment{}).Type(), "out of money"), nil, nil
		}
		spendable = spendable.Sub(depositAmount)

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCreateDeployment{}).Type(), "unable to generate fees"), nil, err
		}

		msg := v1beta4.NewMsgCreateDeployment(dID, make([]v1beta4.GroupSpec, 0, len(groupSpecs)), sdlSum, deposit.Deposit{
			Amount:  depositAmount,
			Sources: deposit.Sources{deposit.SourceBalance},
		})

		msg.Groups = append(msg.Groups, groupSpecs...)

		txGen := sdkutil.MakeEncodingConfig().TxConfig
		tx, err := simtestutil.GenSignedMockTx(
			r,
			txGen,
			[]sdk.Msg{msg},
			fees,
			simtestutil.DefaultGenTxGas,
			chainID,
			[]uint64{account.GetAccountNumber()},
			[]uint64{account.GetSequence()},
			simAccount.PrivKey,
		)
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, msg.Type(), "unable to generate mock tx"), nil, err
		}

		_, _, err = app.SimDeliver(txGen.TxEncoder(), tx)
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, msg.Type(), "create deployment - unable to deliver mock tx"), nil, err
		}

		return simtypes.NewOperationMsg(msg, true, ""), nil, nil
	}
}

// SimulateMsgUpdateDeployment generates a MsgUpdate with random values
func SimulateMsgUpdateDeployment(ak govtypes.AccountKeeper, bk bankkeeper.Keeper, k keeper.IKeeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		var deployments []v1.Deployment

		sdl, readError := sdlv1.ReadFile("../x/deployment/testdata/deployment-v2.yaml")
		if readError != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgUpdateDeployment{}).Type(), "unable to read config file"), nil, readError
		}

		sdlSum, err := sdl.Version()
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgUpdateDeployment{}).Type(), "error parsing deployment version sum"),
				nil, err
		}

		k.WithDeployments(ctx, func(deployment v1.Deployment) bool {
			// skip deployments that already have been updated
			if !bytes.Equal(deployment.Hash, sdlSum) {
				deployments = append(deployments, deployment)
			}

			return false
		})

		if len(deployments) == 0 {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgUpdateDeployment{}).Type(), "no deployments found"), nil, nil
		}

		// Get random deployment
		deployment := deployments[testsim.RandIdx(r, len(deployments)-1)]

		if deployment.State != v1.DeploymentActive {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgUpdateDeployment{}).Type(), "deployment closed"), nil, nil
		}

		owner, convertErr := sdk.AccAddressFromBech32(deployment.ID.Owner)
		if convertErr != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgUpdateDeployment{}).Type(), "error while converting address"), nil, convertErr
		}

		simAccount, found := simtypes.FindAccount(accounts, owner)
		if !found {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgUpdateDeployment{}).Type(), "unable to find deployment with given id"),
				nil, fmt.Errorf("deployment with %s not found", deployment.ID.Owner)
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgUpdateDeployment{}).Type(), "unable to generate fees"), nil, err
		}

		msg := v1beta4.NewMsgUpdateDeployment(deployment.ID, sdlSum)

		txGen := sdkutil.MakeEncodingConfig().TxConfig
		tx, err := simtestutil.GenSignedMockTx(
			r,
			txGen,
			[]sdk.Msg{msg},
			fees,
			simtestutil.DefaultGenTxGas,
			chainID,
			[]uint64{account.GetAccountNumber()},
			[]uint64{account.GetSequence()},
			simAccount.PrivKey,
		)
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, msg.Type(), "unable to generate mock tx"), nil, err
		}

		_, _, err = app.SimDeliver(txGen.TxEncoder(), tx)
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, msg.Type(), "update deployment - unable to deliver mock tx"), nil, err
		}

		return simtypes.NewOperationMsg(msg, true, ""), nil, nil
	}
}

// SimulateMsgCloseDeployment generates a MsgClose with random values
func SimulateMsgCloseDeployment(ak govtypes.AccountKeeper, bk bankkeeper.Keeper, k keeper.IKeeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		var deployments []v1.Deployment

		k.WithDeployments(ctx, func(deployment v1.Deployment) bool {
			if deployment.State == v1.DeploymentActive {
				deployments = append(deployments, deployment)
			}

			return false
		})

		if len(deployments) == 0 {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCloseDeployment{}).Type(), "no deployments found"), nil, nil
		}

		// Get random deployment
		deployment := deployments[testsim.RandIdx(r, len(deployments)-1)]

		owner, convertErr := sdk.AccAddressFromBech32(deployment.ID.Owner)
		if convertErr != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCloseDeployment{}).Type(), "error while converting address"), nil, convertErr
		}

		simAccount, found := simtypes.FindAccount(accounts, owner)
		if !found {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCloseDeployment{}).Type(), "unable to find deployment"), nil,
				fmt.Errorf("deployment with %s not found", deployment.ID.Owner)
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCloseDeployment{}).Type(), "unable to generate fees"), nil, err
		}

		msg := v1beta4.NewMsgCloseDeployment(deployment.ID)

		txGen := sdkutil.MakeEncodingConfig().TxConfig
		tx, err := simtestutil.GenSignedMockTx(
			r,
			txGen,
			[]sdk.Msg{msg},
			fees,
			simtestutil.DefaultGenTxGas,
			chainID,
			[]uint64{account.GetAccountNumber()},
			[]uint64{account.GetSequence()},
			simAccount.PrivKey,
		)
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, msg.Type(), "unable to generate mock tx"), nil, err
		}

		_, _, err = app.SimDeliver(txGen.TxEncoder(), tx)
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, msg.Type(), "close deployment - unable to deliver mock tx"), nil, err
		}

		return simtypes.NewOperationMsg(msg, true, ""), nil, nil
	}
}

// SimulateMsgCloseGroup generates a MsgCloseGroup for a random deployment
func SimulateMsgCloseGroup(ak govtypes.AccountKeeper, bk bankkeeper.Keeper, k keeper.IKeeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		var deployments []v1.Deployment

		k.WithDeployments(ctx, func(deployment v1.Deployment) bool {
			if deployment.State == v1.DeploymentActive {
				deployments = append(deployments, deployment)
			}

			return false
		})

		if len(deployments) == 0 {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCloseGroup{}).Type(), "no deployments found"), nil, nil
		}

		// Get random deployment
		deployment := deployments[testsim.RandIdx(r, len(deployments)-1)]

		owner, convertErr := sdk.AccAddressFromBech32(deployment.ID.Owner)
		if convertErr != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCloseGroup{}).Type(), "error while converting address"), nil, convertErr
		}

		simAccount, found := simtypes.FindAccount(accounts, owner)
		if !found {
			err := fmt.Errorf("deployment with %s not found", deployment.ID.Owner)
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCloseGroup{}).Type(), err.Error()), nil, err
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCloseGroup{}).Type(), "unable to generate fees"), nil, err
		}

		// Select Group to close
		groups := k.GetGroups(ctx, deployment.ID)
		if len(groups) < 1 {
			// No groups to close
			err := fmt.Errorf("no groups for deployment ID: %v", deployment.ID)
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCloseGroup{}).Type(), err.Error()), nil, err
		}
		group := groups[testsim.RandIdx(r, len(groups)-1)]
		if group.State == v1beta4.GroupClosed {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCloseGroup{}).Type(), "group already closed"), nil, nil
		}

		msg := v1beta4.NewMsgCloseGroup(group.ID)

		err = msg.ValidateBasic()
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&v1beta4.MsgCloseGroup{}).Type(), "msg validation failure"), nil, err
		}

		txGen := sdkutil.MakeEncodingConfig().TxConfig
		tx, err := simtestutil.GenSignedMockTx(
			r,
			txGen,
			[]sdk.Msg{msg},
			fees,
			simtestutil.DefaultGenTxGas,
			chainID,
			[]uint64{account.GetAccountNumber()},
			[]uint64{account.GetSequence()},
			simAccount.PrivKey,
		)
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, msg.Type(), "close group - unable to generate mock tx"), nil, err
		}

		_, _, err = app.SimDeliver(txGen.TxEncoder(), tx)
		if err != nil {
			err = fmt.Errorf("%w: %s: msg delivery error closing group: %v", err, v1.ModuleName, group.ID)
			return simtypes.NoOpMsg(v1.ModuleName, msg.Type(), err.Error()), nil, err
		}
		return simtypes.NewOperationMsg(msg, true, "submitting MsgCloseGroup"), nil, nil
	}
}
