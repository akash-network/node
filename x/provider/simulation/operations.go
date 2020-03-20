package simulation

import (
	"errors"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/simapp/helpers"
	simappparams "github.com/cosmos/cosmos-sdk/simapp/params"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"
	"github.com/ovrclk/akash/x/provider/keeper/keeper"
	"github.com/ovrclk/akash/x/provider/types"
	"github.com/cosmos/cosmos-sdk/x/staking/types"
)

// Simulation operation weights constants
const (
	OpWeightMsgCreate = "op_weight_msg_create"
	OpWeightMsgUpdate = "op_weight_msg_update"
	OpWeightMsgDelete = "op_weight_msg_delete"
)

// WeightedOperations returns all the operations from the module with their respective weights
func WeightedOperations(
	appParams simulation.AppParams, cdc *codec.Codec, k keeper.Keeper
) simlation.WeightedOperations {
	
	var weightMsgCreate int
	var weightMsgUpdate int
	var weightMsgDelete int

	appParams.GetOrGenerate(
		cdc, OpWeightMsgCreate, &weightMsgCreate, nil, func(r *rand.Rand) {
			weightMsgCreate = simappparams.DefaultWeightMsgCreate
		}
	)

	return simulation.WeightedOperations{
		simulation.NewWeightedOperation(
			weightMsgCreate,
			SimulateMsgCreate(k)
		)
	}
}

// SimulateMsgCreate generates a MsgCreate with random values
// nolint:funlen

func SimulateMsgCreate(k keeper.Keeper) simulation.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, 
		accounts []simulation.Account, chainID string) 
	(OperationMsg simulation.OperationMsg, futureOps []simulation.FutureOperation, err error) {
		msg := types.MsgCreate

		denom := k.GetParams(ctx).BondDenom
		amount := ak.GetAccount(ctx, simAccount.Address).GetCoins().AmountOf(denom)
		if !amount.IsPositive() {
			return simulation.NoOpMsg(types.ModuleName), nil, nil
		}

		amount, err := simulation.RandPositiveInt(r, amount)
		if err != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, err
		}

		selfDelegation := sdk.NewCoin(denom, amount)

		account := ak.GetAccount(ctx, simAccount.Address)
		coins := account.SpendableCoins(ctx.BlockTime())

		var fees sdk.Coins
		coins, hasNeg := coins.SafeSub(sdk.Coins{selfDelegation})
		if !hasNeg {
			fees, err = simulation.RandomFees(r, ctx, coins)
			if err != nil {
				return simulation.NoOpMsg(types.ModuleName), nil, err
			}
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

