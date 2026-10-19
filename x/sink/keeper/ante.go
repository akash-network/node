package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "pkg.akt.dev/go/node/sink/v1"
)

// DepositDecorator records transaction shape, never ledger state or bank access.
// A nested MsgSink receives no capability, even if it authenticates the sender.
type DepositDecorator struct{}

func NewDepositDecorator() DepositDecorator { return DepositDecorator{} }

func (d DepositDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	ctx = ctx.WithValue(sinkRequestContextKey{}, nil)
	msgs := tx.GetMsgs()
	for _, msg := range msgs {
		sink, ok := msg.(*types.MsgSink)
		if !ok {
			continue
		}
		if sink == nil || len(msgs) != 1 {
			return ctx, sdkerrors.ErrInvalidRequest.Wrap("sink deposit must be the only transaction message")
		}
		amount := ""
		if !sink.Amount.Amount.IsNil() {
			amount = sink.Amount.Amount.String()
		}
		ctx = ctx.WithValue(sinkRequestContextKey{}, sinkRequest{
			sender: sink.Sender, destType: sink.DestType, dest: sink.Dest, denom: sink.Amount.Denom, amount: amount,
		})
	}
	return next(ctx, tx, simulate)
}
