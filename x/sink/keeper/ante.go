package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "pkg.akt.dev/go/node/sink/v1"
)

// DepositDecorator records transaction shape, never ledger state or bank access.
// A nested MsgSink receives no capability, even if it authenticates the sender.
type DepositDecorator struct {
	keeper Keeper
}

func NewDepositDecorator(k Keeper) DepositDecorator {
	return DepositDecorator{
		keeper: k,
	}
}

func (d DepositDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	msgs := tx.GetMsgs()

	params, err := d.keeper.GetParams(ctx)
	if err != nil {
		return ctx, err
	}

	if !params.Enabled {
		return ctx, types.ErrDepositsDisabled
	}

	for _, msg := range msgs {
		if _, ok := msg.(*types.MsgSink); !ok {
			return ctx, sdkerrors.ErrInvalidRequest.Wrap("sink deposit must be the only transaction message")
		}
	}

	return next(ctx, tx, simulate)
}
