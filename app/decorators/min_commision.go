package decorators

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/authz"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	astakingkeeper "github.com/akash-network/node/x/staking/keeper"
)

type MinCommissionDecorator struct {
	cdc    codec.BinaryCodec
	keeper astakingkeeper.IKeeper
}

func NewMinCommissionDecorator(cdc codec.BinaryCodec, k astakingkeeper.IKeeper) *MinCommissionDecorator {
	min := &MinCommissionDecorator{
		cdc:    cdc,
		keeper: k,
	}

	return min
}

func (min *MinCommissionDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (newCtx sdk.Context, err error) {
	if err := min.checkMsgs(ctx, tx.GetMsgs()); err != nil {
		return ctx, err
	}

	return next(ctx, tx, simulate)
}

func (min *MinCommissionDecorator) isValidMsg(ctx sdk.Context, m sdk.Msg) error {
	var rate sdk.Dec
	var maxRate *sdk.Dec

	switch msg := m.(type) {
	case *stakingtypes.MsgCreateValidator:
		maxRate = &msg.Commission.MaxRate
		rate = msg.Commission.Rate
	case *stakingtypes.MsgEditValidator:
		// if commission rate is nil, it means only
		// other fields are affected - skip
		if msg.CommissionRate == nil {
			return nil
		}

		rate = *msg.CommissionRate
	default:
		// the message is not for validator, so just skip over it
		return nil
	}

	minRate := min.keeper.MinCommissionRate(ctx)

	// prevent new validators joining the set with
	// commission set below 5%
	if rate.LT(minRate) {
		return sdkerrors.Wrap(sdkerrors.ErrUnauthorized, fmt.Sprintf("commission can't be lower than %s%%", minRate))
	}

	if maxRate != nil && maxRate.LT(minRate) {
		return sdkerrors.Wrap(sdkerrors.ErrUnauthorized, fmt.Sprintf("commission max rate can't be lower than %s%%", minRate))
	}

	return nil
}

func (min *MinCommissionDecorator) isValidAuthz(ctx sdk.Context, execMsg *authz.MsgExec) error {
	for _, v := range execMsg.Msgs {
		var innerMsg sdk.Msg
		err := min.cdc.UnpackAny(v, &innerMsg)
		if err != nil {
			return sdkerrors.Wrapf(sdkerrors.ErrUnauthorized, "cannot unmarshal authz exec msgs")
		}

		err = min.isValidMsg(ctx, innerMsg)
		if err != nil {
			return err
		}
	}

	return nil
}

func (min *MinCommissionDecorator) checkMsgs(ctx sdk.Context, msgs []sdk.Msg) error {
	for _, m := range msgs {
		if msg, ok := m.(*authz.MsgExec); ok {
			if err := min.isValidAuthz(ctx, msg); err != nil {
				return err
			}
			continue
		}

		// validate normal msgs
		err := min.isValidMsg(ctx, m)
		if err != nil {
			return err
		}
	}

	return nil
}
