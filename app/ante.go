package app

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"

	"github.com/akash-network/node/app/decorators"
	agovkeeper "github.com/akash-network/node/x/gov/keeper"
	astakingkeeper "github.com/akash-network/node/x/staking/keeper"
)

// HandlerOptions extends the SDK's AnteHandler options
type HandlerOptions struct {
	ante.HandlerOptions
	CDC            codec.BinaryCodec
	AStakingKeeper astakingkeeper.IKeeper
	GovKeeper      *govkeeper.Keeper
	AGovKeeper     agovkeeper.IKeeper
}

// NewAnteHandler returns an AnteHandler that checks and increments sequence
// numbers, checks signatures & account numbers, and deducts fees from the first
// signer.
func NewAnteHandler(options HandlerOptions) (sdk.AnteHandler, error) {
	if options.AccountKeeper == nil {
		return nil, sdkerrors.Wrap(sdkerrors.ErrLogic, "account keeper is required for ante builder")
	}

	if options.BankKeeper == nil {
		return nil, sdkerrors.Wrap(sdkerrors.ErrLogic, "bank keeper is required for ante builder")
	}

	if options.SignModeHandler == nil {
		return nil, sdkerrors.Wrap(sdkerrors.ErrLogic, "sign mode handler is required for ante builder")
	}

	if options.SigGasConsumer == nil {
		return nil, sdkerrors.Wrap(sdkerrors.ErrLogic, "sig gas consumer handler is required for ante builder")
	}

	if options.AStakingKeeper == nil {
		return nil, sdkerrors.Wrap(sdkerrors.ErrLogic, "custom akash staking keeper is required for ante builder")
	}

	if options.GovKeeper == nil {
		return nil, sdkerrors.Wrap(sdkerrors.ErrLogic, "akash governance keeper is required for ante builder")
	}

	if options.AGovKeeper == nil {
		return nil, sdkerrors.Wrap(sdkerrors.ErrLogic, "custom akash governance keeper is required for ante builder")
	}

	anteDecorators := []sdk.AnteDecorator{
		ante.NewSetUpContextDecorator(), // outermost AnteDecorator. SetUpContext must be called first
		ante.NewRejectExtensionOptionsDecorator(),
		ante.NewMempoolFeeDecorator(),
		ante.NewValidateBasicDecorator(),
		ante.NewTxTimeoutHeightDecorator(),
		ante.NewValidateMemoDecorator(options.AccountKeeper),
		ante.NewConsumeGasForTxSizeDecorator(options.AccountKeeper),
		ante.NewDeductFeeDecorator(options.AccountKeeper, options.BankKeeper, options.FeegrantKeeper),
		ante.NewSetPubKeyDecorator(options.AccountKeeper), // SetPubKeyDecorator must be called before all signature verification decorators
		ante.NewValidateSigCountDecorator(options.AccountKeeper),
		ante.NewSigGasConsumeDecorator(options.AccountKeeper, options.SigGasConsumer),
		ante.NewSigVerificationDecorator(options.AccountKeeper, options.SignModeHandler),
		ante.NewIncrementSequenceDecorator(options.AccountKeeper),
		decorators.NewMinCommissionDecorator(options.CDC, options.AStakingKeeper),
		decorators.NewGovPreventSpamDecorator(options.CDC, *options.GovKeeper, options.AGovKeeper),
	}

	return sdk.ChainAnteDecorators(anteDecorators...), nil
}
