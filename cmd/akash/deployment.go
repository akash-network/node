package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/cmd/akash/query"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/marketplace"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	. "github.com/ovrclk/akash/util"
	"github.com/spf13/cobra"
)

func deploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "deployment",
		Short: "manage deployments",
	}

	cmd.AddCommand(createDeploymentCommand())
	cmd.AddCommand(closeDeploymentCommand())
	cmd.AddCommand(sendManifestCommand())

	return cmd
}

func createDeploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "create <deployment-file>",
		Short: "create a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: context.WithContext(
			context.RequireKey(context.RequireNode(createDeployment))),
	}

	context.AddFlagNode(cmd, cmd.Flags())
	context.AddFlagKey(cmd, cmd.Flags())
	context.AddFlagNonce(cmd, cmd.Flags())
	context.AddFlagWait(cmd, cmd.Flags())

	return cmd
}

func createDeployment(ctx context.Context, cmd *cobra.Command, args []string) error {

	const ttl = int64(5)

	txclient, err := ctx.TxClient()
	if err != nil {
		return err
	}

	nonce, err := txclient.Nonce()
	if err != nil {
		return err
	}

	sdl, err := sdl.ReadFile(args[0])
	if err != nil {
		return err
	}

	groups, err := sdl.DeploymentGroups()
	if err != nil {
		return err
	}

	mani, err := sdl.Manifest()
	if err != nil {
		return err
	}

	res, err := txclient.BroadcastTxCommit(&types.TxCreateDeployment{
		Tenant:   txclient.Key().Address(),
		Nonce:    nonce,
		OrderTTL: ttl,
		Groups:   groups,
	})

	if err != nil {
		ctx.Log().Error("error sending tx", "error", err)
		return err
	}

	address := res.DeliverTx.Data

	fmt.Println(X(address))

	if ctx.Wait() {
		fmt.Printf("Waiting...\n")
		expected := len(groups)
		handler := marketplace.NewBuilder().
			OnTxCreateFulfillment(func(tx *types.TxCreateFulfillment) {
				if bytes.Equal(tx.Deployment, address) {
					fmt.Printf("Group %v/%v Fulfillment: %v\n", tx.Group, len(groups),
						X(state.FulfillmentID(tx.Deployment, tx.Group, tx.Order, tx.Provider)))
				}
			}).
			OnTxCreateLease(func(tx *types.TxCreateLease) {
				if bytes.Equal(tx.Deployment, address) {
					fmt.Printf("Group %v/%v Lease: %v\n", tx.Group, len(groups),
						X(state.FulfillmentID(tx.Deployment, tx.Group, tx.Order, tx.Provider)))
					// get lease provider
					prov, err := query.Provider(ctx, tx.Provider)
					if err != nil {
						fmt.Printf("ERROR: %v", err)
					}

					// send manifest over http to provider uri
					fmt.Printf("Sending manifest to %v...\n", prov.HostURI)
					err = manifest.Send(mani, txclient.Signer(), prov, tx.Deployment)
					if err != nil {
						fmt.Printf("ERROR: %v", err)
					}
					expected--
				}
				if expected == 0 {
					os.Exit(0)
				}
			}).Create()
		return common.MonitorMarketplace(ctx.Log(), ctx.Client(), handler)
	}

	return nil
}

func closeDeploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "close <deployment-id>",
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
	txclient, err := ctx.TxClient()
	if err != nil {
		return err
	}

	deployment, err := base.DecodeString(args[0])
	if err != nil {
		return err
	}

	_, err = txclient.BroadcastTxCommit(&types.TxCloseDeployment{
		Deployment: deployment,
		Reason:     types.TxCloseDeployment_TENANT_CLOSE,
	})

	if err != nil {
		ctx.Log().Error("error sending tx", "error", err)
		return err
	}

	fmt.Println("Closing deployment")
	return nil
}

func sendManifestCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "sendmani [manifest] [lease]",
		Short: "send manifest to lease provider",
		Args:  cobra.ExactArgs(2),
		RunE: context.WithContext(
			context.RequireKey(context.RequireNode(sendManifest))),
	}

	context.AddFlagNode(cmd, cmd.Flags())
	context.AddFlagKey(cmd, cmd.Flags())

	return cmd
}

func sendManifest(ctx context.Context, cmd *cobra.Command, args []string) error {
	signer, _, err := ctx.Signer()
	if err != nil {
		return err
	}

	sdl, err := sdl.ReadFile(args[0])
	if err != nil {
		return err
	}

	mani, err := sdl.Manifest()
	if err != nil {
		return err
	}

	leaseAddr, err := base.DecodeString(args[1])
	if err != nil {
		return err
	}

	lease, err := query.Lease(ctx, leaseAddr)
	if err != nil {
		return err
	}

	provider, err := query.Provider(ctx, lease.Provider)
	if err != nil {
		return err
	}

	err = manifest.Send(mani, signer, provider, lease.Deployment)
	if err != nil {
		return err
	}
	return nil
}
