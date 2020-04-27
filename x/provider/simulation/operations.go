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
	"github.com/ovrclk/akash/x/provider/config"
	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
)

// Simulation operation weights constants
const (
	OpWeightMsgCreate = "op_weight_msg_create"
	OpWeightMsgUpdate = "op_weight_msg_update"
)

// WeightedOperations returns all the operations from the module with their respective weights
func WeightedOperations(
	appParams simtypes.AppParams, cdc *codec.Codec, ak govtypes.AccountKeeper,
	bk govtypes.BankKeeper, k keeper.Keeper) simulation.WeightedOperations {
	var (
		weightMsgCreate int
		weightMsgUpdate int
	)

	appParams.GetOrGenerate(
		cdc, OpWeightMsgCreate, &weightMsgCreate, nil, func(r *rand.Rand) {
			weightMsgCreate = simappparams.DefaultWeightMsgCreateProvider
		},
	)

	appParams.GetOrGenerate(
		cdc, OpWeightMsgUpdate, &weightMsgUpdate, nil, func(r *rand.Rand) {
			weightMsgUpdate = simappparams.DefaultWeightMsgUpdateProvider
		},
	)

	return simulation.WeightedOperations{
		simulation.NewWeightedOperation(
			weightMsgCreate,
			SimulateMsgCreate(ak, bk, k),
		),
		simulation.NewWeightedOperation(
			weightMsgUpdate,
			SimulateMsgUpdate(ak, bk, k),
		),
	}
}

// SimulateMsgCreate generates a MsgCreate with random values
// nolint:funlen
func SimulateMsgCreate(ak govtypes.AccountKeeper, bk govtypes.BankKeeper, k keeper.Keeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		simAccount, _ := simtypes.RandomAcc(r, accounts)

		// ensure the provider doesn't exist already
		_, found := k.Get(ctx, simAccount.Address)
		if found {
			return simtypes.NoOpMsg(types.ModuleName), nil, nil
		}

		cfg, readError := config.ReadConfigPath("../x/provider/testdata/provider.yml")
		if readError != nil {
			return simtypes.NoOpMsg(types.ModuleName), nil, readError
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName), nil, err
		}

		msg := types.MsgCreate{
			Owner:      simAccount.Address,
			HostURI:    cfg.Host,
			Attributes: cfg.GetAttributes(),
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

// SimulateMsgUpdate generates a MsgUpdate with random values
// nolint:funlen
func SimulateMsgUpdate(ak govtypes.AccountKeeper, bk govtypes.BankKeeper, k keeper.Keeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account,
		chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		var providers []types.Provider

		k.WithProviders(ctx, func(provider types.Provider) bool {
			providers = append(providers, provider)

			return false
		})

		if len(providers) == 0 {
			return simtypes.NoOpMsg(types.ModuleName), nil, nil
		}

		// Get random deployment
		i := r.Intn(len(providers))
		provider := providers[i]

		simAccount, found := simtypes.FindAccount(accounts, provider.Owner)
		if !found {
			return simtypes.NoOpMsg(types.ModuleName), nil, fmt.Errorf("provider with %s not found", provider.Owner)
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		spendable := bk.SpendableCoins(ctx, account.GetAddress())

		fees, err := simtypes.RandomFees(r, ctx, spendable)
		if err != nil {
			return simtypes.NoOpMsg(types.ModuleName), nil, err
		}

		msg := types.MsgUpdate{
			Owner:      simAccount.Address,
			HostURI:    provider.HostURI,
			Attributes: provider.Attributes,
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
