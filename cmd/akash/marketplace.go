package main

import (
	"fmt"

	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/marketplace"
	"github.com/ovrclk/akash/types"
	"github.com/spf13/cobra"
)

func marketplaceCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "marketplace",
		Short: "monitor marketplace",
		Args:  cobra.NoArgs,
		RunE:  context.WithContext(context.RequireNode(doMarketplaceMonitorCommand)),
	}

	context.AddFlagNode(cmd, cmd.PersistentFlags())

	return cmd
}

func doMarketplaceMonitorCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	handler := marketplaceMonitorHandler()
	return common.MonitorMarketplace(ctx.Log(), ctx.Client(), handler)
}

func marketplaceMonitorHandler() marketplace.Handler {
	return marketplace.NewBuilder().
		OnTxSend(func(tx *types.TxSend) {
			fmt.Printf("TRANSFER\t%v tokens from %X to %X\n", tx.GetAmount(), tx.From, tx.To)
		}).
		OnTxCreateProvider(func(tx *types.TxCreateProvider) {
			fmt.Printf("DATACENTER CREATED\t%X created by %X\n", tx.Provider.Address, tx.Provider.Owner)
		}).
		OnTxCreateDeployment(func(tx *types.TxCreateDeployment) {
			fmt.Printf("DEPLOYMENT CREATED\t%X created by %X\n", tx.Deployment.Address, tx.Deployment.Tenant)
		}).
		OnTxCreateOrder(func(tx *types.TxCreateOrder) {
			fmt.Printf("ORDER CREATED\t%X/%v/%v\n",
				tx.Order.Deployment, tx.Order.Group, tx.Order.Order)
		}).
		OnTxCreateFulfillment(func(tx *types.TxCreateFulfillment) {
			fmt.Printf("FULFILLMENT CREATED\t%X/%v/%v by %X [price=%v]\n",
				tx.Fulfillment.Deployment, tx.Fulfillment.Group, tx.Fulfillment.Order,
				tx.Fulfillment.Provider, tx.Fulfillment.Price)
		}).
		OnTxCreateLease(func(tx *types.TxCreateLease) {
			fmt.Printf("LEASE CREATED\t%X/%v/%v by %X [price=%v]\n",
				tx.Lease.Deployment, tx.Lease.Group, tx.Lease.Order,
				tx.Lease.Provider, tx.Lease.Price)
		}).
		OnTxDeploymentClosed(func(tx *types.TxDeploymentClosed) {
			fmt.Printf("DEPLOYMENT CLOSED\t%X", tx.Deployment)
		}).
		Create()
}
