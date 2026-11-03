package simulation

import (
	"crypto/ed25519"
	"math/rand"
	"slices"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"
	"github.com/mr-tron/base58"

	types "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/x/sink/keeper"
)

const (
	OpWeightMsgSink           = "op_weight_sink"                 //nolint:gosec // simulation weight key
	OpWeightMsgReportOutcome  = "op_weight_sink_report_outcome"  //nolint:gosec // simulation weight key
	OpWeightMsgFinalizeRefund = "op_weight_sink_finalize_refund" //nolint:gosec // simulation weight key
)

// WeightedOperations uses signed, single-message transactions, including Sink's ante binding.
func WeightedOperations(appParams simtypes.AppParams, txConfig client.TxConfig, ak keeper.AccountKeeper, bk keeper.BankKeeper, k keeper.Keeper) simulation.WeightedOperations {
	operations := []struct {
		key    string
		weight int
		op     simtypes.Operation
	}{
		{
			key:    OpWeightMsgSink,
			weight: 100,
			op:     SimulateMsgSink(txConfig, ak, bk, k),
		},
		{
			key:    OpWeightMsgReportOutcome,
			weight: 100,
			op:     SimulateMsgReportOutcome(txConfig, ak, bk, k),
		},
		{
			key:    OpWeightMsgFinalizeRefund,
			weight: 50,
			op:     SimulateMsgFinalizeRefund(txConfig, ak, bk, k),
		},
	}

	weighted := make(simulation.WeightedOperations, 0, len(operations))
	for _, operation := range operations {
		weight := operation.weight
		appParams.GetOrGenerate(operation.key, &weight, nil, func(_ *rand.Rand) {
			weight = operation.weight
		})

		weighted = append(weighted, simulation.NewWeightedOperation(weight, operation.op))
	}

	return weighted
}

func SimulateMsgSink(txConfig client.TxConfig, ak keeper.AccountKeeper, bk keeper.BankKeeper, k keeper.Keeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account, chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		msg := &types.MsgSink{DestType: types.DestTypeSolana}
		state, err := k.NewQuerier().State(ctx, nil)
		if err != nil {
			return noOp(msg, "query state"), nil, err
		}

		if !state.Params.Enabled || state.Policy.Threshold == 0 || state.Epoch == 0 {
			return noOp(msg, "deposits disabled or quota epoch not started"), nil, nil
		}

		for _, index := range r.Perm(len(accounts)) {
			account := accounts[index]
			balance := spendable(ctx, ak, bk, account).AmountOf(sdkutil.DenomUakt).SubRaw(1)
			usage, err := k.NewQuerier().Account(ctx, &types.QueryAccountRequest{Owner: account.Address.String()})
			if err != nil {
				return noOp(msg, "query account usage"), nil, err
			}

			maximum := sdkmath.MinInt(balance, state.Params.MaxAmountPerTransfer.Amount)
			maximum = sdkmath.MinInt(maximum, state.Params.MaxTransferAmountPerEpoch.Amount.Sub(state.SendUsage.Amount.Amount))
			if state.Params.MaxEpochAmountPerAccount.IsPositive() {
				maximum = sdkmath.MinInt(maximum, state.Params.MaxEpochAmountPerAccount.Amount.Sub(usage.Usage.EpochSent.Amount))
			}

			minimum := state.Params.MinAmountPerTransfer.Amount
			if maximum.LT(minimum) {
				continue
			}

			amount, err := simtypes.RandPositiveInt(r, maximum.Sub(minimum).AddRaw(1))
			if err != nil {
				return noOp(msg, "random deposit amount"), nil, err
			}

			seed := make([]byte, ed25519.SeedSize)
			_, _ = r.Read(seed)
			pubkey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
			msg.Sender = account.Address.String()
			msg.Dest = base58.Encode(pubkey)
			msg.Amount = sdk.NewCoin(sdkutil.DenomUakt, minimum.Add(amount).SubRaw(1))

			return deliver(r, app, ctx, txConfig, ak, account, chainID, msg)
		}

		return noOp(msg, "no funded account with send quota"), nil, nil
	}
}

func SimulateMsgReportOutcome(txConfig client.TxConfig, ak keeper.AccountKeeper, bk keeper.BankKeeper, k keeper.Keeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account, chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		msg := &types.MsgReportOutcome{}
		policy, err := k.GetCurrentPolicy(ctx)
		if err != nil {
			return noOp(msg, "query reporter policy"), nil, err
		}

		transfers, err := openTransfers(ctx, k)
		if err != nil {
			return noOp(msg, "query open transfers"), nil, err
		}

		for _, index := range r.Perm(len(transfers)) {
			transfer := transfers[index]
			settlement := transfer.Settlement
			if settlement.Hold != types.HoldNone || settlement.GovernanceObservation != nil {
				continue
			}

			for _, accountIndex := range r.Perm(len(accounts)) {
				account := accounts[accountIndex]
				reporter := account.Address.String()
				if !slices.Contains(policy.Members, reporter) || spendable(ctx, ak, bk, account).AmountOf(sdkutil.DenomUakt).LT(sdkmath.OneInt()) {
					continue
				}

				if settlement.Policy.Version == policy.Version && slices.ContainsFunc(settlement.Votes, func(vote types.Vote) bool {
					return vote.Reporter == reporter
				}) {
					continue
				}

				outcome := types.OutcomeMinted
				if settlement.VoidAuthorization != nil {
					outcome = types.OutcomeVoided
				}

				if settlement.Policy.Version == policy.Version && len(settlement.Votes) > 0 {
					outcome = settlement.Votes[0].Observation.Outcome
				}

				signature := make([]byte, ed25519.SignatureSize)
				_, _ = r.Read(signature)
				msg = &types.MsgReportOutcome{
					Reporter:      reporter,
					ID:            transfer.ID,
					PolicyVersion: policy.Version,
					Round:         settlement.Round,
					Observation: types.Observation{
						Outcome:              outcome,
						Slot:                 1 + r.Uint64()%1_000_000,
						TransactionSignature: signature,
					},
				}

				return deliver(r, app, ctx, txConfig, ak, account, chainID, msg)
			}
		}

		return noOp(msg, "no open transfer with an eligible reporter"), nil, nil
	}
}

func SimulateMsgFinalizeRefund(txConfig client.TxConfig, ak keeper.AccountKeeper, bk keeper.BankKeeper, k keeper.Keeper) simtypes.Operation {
	return func(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, accounts []simtypes.Account, chainID string) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
		msg := &types.MsgFinalizeRefund{}
		state, err := k.NewQuerier().State(ctx, nil)
		if err != nil {
			return noOp(msg, "query refund state"), nil, err
		}

		if !state.Params.RefundsEnabled || state.RefundHead == nil || state.Epoch == 0 {
			return noOp(msg, "refunds disabled or no refund candidate"), nil, nil
		}

		transfer, err := k.GetTransfer(ctx, *state.RefundHead)
		if err != nil {
			return noOp(msg, "query refund candidate"), nil, err
		}

		if ctx.BlockTime().Unix() < transfer.Settlement.ChallengeEnd {
			return noOp(msg, "refund challenge has not elapsed"), nil, nil
		}

		if state.RefundUsage.Amount.Add(transfer.Data.Amount).IsGT(state.Params.MaxEpochRefund) {
			return noOp(msg, "refund quota exhausted"), nil, nil
		}

		for _, index := range r.Perm(len(accounts)) {
			account := accounts[index]
			if spendable(ctx, ak, bk, account).AmountOf(sdkutil.DenomUakt).LT(sdkmath.OneInt()) {
				continue
			}

			msg.Executor = account.Address.String()
			msg.ID = transfer.ID

			return deliver(r, app, ctx, txConfig, ak, account, chainID, msg)
		}

		return noOp(msg, "no funded refund executor"), nil, nil
	}
}

func openTransfers(ctx sdk.Context, k keeper.Keeper) ([]types.TransferState, error) {
	var transfers []types.TransferState
	for _, status := range []types.Status{types.StatusTransferPending, types.StatusVoided} {
		request := &types.QueryTransfersRequest{
			Status:     status,
			Pagination: &query.PageRequest{Limit: 100},
		}
		for {
			response, err := k.NewQuerier().Transfers(ctx, request)
			if err != nil {
				return nil, err
			}

			transfers = append(transfers, response.Transfers...)

			if response.Pagination == nil || len(response.Pagination.NextKey) == 0 {
				break
			}

			request.Pagination.Key = response.Pagination.NextKey
		}
	}

	return transfers, nil
}

func spendable(ctx sdk.Context, ak keeper.AccountKeeper, bk keeper.BankKeeper, account simtypes.Account) sdk.Coins {
	acc := ak.GetAccount(ctx, account.Address)
	if acc == nil {
		return nil
	}

	if _, module := acc.(sdk.ModuleAccountI); module {
		return nil
	}

	balance := bk.GetAllBalances(ctx, account.Address)
	if vesting, ok := acc.(interface{ LockedCoins(time.Time) sdk.Coins }); ok {
		balance = balance.Sub(vesting.LockedCoins(ctx.BlockTime())...)
	}

	return balance
}

func deliver(r *rand.Rand, app *baseapp.BaseApp, ctx sdk.Context, txConfig client.TxConfig, ak keeper.AccountKeeper, account simtypes.Account, chainID string, msg sdk.Msg) (simtypes.OperationMsg, []simtypes.FutureOperation, error) {
	acc := ak.GetAccount(ctx, account.Address)
	tx, err := simtestutil.GenSignedMockTx(
		r,
		txConfig,
		[]sdk.Msg{msg},
		sdk.NewCoins(sdk.NewInt64Coin(sdkutil.DenomUakt, 1)),
		simtestutil.DefaultGenTxGas,
		chainID,
		[]uint64{acc.GetAccountNumber()},
		[]uint64{acc.GetSequence()},
		account.PrivKey,
	)
	if err != nil {
		return noOp(msg, "sign transaction"), nil, err
	}

	if _, _, err = app.SimDeliver(txConfig.TxEncoder(), tx); err != nil {
		return noOp(msg, "deliver transaction"), nil, err
	}

	return simtypes.NewOperationMsg(msg, true, ""), nil, nil
}

func noOp(msg sdk.Msg, reason string) simtypes.OperationMsg {
	return simtypes.NoOpMsg(types.ModuleName, sdk.MsgTypeURL(msg), reason)
}
