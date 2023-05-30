package decorators

import (
	errorsmod "cosmossdk.io/errors"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/authz"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"

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
			minDeposit := gpsd.govKeeper.GetParams(ctx).MinDeposit
			aDepositParams := gpsd.aGovKeeper.GetDepositParams(ctx)
			minimumInitialDeposit := gpsd.calcMinimumInitialDeposit(aDepositParams.MinInitialDepositRate, minDeposit)
			if msg.InitialDeposit.IsAllLT(minimumInitialDeposit) {
				return errorsmod.Wrapf(sdkerrors.ErrUnauthorized, "not enough initial deposit. required: %v", minimumInitialDeposit)
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
					return errorsmod.Wrapf(sdkerrors.ErrUnauthorized, "cannot unmarshal authz exec msgs")
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
