package simulation

import (
	"math/rand"

	sdk "github.com/cosmos/cosmos-sdk/types"
	simtypes "github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/simulation"

	types "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/x/sink/keeper"
)

const (
	OpWeightMsgUpdateParams = "op_weight_sink_update_params" //nolint:gosec // simulation weight key
	OpWeightMsgRequestVoid  = "op_weight_sink_request_void"  //nolint:gosec // simulation weight key
)

func ProposalMsgs(k keeper.Keeper) []simtypes.WeightedProposalMsg {
	return []simtypes.WeightedProposalMsg{
		simulation.NewWeightedProposalMsg(OpWeightMsgUpdateParams, 10, SimulateMsgUpdateParams(k)),
		simulation.NewWeightedProposalMsg(OpWeightMsgRequestVoid, 100, SimulateMsgRequestVoid(k)),
	}
}

func SimulateMsgUpdateParams(k keeper.Keeper) simtypes.MsgSimulatorFn {
	return func(r *rand.Rand, ctx sdk.Context, _ []simtypes.Account) sdk.Msg {
		params, err := k.GetParams(ctx)
		if err != nil {
			panic(err)
		}

		if params.MinAmountPerTransfer.IsZero() {
			return nil
		}

		// Vary the admission minimum without invalidating existing settlements or policy.
		minimum, err := simtypes.RandPositiveInt(r, params.MinAmountPerTransfer.Amount)
		if err != nil {
			panic(err)
		}

		params.MinAmountPerTransfer.Amount = minimum

		return &types.MsgUpdateParams{
			Authority: k.GetAuthority(),
			Params:    params,
		}
	}
}

func SimulateMsgRequestVoid(k keeper.Keeper) simtypes.MsgSimulatorFn {
	return func(r *rand.Rand, ctx sdk.Context, _ []simtypes.Account) sdk.Msg {
		transfers, err := openTransfers(ctx, k)
		if err != nil {
			panic(err)
		}

		for _, index := range r.Perm(len(transfers)) {
			transfer := transfers[index]
			if transfer.Settlement.VoidAuthorization != nil || len(transfer.Settlement.Votes) > 0 {
				continue
			}

			return &types.MsgRequestVoid{
				Authority:     k.GetAuthority(),
				ID:            transfer.ID,
				ExpectedRound: transfer.Settlement.Round,
			}
		}

		return nil
	}
}
