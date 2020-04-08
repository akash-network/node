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

// DENOM represents bond denom
const DENOM = "stake"

// WeightedOperations returns all the operations from the module with their respective weights
func WeightedOperations(
	appParams simulation.AppParams, cdc *codec.Codec, ak stakingtypes.AccountKeeper, k keeper.Keeper,
) simulation.WeightedOperations {

	var weightMsgCreate int
	var weightMsgUpdate int

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
			SimulateMsgCreate(ak, k),
		),
		simulation.NewWeightedOperation(
			weightMsgUpdate,
			SimulateMsgUpdate(ak, k),
		),
	}
}

// SimulateMsgCreate generates a MsgCreate with random values
// nolint:funlen
func SimulateMsgCreate(ak stakingtypes.AccountKeeper, k keeper.Keeper) simulation.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context,
		accounts []simulation.Account, chainID string,
	) (simulation.OperationMsg, []simulation.FutureOperation, error) {

		simAccount, _ := simulation.RandomAcc(r, accounts)

		// ensure the provider doesn't exist already
		_, found := k.Get(ctx, simAccount.Address)
		if found {
			return simulation.NoOpMsg(types.ModuleName), nil, nil
		}

		cfg, readError := config.ReadConfigPath("../x/provider/testdata/provider.yml")
		if readError != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, readError
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		fees, err := simulation.RandomFees(r, ctx, account.SpendableCoins(ctx.BlockTime()))
		if err != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, err
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
			return simulation.NoOpMsg(types.ModuleName), nil, err
		}

		return simulation.NewOperationMsg(msg, true, ""), nil, nil
	}
}

// SimulateMsgUpdate generates a MsgUpdate with random values
// nolint:funlen
func SimulateMsgUpdate(ak stakingtypes.AccountKeeper, k keeper.Keeper) simulation.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context,
		accounts []simulation.Account, chainID string,
	) (simulation.OperationMsg, []simulation.FutureOperation, error) {

		var providers []types.Provider
		k.WithProviders(ctx, func(provider types.Provider) bool {
			providers = append(providers, provider)
			return false
		})

		if len(providers) == 0 {
			return simulation.NoOpMsg(types.ModuleName), nil, nil
		}

		// Get random deployment
		i := r.Intn(len(providers))
		provider := providers[i]

		simAccount, found := simulation.FindAccount(accounts, provider.Owner)
		if !found {
			return simulation.NoOpMsg(types.ModuleName), nil, fmt.Errorf("provider with %s not found", provider.Owner)
		}

		account := ak.GetAccount(ctx, simAccount.Address)
		fees, err := simulation.RandomFees(r, ctx, account.SpendableCoins(ctx.BlockTime()))
		if err != nil {
			return simulation.NoOpMsg(types.ModuleName), nil, err
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
			return simulation.NoOpMsg(types.ModuleName), nil, err
		}

		return simulation.NewOperationMsg(msg, true, ""), nil, nil
	}
}
