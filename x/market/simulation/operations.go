package simulation

import (
	"math/rand"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/simapp/helpers"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	simappparams "github.com/ovrclk/akash/simapp/params"
	keepers "github.com/ovrclk/akash/x/market/handler"
	"github.com/ovrclk/akash/x/market/types"
)

// Simulation operation weights constants
const (
	OpWeightMsgCreateBid  = "op_weight_msg_create_bid"
	OpWeightMsgCloseBid   = "op_weight_msg_close_bid"
	OpWeightMsgCloseOrder = "op_weight_msg_close_order"
)

// DENOM represents bond denom
const DENOM = "stake"

// WeightedOperations returns all the operations from the module with their respective weights
func WeightedOperations(
	appParams simulation.AppParams, cdc *codec.Codec, ak stakingtypes.AccountKeeper, ks keepers.Keepers,
) simulation.WeightedOperations {

	var weightMsgCreateBid int
	var weightMsgCloseBid int
	var weightMsgCloseOrder int

	appParams.GetOrGenerate(
		cdc, OpWeightMsgCreateBid, &weightMsgCreateBid, nil, func(r *rand.Rand) {
			weightMsgCreateBid = simappparams.DefaultWeightMsgCreateBid
		},
	)

	appParams.GetOrGenerate(
		cdc, OpWeightMsgCloseBid, &weightMsgCloseBid, nil, func(r *rand.Rand) {
			weightMsgCloseBid = simappparams.DefaultWeightMsgCloseBid
		},
	)

	appParams.GetOrGenerate(
		cdc, OpWeightMsgCloseOrder, &weightMsgCloseOrder, nil, func(r *rand.Rand) {
			weightMsgCloseOrder = simappparams.DefaultWeightMsgCloseOrder
		},
	)

	return simulation.WeightedOperations{
		simulation.NewWeightedOperation(
			weightMsgCreateBid,
			SimulateMsgCreateBid(ak, ks),
		),
	}
}

// SimulateMsgCreateBid generates a MsgCreate with random values
func SimulateMsgCreateBid(ak stakingtypes.AccountKeeper, ks keepers.Keepers) simulation.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context,
		accounts []simulation.Account, chainID string,
	) (simulation.OperationMsg, []simulation.FutureOperation, error) {
		simAccount, _ := simulation.RandomAcc(r, accounts)

		amount := ak.GetAccount(ctx, simAccount.Address).GetCoins().AmountOf(DENOM)

		if !amount.IsPositive() {
			return simulation.NoOpMsg(types.ModuleName), nil, nil
		}

		amount, err := simulation.RandPositiveInt(r, amount)
		if err != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, err
		}

		selfDelegation := sdk.NewCoin(DENOM, amount)

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

		orderId := types.OrderID{
			Owner: simAccount.Address,
			DSeq:  rand.Uint64(),
			GSeq:  rand.Uint32(),
			OSeq:  rand.Uint32(),
		}

		msg := types.MsgCreateBid{
			Order:    orderId,
			Provider: simAccount.Address,
			Price:    coins[0],
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
