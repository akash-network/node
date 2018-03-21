package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/marketplace"
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

	cmd.AddCommand(createDeploymentCommand())
	cmd.AddCommand(closeDeploymentCommand())

	return cmd
}

func createDeploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "create <file>",
		Short: "create a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: context.WithContext(
			context.RequireKey(context.RequireNode(createDeployment))),
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

func createDeployment(ctx context.Context, cmd *cobra.Command, args []string) error {
	signer, key, err := ctx.Signer()
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

func closeDeploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "close <deployment>",
		Short: "close a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: context.WithContext(
			context.RequireKey(context.RequireNode(closeDeployment))),
	}

	context.AddFlagNode(cmd, cmd.Flags())
	context.AddFlagKey(cmd, cmd.Flags())
	context.AddFlagNonce(cmd, cmd.Flags())

	return cmd
}

func closeDeployment(ctx context.Context, cmd *cobra.Command, args []string) error {
	signer, _, err := ctx.Signer()
	if err != nil {
		return err
	}

	nonce, err := ctx.Nonce()
	if err != nil {
		return err
	}

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

	fmt.Println("Closing deployment...")

	handler := marketplace.NewBuilder().
		OnTxDeploymentClosed(func(tx *types.TxDeploymentClosed) {
			if bytes.Equal(tx.Deployment, *deployment) {
				fmt.Printf("Closed deployment: %X\n", tx.Deployment)
				os.Exit(1)
			}
		}).Create()

	return common.MonitorMarketplace(ctx.Log(), ctx.Client(), handler)
}
