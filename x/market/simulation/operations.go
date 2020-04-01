package simulation

import (
	"fmt"
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
	ptypes "github.com/ovrclk/akash/x/provider/types"
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
func SimulateMsgCreateBid(ak stakingtypes.AccountKeeper, ks keepers.Keepers) simulation.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context,
		accounts []simulation.Account, chainID string,
	) (simulation.OperationMsg, []simulation.FutureOperation, error) {
		var orders []types.Order

		ks.Market.WithOrders(ctx, func(order types.Order) bool {
			if order.State == types.OrderOpen {
				orders = append(orders, order)
			}
			return false
		})

		if len(orders) == 0 {
			return simulation.NoOpMsg(types.ModuleName), nil, nil
		}

		// Get random order
		i := r.Intn(len(orders))
		order := orders[i]

		var providers []ptypes.Provider
		ks.Provider.WithProviders(ctx, func(provider ptypes.Provider) bool {
			providers = append(providers, provider)
			return false
		})

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
func SimulateMsgCloseBid(ak stakingtypes.AccountKeeper, ks keepers.Keepers) simulation.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context,
		accounts []simulation.Account, chainID string,
	) (simulation.OperationMsg, []simulation.FutureOperation, error) {
		var bids []types.Bid

		ks.Market.WithBids(ctx, func(bid types.Bid) bool {
			if bid.State == types.BidMatched {
				bids = append(bids, bid)
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
func SimulateMsgCloseOrder(ak stakingtypes.AccountKeeper, ks keepers.Keepers) simulation.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context,
		accounts []simulation.Account, chainID string,
	) (simulation.OperationMsg, []simulation.FutureOperation, error) {
		var orders []types.Order

		ks.Market.WithOrders(ctx, func(order types.Order) bool {
			if order.State == types.OrderMatched {
				orders = append(orders, order)
			}
			return false
		})

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
