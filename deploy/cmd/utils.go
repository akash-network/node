package cmd

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/pflag"
)

// SendMsgs sends given sdk messages
func SendMsgs(clientCtx client.Context, flags *pflag.FlagSet, datagrams []sdk.Msg) (res *sdk.TxResponse, err error) {
	// validate basic all the msgs
	for _, msg := range datagrams {
		if err := msg.ValidateBasic(); err != nil {
			return res, err
		}
	}

	return BuildAndBroadcastTx(clientCtx, flags, datagrams)
}

// BuildAndBroadcastTx takes messages and builds, signs and marshals a sdk.Tx to prepare it for broadcast
func BuildAndBroadcastTx(clientCtx client.Context, flags *pflag.FlagSet, msgs []sdk.Msg) (*sdk.TxResponse, error) {
	txf := tx.NewFactoryCLI(clientCtx, flags).
		WithTxConfig(clientCtx.TxConfig).
		WithAccountRetriever(clientCtx.AccountRetriever)

	keyname := clientCtx.GetFromName()
	info, err := txf.Keybase().Key(keyname)
	if err != nil {
		return nil, err
	}

	txf, err = tx.PrepareFactory(clientCtx, txf)
	if err != nil {
		return nil, err
	}

	// If users pass gas adjustment, then calculate gas
	_, adjusted, err := tx.CalculateGas(clientCtx.QueryWithData, txf, msgs...)
	if err != nil {
		return nil, err
	}

	// Set the gas amount on the transaction factory
	txf = txf.WithGas(adjusted)

	// Build the transaction builder
	txb, err := tx.BuildUnsignedTx(txf, msgs...)
	if err != nil {
		return nil, err
	}

	// Attach the signature to the transaction
	err = tx.Sign(txf, info.GetName(), txb, true)
	if err != nil {
		return nil, err
	}

	// Generate the transaction bytes
	txBytes, err := clientCtx.TxConfig.TxEncoder()(txb.GetTx())
	if err != nil {
		return nil, err
	}

	return clientCtx.BroadcastTxSync(txBytes)
}
