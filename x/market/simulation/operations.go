package simulation

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"
	deposit "pkg.akt.dev/go/node/types/deposit/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/go/node/market/v1"
	types "pkg.akt.dev/go/node/market/v1beta5"

	appparams "pkg.akt.dev/node/app/params"
	testsim "pkg.akt.dev/node/testutil/sim"
	keepers "pkg.akt.dev/node/x/market/handler"
)

// Simulation operation weights constants
const (
	OpWeightMsgCreateBid  = "op_weight_msg_create_bid"  // nolint gosec
	OpWeightMsgCloseBid   = "op_weight_msg_close_bid"   // nolint gosec
	OpWeightMsgCloseLease = "op_weight_msg_close_lease" // nolint gosec
)

// WeightedOperations returns all the operations from the module with their respective weights
func WeightedOperations(
	appParams simtypes.AppParams, _ codec.JSONCodec, ks keepers.Keepers) simulation.WeightedOperations {
	var (
		weightMsgCreateBid  int
		weightMsgCloseBid   int
		weightMsgCloseLease int
	)

	appParams.GetOrGenerate(
		OpWeightMsgCreateBid, &weightMsgCreateBid, nil, func(_ *rand.Rand) {
			weightMsgCreateBid = appparams.DefaultWeightMsgCreateBid
		},
	)

	appParams.GetOrGenerate(
		OpWeightMsgCloseBid, &weightMsgCloseBid, nil, func(_ *rand.Rand) {
			weightMsgCloseBid = appparams.DefaultWeightMsgCloseBid
		},
	)

	appParams.GetOrGenerate(
		OpWeightMsgCloseLease, &weightMsgCloseLease, nil, func(_ *rand.Rand) {
			weightMsgCloseLease = appparams.DefaultWeightMsgCloseLease
		},
	)

	return simulation.WeightedOperations{
		simulation.NewWeightedOperation(
			weightMsgCreateBid,
			SimulateMsgCreateBid(ks),
		),
		simulation.NewWeightedOperation(
			weightMsgCloseBid,
			SimulateMsgCloseBid(ks),
		),
		simulation.NewWeightedOperation(
			weightMsgCloseLease,
			SimulateMsgCloseLease(ks),
		),
	}
}

// SimulateMsgCreateBid generates a MsgCreateBid with random values
func SimulateMsgCreateBid(ks keepers.Keepers) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account, chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		orders := getOrdersWithState(ctx, ks, types.OrderOpen)
		if len(orders) == 0 {
			return simtypes.NoOpMsg(v1.ModuleName, (&types.MsgCreateBid{}).Type(), "no open orders found"), nil, nil
		}

		// Get random order
		order := orders[testsim.RandIdx(r, len(orders)-1)]

		providers := getProviders(ctx, ks)

		if len(providers) == 0 {
			return simtypes.NoOpMsg(v1.ModuleName, (&types.MsgCreateBid{}).Type(), "no providers found"), nil, nil
		}

		// Get random deployment
		provider := providers[testsim.RandIdx(r, len(providers)-1)]

		ownerAddr, convertErr := sdk.AccAddressFromBech32(provider.Owner)
		if convertErr != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&types.MsgCreateBid{}).Type(), "error while converting address"), nil, convertErr
		}

		simAccount, found := simtypes.FindAccount(accounts, ownerAddr)
		if !found {
			return simtypes.NoOpMsg(v1.ModuleName, (&types.MsgCreateBid{}).Type(), "unable to find provider"),
				nil, fmt.Errorf("provider with %s not found", provider.Owner)
		}

		if provider.Owner == order.ID.Owner {
			return simtypes.NoOpMsg(v1.ModuleName, (&types.MsgCreateBid{}).Type(), "provider and order owner cannot be same"),
				nil, nil
		}

		depositAmount := minDeposit
		account := ks.Account.GetAccount(ctx, simAccount.Address)
		spendable := ks.Bank.SpendableCoins(ctx, account.GetAddress())

		if spendable.AmountOf(depositAmount.Denom).LT(depositAmount.Amount.MulRaw(2)) {
			return simtypes.NoOpMsg(v1.ModuleName, (&types.MsgCreateBid{}).Type(), "out of money"), nil, nil
		}
		spendable = spendable.Sub(depositAmount)

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&types.MsgCreateBid{}).Type(), "unable to generate fees"), nil, err
		}

		msg := types.NewMsgCreateBid(v1.MakeBidID(order.ID, simAccount.Address), order.Price(), deposit.Deposit{
			Amount:  depositAmount,
			Sources: deposit.Sources{deposit.SourceBalance},
		}, nil)

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
		switch {
		case err == nil:
			return simtypes.NewOperationMsg(msg, true, ""), nil, nil
		case errors.Is(err, v1.ErrBidExists):
			return simtypes.NewOperationMsg(msg, false, ""), nil, nil
		default:
			return simtypes.NoOpMsg(v1.ModuleName, msg.Type(), "unable to deliver mock tx"), nil, err
		}
	}
}

// SimulateMsgCloseBid generates a MsgCloseBid with random values
func SimulateMsgCloseBid(ks keepers.Keepers) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		var bids []types.Bid

		ks.Market.WithBids(ctx, func(bid types.Bid) bool {
			if bid.State == types.BidActive {
				lease, ok := ks.Market.GetLease(ctx, v1.LeaseID(bid.ID))
				if ok && lease.State == v1.LeaseActive {
					bids = append(bids, bid)
				}
			}

			return false
		})

		if len(bids) == 0 {
			return simtypes.NoOpMsg(v1.ModuleName, (&types.MsgCloseBid{}).Type(), "no matched bids found"), nil, nil
		}

		// Get random bid
		bid := bids[testsim.RandIdx(r, len(bids)-1)]

		providerAddr, convertErr := sdk.AccAddressFromBech32(bid.ID.Provider)
		if convertErr != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&types.MsgCloseBid{}).Type(), "error while converting address"), nil, convertErr
		}

		simAccount, found := simtypes.FindAccount(accounts, providerAddr)
		if !found {
			return simtypes.NoOpMsg(v1.ModuleName, (&types.MsgCloseBid{}).Type(), "unable to find bid with provider"),
				nil, fmt.Errorf("bid with %s not found", bid.ID.Provider)
		}

		account := ks.Account.GetAccount(ctx, simAccount.Address)
		spendable := ks.Bank.SpendableCoins(ctx, account.GetAddress())

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(v1.ModuleName, (&types.MsgCloseBid{}).Type(), "unable to generate fees"), nil, err
		}

		msg := types.NewMsgCloseBid(bid.ID, v1.LeaseClosedReasonUnspecified)

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
			return simtypes.NoOpMsg(v1.ModuleName, msg.Type(), "unable to deliver tx"), nil, err
		}

		return simtypes.NewOperationMsg(msg, true, ""), nil, nil
	}
}

// SimulateMsgCloseLease generates a MsgCloseLease with random values
func SimulateMsgCloseLease(_ keepers.Keepers) simtypes.Operation {
	return func(
		r *rand.Rand, // nolint revive
		app *baseapp.BaseApp, // nolint revive
		ctx sdk.Context, // nolint revive
		accounts []simtypes.Account, // nolint revive
		chainID string, // nolint revive
	) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		// leases := getLeasesWithState(ctx, ks, v1.LeaseActive)
		// if len(leases) == 0 {
		// 	return simtypes.NoOpMsg(types.ModuleName, (&types.MsgCloseLease{}).Type(), "no orders with state matched found"), nil, nil
		// }
		//
		// // Get random order
		// lease := leases[testsim.RandIdx(r, len(leases)-1)]
		//
		// ownerAddr, convertErr := sdk.AccAddressFromBech32(lease.ID.Owner)
		// if convertErr != nil {
		// 	return simtypes.NoOpMsg(types.ModuleName, (&types.MsgCloseLease{}).Type(), "error while converting address"), nil, convertErr
		// }
		//
		// simAccount, found := simtypes.FindAccount(accounts, ownerAddr)
		// if !found {
		// 	return simtypes.NoOpMsg(types.ModuleName, (&types.MsgCloseLease{}).Type(), "unable to find lease"),
		// 		nil, fmt.Errorf("lease with %s not found", lease.ID.Owner)
		// }
		//
		// account := ks.Account.GetAccount(ctx, simAccount.Address)
		// spendable := ks.Bank.SpendableCoins(ctx, account.GetAddress())
		//
		// fees, err := simtypes.RandomFees(r, ctx, spendable)
		// if err != nil {
		// 	return simtypes.NoOpMsg(types.ModuleName, (&types.MsgCloseLease{}).Type(), "unable to generate fees"), nil, err
		// }
		//
		// lease.ID.Provider = "3425q"
		//
		// msg := types.NewMsgCloseLease(lease.ID)
		//
		// txGen := moduletestutil.MakeTestEncodingConfig().TxConfig
		//
		// tx, err := simtestutil.GenSignedMockTx(
		// 	r,
		// 	txGen,
		// 	[]sdk.Msg{msg},
		// 	fees,
		// 	simtestutil.DefaultGenTxGas,
		// 	chainID,
		// 	[]uint64{account.GetAccountNumber()},
		// 	[]uint64{account.GetSequence()},
		// 	simAccount.PrivKey,
		// )
		// if err != nil {
		// 	return simtypes.NoOpMsg(types.ModuleName, msg.Type(), "unable to generate mock tx"), nil, err
		// }
		//
		// _, _, err = app.SimDeliver(txGen.TxEncoder(), tx)
		// if err != nil {
		// 	return simtypes.NoOpMsg(types.ModuleName, msg.Type(), "unable to deliver tx"), nil, err
		// }
		//
		// return simtypes.NoOpMsg(types.ModuleName, (&types.MsgCloseLease{}).Type(), "skipping"), nil, nil

		return simtypes.NoOpMsg(v1.ModuleName, (&types.MsgCloseLease{}).Type(), "skipping"), nil, nil
	}
}
