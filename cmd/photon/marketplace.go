package main

import (
	"fmt"

	"github.com/ovrclk/photon/cmd/common"
	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/ovrclk/photon/marketplace"
	"github.com/ovrclk/photon/types"
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
			fmt.Printf("TRANSFER %v tokens from %X to %X\n", tx.GetAmount(), tx.From, tx.To)
		}).
		OnTxCreateProvider(func(tx *types.TxCreateProvider) {
			fmt.Printf("DATACENTER CREATED: %X created by %X\n", tx.Provider.Address, tx.Provider.Owner)
		}).
		OnTxCreateDeployment(func(tx *types.TxCreateDeployment) {
			fmt.Printf("DEPLOYMENT CREATED: %X created by %X\n", tx.Deployment.Address, tx.Deployment.Tenant)
		}).
		OnTxCreateOrder(func(tx *types.TxCreateOrder) {
			fmt.Printf("order CREATED: %X/%v/%v\n",
				tx.Order.Deployment, tx.Order.Group, tx.Order.Order)
		}).
		OnTxCreateFulfillmentOrder(func(tx *types.TxCreateFulfillmentOrder) {
			fmt.Printf("FULFILLMENT ORDER CREATED %X/%v/%v by %X\n",
				tx.Order.Deployment, tx.Order.Group, tx.Order.Order,
				tx.Order.Provider)
		}).
		OnTxCreateLease(func(tx *types.TxCreateLease) {
			fmt.Printf("LEASE CREATED %X/%v/%v by %X\n",
				tx.Lease.Deployment, tx.Lease.Group, tx.Lease.Order,
				tx.Lease.Provider)
		}).
		Create()
}
