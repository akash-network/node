package simulation

import (
	"fmt"
	"math/rand"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/simapp/helpers"
	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/cosmos/cosmos-sdk/x/simulation"
	simappparams "github.com/ovrclk/akash/app/params"
	keepers "github.com/ovrclk/akash/x/market/handler"
	"github.com/ovrclk/akash/x/market/types"
)

// Simulation operation weights constants
const (
	OpWeightMsgCreateBid  = "op_weight_msg_create_bid"
	OpWeightMsgCloseBid   = "op_weight_msg_close_bid"
	OpWeightMsgCloseOrder = "op_weight_msg_close_order"
)

// WeightedOperations returns all the operations from the module with their respective weights
func WeightedOperations(
	appParams simulation.AppParams, cdc *codec.Codec, ak govtypes.AccountKeeper,
	ks keepers.Keepers) simulation.WeightedOperations {
	var (
		weightMsgCreateBid  int
		weightMsgCloseBid   int
		weightMsgCloseOrder int
	)

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
		simulation.NewWeightedOperation(
			weightMsgCloseBid,
			SimulateMsgCloseBid(ak, ks),
		),
		simulation.NewWeightedOperation(
			weightMsgCloseOrder,
			SimulateMsgCloseOrder(ak, ks),
		),
	}
}

// SimulateMsgCreateBid generates a MsgCreateBid with random values
func SimulateMsgCreateBid(ak govtypes.AccountKeeper, ks keepers.Keepers) simulation.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simulation.Account,
		chainID string) (simulation.OperationMsg, []simulation.FutureOperation, error) {
		orders := getOrdersWithState(ctx, ks, types.OrderOpen)
		if len(orders) == 0 {
			return simulation.NoOpMsg(types.ModuleName), nil, nil
		}

		// Get random order
		i := r.Intn(len(orders))
		order := orders[i]

		providers := getProviders(ctx, ks)

		if len(providers) == 0 {
			return simulation.NoOpMsg(types.ModuleName), nil, nil
		}

		// Get random deployment
		i = r.Intn(len(providers))
		provider := providers[i]

		simAccount, found := simulation.FindAccount(accounts, provider.Owner)
		if !found {
			return simulation.NoOpMsg(types.ModuleName), nil, fmt.Errorf("provider with %s not found", provider.Owner)
		}

		if provider.Owner.Equals(order.Owner) {
			return simulation.NoOpMsg(types.ModuleName), nil, nil
		}

		account := ak.GetAccount(ctx, simAccount.Address)

		fees, err := simulation.RandomFees(r, ctx, account.SpendableCoins(ctx.BlockTime()))
		if err != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, err
		}

		msg := types.MsgCreateBid{
			Order:    order.OrderID,
			Provider: simAccount.Address,
			Price:    order.Price(),
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

// SimulateMsgCloseBid generates a MsgCloseBid with random values
func SimulateMsgCloseBid(ak govtypes.AccountKeeper, ks keepers.Keepers) simulation.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simulation.Account,
		chainID string) (simulation.OperationMsg, []simulation.FutureOperation, error) {
		var bids []types.Bid

		ks.Market.WithBids(ctx, func(bid types.Bid) bool {
			if bid.State == types.BidMatched {
				lease, ok := ks.Market.GetLease(ctx, types.LeaseID(bid.BidID))
				if ok && lease.State == types.LeaseActive {
					bids = append(bids, bid)
				}
			}

			return false
		})

		if len(bids) == 0 {
			return simulation.NoOpMsg(types.ModuleName), nil, nil
		}

		// Get random bid
		i := r.Intn(len(bids))
		bid := bids[i]

		simAccount, found := simulation.FindAccount(accounts, bid.Provider)
		if !found {
			return simulation.NoOpMsg(types.ModuleName), nil, fmt.Errorf("bid with %s not found", bid.Provider)
		}

		account := ak.GetAccount(ctx, simAccount.Address)

		fees, err := simulation.RandomFees(r, ctx, account.SpendableCoins(ctx.BlockTime()))
		if err != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, err
		}

		msg := types.MsgCloseBid{
			BidID: bid.BidID,
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

// SimulateMsgCloseOrder generates a MsgCloseOrder with random values
func SimulateMsgCloseOrder(ak govtypes.AccountKeeper, ks keepers.Keepers) simulation.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simulation.Account,
		chainID string) (simulation.OperationMsg, []simulation.FutureOperation, error) {
		orders := getOrdersWithState(ctx, ks, types.OrderMatched)
		if len(orders) == 0 {
			return simulation.NoOpMsg(types.ModuleName), nil, nil
		}

		// Get random order
		i := r.Intn(len(orders))
		order := orders[i]

		simAccount, found := simulation.FindAccount(accounts, order.ID().Owner)
		if !found {
			return simulation.NoOpMsg(types.ModuleName), nil, fmt.Errorf("order with %s not found", order.ID().Owner)
		}

		account := ak.GetAccount(ctx, simAccount.Address)

		fees, err := simulation.RandomFees(r, ctx, account.SpendableCoins(ctx.BlockTime()))
		if err != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, err
		}

		msg := types.MsgCloseOrder{
			OrderID: order.OrderID,
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
