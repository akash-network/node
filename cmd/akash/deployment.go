package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/marketplace"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/txutil"
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
	context.AddFlagWait(cmd, cmd.Flags())

	return cmd
}

func parseDeployment(file string, nonce uint64) ([]*types.GroupSpec, int64, error) {
	// todo: read and parse deployment yaml file

	specs := []*types.GroupSpec{}

	/* begin stub data */
	groups := testutil.DeploymentGroups(*new(base.Bytes), nonce)

	for _, group := range groups.GetItems() {
		s := &types.GroupSpec{
			Resources:    group.Resources,
			Requirements: group.Requirements,
		}
		specs = append(specs, s)
	}

	ttl := int64(5)

	/* end stub data */

	return specs, ttl, nil
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

	groups, ttl, err := parseDeployment(args[0], nonce)
	if err != nil {
		return err
	}

	tx, err := txutil.BuildTx(signer, nonce, &types.TxCreateDeployment{
		Tenant:   key.Address(),
		Nonce:    nonce,
		OrderTTL: ttl,
		Groups:   groups,
	})
	if err != nil {
		return err
	}

	res, err := ctx.Client().BroadcastTxCommit(tx)
	if err != nil {
		ctx.Log().Error("error sending tx", "error", err)
		return err
	}
	if !res.CheckTx.IsOK() {
		ctx.Log().Error("error delivering tx", "error", res.CheckTx.GetLog())
		return errors.New(res.CheckTx.GetLog())
	}
	if !res.DeliverTx.IsOK() {
		ctx.Log().Error("error delivering tx", "error", res.DeliverTx.GetLog())
		return errors.New(res.DeliverTx.GetLog())
	}

	address := res.DeliverTx.Data

	fmt.Println(X(address))

	if ctx.Wait() {
		fmt.Printf("Waiting...\n")
		expected := len(groups)
		handler := marketplace.NewBuilder().
			OnTxCreateFulfillment(func(tx *types.TxCreateFulfillment) {
				if bytes.Equal(tx.Fulfillment.Deployment, address) {
					f := tx.Fulfillment
					fmt.Printf("Group %v/%v Fulfillment: %v\n", f.Group, len(groups),
						X(state.FulfillmentID(f.Deployment, f.Group, f.Order, f.Provider)))
				}
			}).
			OnTxCreateLease(func(tx *types.TxCreateLease) {
				if bytes.Equal(tx.Lease.Deployment, address) {
					l := tx.Lease
					fmt.Printf("Group %v/%v Lease: %v\n", l.Group, len(groups),
						X(state.FulfillmentID(l.Deployment, l.Group, l.Order, l.Provider)))
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
		Use:   "close <deployment>",
		Short: "close a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: context.WithContext(
			context.RequireKey(context.RequireNode(closeDeployment))),
	}

	context.AddFlagNode(cmd, cmd.Flags())
	context.AddFlagKey(cmd, cmd.Flags())
	context.AddFlagNonce(cmd, cmd.Flags())
	context.AddFlagWait(cmd, cmd.Flags())

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
		ctx.Log().Error("error sending tx", "error", err)
		return err
	}
	if !res.CheckTx.IsOK() {
		ctx.Log().Error("error delivering tx", "error", res.CheckTx.GetLog())
		return errors.New(res.CheckTx.GetLog())
	}
	if !res.DeliverTx.IsOK() {
		ctx.Log().Error("error delivering tx", "error", res.DeliverTx.GetLog())
		return errors.New(res.DeliverTx.GetLog())
	}

	fmt.Println("Closing deployment")

	if ctx.Wait() {
		fmt.Printf("Waiting...\n")
		handler := marketplace.NewBuilder().
			OnTxDeploymentClosed(func(tx *types.TxDeploymentClosed) {
				if bytes.Equal(tx.Deployment, *deployment) {
					fmt.Printf("Closed deployment: %v\n", X(tx.Deployment))
					os.Exit(0)
				}
			}).Create()
		return common.MonitorMarketplace(ctx.Log(), ctx.Client(), handler)
	}

	return nil
}
