package main

import (
	"errors"
	"fmt"

	"github.com/ovrclk/akash/cmd/akash/constants"
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/spf13/cobra"
)

func deploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "deployment",
		Short: "manage deployments",
	}

	cmd.AddCommand(deployCommand())
	cmd.AddCommand(cancelCommand())

	return cmd
}

func deployCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "create <file>",
		Short: "create a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: context.WithContext(
			context.RequireKey(context.RequireNode(doDeployCommand))),
	}

	context.AddFlagNode(cmd, cmd.Flags())
	context.AddFlagKey(cmd, cmd.Flags())
	context.AddFlagNonce(cmd, cmd.Flags())

	return cmd
}

func parseDeployment(file string, tenant []byte, nonce uint64) (*types.Deployment, error) {
	// todo: read and parse deployment yaml file

	/* begin stub data */
	deployment := testutil.Deployment(tenant, nonce)
	/* end stub data */

	return deployment, nil
}

func doDeployCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	kmgr, err := ctx.KeyManager()
	if err != nil {
		return err
	}

	key, err := ctx.Key()
	if err != nil {
		return err
	}

	nonce, err := ctx.Nonce()
	if err != nil {
		return err
	}

	deployment, err := parseDeployment(args[0], key.Address, nonce)
	if err != nil {
		return err
	}

	signer := txutil.NewKeystoreSigner(kmgr, key.Name, constants.Password)

	tx, err := txutil.BuildTx(signer, nonce, &types.TxCreateDeployment{
		Deployment: deployment,
	})
	if err != nil {
		return err
	}

	res, err := ctx.Client().BroadcastTxCommit(tx)
	if err != nil {
		ctx.Log().Error("error sending tx", err)
		return err
	}
	if !res.CheckTx.IsOK() {
		ctx.Log().Error("error delivering tx", "err", res.CheckTx.GetLog())
		return errors.New(res.CheckTx.GetLog())
	}
	if !res.DeliverTx.IsOK() {
		ctx.Log().Error("error delivering tx", "err", res.DeliverTx.GetLog())
		return errors.New(res.DeliverTx.GetLog())
	}
	fmt.Printf("Created deployment: %X\n", deployment.Address)

	return nil
}

func cancelCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "close <deployment>",
		Short: "close a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: context.WithContext(
			context.RequireKey(context.RequireNode(doCancelCommand))),
	}

	context.AddFlagNode(cmd, cmd.Flags())
	context.AddFlagKey(cmd, cmd.Flags())
	context.AddFlagNonce(cmd, cmd.Flags())

	return cmd
}

func doCancelCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	kmgr, err := ctx.KeyManager()
	if err != nil {
		return err
	}

	key, err := ctx.Key()
	if err != nil {
		return err
	}

	nonce, err := ctx.Nonce()
	if err != nil {
		return err
	}

	signer := txutil.NewKeystoreSigner(kmgr, key.Name, constants.Password)

	deployment := new(base.Bytes)
	err = deployment.DecodeString(args[0])
	if err != nil {
		return err
	}

	tx, err := txutil.BuildTx(signer, nonce, &types.TxCloseDeployment{
		Deployment: *deployment,
	})
	if err != nil {
		return err
	}

	res, err := ctx.Client().BroadcastTxCommit(tx)
	if err != nil {
		ctx.Log().Error("error sending tx", err)
		return err
	}
	if !res.CheckTx.IsOK() {
		ctx.Log().Error("error delivering tx", "err", res.CheckTx.GetLog())
		return errors.New(res.CheckTx.GetLog())
	}
	if !res.DeliverTx.IsOK() {
		ctx.Log().Error("error delivering tx", "err", res.DeliverTx.GetLog())
		return errors.New(res.DeliverTx.GetLog())
	}
	fmt.Printf("Closing deployment: %X\n", deployment)

	// todo: wait for TxCancelDeployment for same deployment

	return nil
}
