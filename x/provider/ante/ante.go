package ante

import (
	"encoding/hex"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
)

type providerDecorator struct {
	keeper keeper.Keeper
}

// NewDecorator returns a decorator for "deployment" type messages
func NewDecorator(keeper keeper.Keeper) sdk.AnteDecorator {
	return providerDecorator{
		keeper: keeper,
	}
}

func (dd providerDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	for _, msg := range tx.GetMsgs() {
		var err error
		switch msg := msg.(type) {
		case types.MsgCreateProvider:
			err = dd.handleMsgCreate(ctx, msg)
		case types.MsgUpdateProvider:
			err = dd.handleMsgUpdate(ctx, msg)
		case types.MsgDeleteProvider:
			err = dd.handleMsgDelete(ctx, msg)
		default:
			continue
		}

		if err != nil {
			return ctx, err
		}
	}
	return next(ctx, tx, simulate)
}

func (dd providerDecorator) handleMsgCreate(ctx sdk.Context, msg types.MsgCreateProvider) error {
	if _, ok := dd.keeper.Get(ctx, msg.Owner); ok {
		return errors.Wrapf(keeper.ErrProviderAlreadyExists, "id: %s", hex.EncodeToString(msg.Owner))
	}

	return nil
}

func (dd providerDecorator) handleMsgUpdate(ctx sdk.Context, msg types.MsgUpdateProvider) error {
	if _, ok := dd.keeper.Get(ctx, msg.Owner); ok {
		return errors.Wrapf(keeper.ErrProviderNotFound, "id: %s", hex.EncodeToString(msg.Owner))
	}

	return nil
}

func (dd providerDecorator) handleMsgDelete(ctx sdk.Context, msg types.MsgDeleteProvider) error {
	if _, ok := dd.keeper.Get(ctx, msg.Owner); ok {
		return keeper.ErrProviderNotFound
	}

	return nil
}
