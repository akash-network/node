package decorators

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/authz"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	agovkeeper "github.com/akash-network/node/x/gov/keeper"
)

type GovPreventSpamDecorator struct {
	cdc        codec.BinaryCodec
	govKeeper  govkeeper.Keeper
	aGovKeeper agovkeeper.IKeeper
}

func NewGovPreventSpamDecorator(cdc codec.BinaryCodec, gov govkeeper.Keeper, aGov agovkeeper.IKeeper) GovPreventSpamDecorator {
	return GovPreventSpamDecorator{
		cdc:        cdc,
		govKeeper:  gov,
		aGovKeeper: aGov,
	}
}

func (gpsd GovPreventSpamDecorator) AnteHandle(
	ctx sdk.Context, tx sdk.Tx,
	simulate bool, next sdk.AnteHandler,
) (newCtx sdk.Context, err error) {
	msgs := tx.GetMsgs()

	err = gpsd.checkSpamSubmitProposalMsg(ctx, msgs)

	if err != nil {
		return ctx, err
	}

	return next(ctx, tx, simulate)
}

func (gpsd GovPreventSpamDecorator) checkSpamSubmitProposalMsg(ctx sdk.Context, msgs []sdk.Msg) error {
	validMsg := func(m sdk.Msg) error {
		if msg, ok := m.(*govtypes.MsgSubmitProposal); ok {
			// prevent spam gov msg
			depositParams := gpsd.govKeeper.GetDepositParams(ctx)
			aDepositParams := gpsd.aGovKeeper.GetDepositParams(ctx)

			minimumInitialDeposit := gpsd.calcMinimumInitialDeposit(aDepositParams.MinInitialDepositRate, depositParams.MinDeposit)
			if msg.InitialDeposit.IsAllLT(minimumInitialDeposit) {
				return sdkerrors.Wrapf(sdkerrors.ErrUnauthorized, "not enough initial deposit. required: %v", minimumInitialDeposit)
			}
		}
		return nil
	}

	// Check every msg in the tx, if it's a MsgExec, check the inner msgs.
	// If it's a MsgSubmitProposal, check the initial deposit is enough.
	for _, m := range msgs {
		var innerMsg sdk.Msg
		if msg, ok := m.(*authz.MsgExec); ok {
			for _, v := range msg.Msgs {
				err := gpsd.cdc.UnpackAny(v, &innerMsg)
				if err != nil {
					return sdkerrors.Wrapf(sdkerrors.ErrUnauthorized, "cannot unmarshal authz exec msgs")
				}

				err = validMsg(innerMsg)
				if err != nil {
					return err
				}
			}
		} else {
			err := validMsg(m)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (gpsd GovPreventSpamDecorator) calcMinimumInitialDeposit(rate sdk.Dec, minDeposit sdk.Coins) (minimumInitialDeposit sdk.Coins) {
	for _, coin := range minDeposit {
		minimumInitialCoin := rate.MulInt(coin.Amount).RoundInt()
		minimumInitialDeposit = minimumInitialDeposit.Add(sdk.NewCoin(coin.Denom, minimumInitialCoin))
	}

	return
}
