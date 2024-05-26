package app

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	auctionante "github.com/skip-mev/block-sdk/x/auction/ante"
	auctionkeeper "github.com/skip-mev/block-sdk/x/auction/keeper"

	"pkg.akt.dev/akashd/app/decorators"
	agovkeeper "pkg.akt.dev/akashd/x/gov/keeper"
	astakingkeeper "pkg.akt.dev/akashd/x/staking/keeper"
)

// BlockSDKAnteHandlerParams are the parameters necessary to configure the block-sdk antehandlers
type BlockSDKAnteHandlerParams struct {
	mevLane       auctionante.MEVLane
	auctionKeeper auctionkeeper.Keeper
	txConfig      client.TxConfig
}

// HandlerOptions extends the SDK's AnteHandler options
type HandlerOptions struct {
	ante.HandlerOptions
	CDC            codec.BinaryCodec
	AStakingKeeper astakingkeeper.IKeeper
	GovKeeper      *govkeeper.Keeper
	AGovKeeper     agovkeeper.IKeeper
	BlockSDK       BlockSDKAnteHandlerParams
}

// NewAnteHandler returns an AnteHandler that checks and increments sequence
// numbers, checks signatures & account numbers, and deducts fees from the first
// signer.
func NewAnteHandler(options HandlerOptions) (sdk.AnteHandler, error) {
	if options.AccountKeeper == nil {
		return nil, sdkerrors.ErrLogic.Wrap("account keeper is required for ante builder")
	}

	if options.BankKeeper == nil {
		return nil, sdkerrors.ErrLogic.Wrap("bank keeper is required for ante builder")
	}

	if options.SignModeHandler == nil {
		return nil, sdkerrors.ErrLogic.Wrap("sign mode handler is required for ante builder")
	}

	if options.SigGasConsumer == nil {
		return nil, sdkerrors.ErrLogic.Wrap("sig gas consumer handler is required for ante builder")
	}

	if options.AStakingKeeper == nil {
		return nil, sdkerrors.ErrLogic.Wrap("custom akash staking keeper is required for ante builder")
	}

	if options.GovKeeper == nil {
		return nil, sdkerrors.ErrLogic.Wrap("akash governance keeper is required for ante builder")
	}

	if options.AGovKeeper == nil {
		return nil, sdkerrors.ErrLogic.Wrap("custom akash governance keeper is required for ante builder")
	}

	if options.FeegrantKeeper == nil {
		return nil, sdkerrors.ErrLogic.Wrap("akash feegrant keeper is required for ante builder")
	}

	anteDecorators := []sdk.AnteDecorator{
		ante.NewSetUpContextDecorator(), // outermost AnteDecorator. SetUpContext must be called first
		// ante.NewRejectExtensionOptionsDecorator(),
		// ante.NewMempoolFeeDecorator(),
		ante.NewValidateBasicDecorator(),
		ante.NewTxTimeoutHeightDecorator(),
		ante.NewValidateMemoDecorator(options.AccountKeeper),
		ante.NewConsumeGasForTxSizeDecorator(options.AccountKeeper),
		ante.NewDeductFeeDecorator(options.AccountKeeper, options.BankKeeper, options.FeegrantKeeper, nil),
		ante.NewSetPubKeyDecorator(options.AccountKeeper), // SetPubKeyDecorator must be called before all signature verification decorators
		ante.NewValidateSigCountDecorator(options.AccountKeeper),
		ante.NewSigGasConsumeDecorator(options.AccountKeeper, options.SigGasConsumer),
		ante.NewSigVerificationDecorator(options.AccountKeeper, options.SignModeHandler),
		ante.NewIncrementSequenceDecorator(options.AccountKeeper),
		decorators.NewMinCommissionDecorator(options.CDC, options.AStakingKeeper),
		// auction module antehandler
		auctionante.NewAuctionDecorator(
			options.BlockSDK.auctionKeeper,
			options.BlockSDK.txConfig.TxEncoder(),
			options.BlockSDK.mevLane,
		),
	}

	return sdk.ChainAnteDecorators(anteDecorators...), nil
}
