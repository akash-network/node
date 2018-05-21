package main

import (
	"bytes"
	gcontext "context"
	"fmt"
	"math/rand"

	"github.com/ovrclk/akash/cmd/akash/constants"
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/cmd/akash/query"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/marketplace"
	qp "github.com/ovrclk/akash/query"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/ovrclk/akash/types/provider"
	. "github.com/ovrclk/akash/util"
	"github.com/spf13/cobra"
)

func providerCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "provider",
		Short: "manage provider",
		Args:  cobra.ExactArgs(1),
	}

	context.AddFlagNode(cmd, cmd.PersistentFlags())
	context.AddFlagKey(cmd, cmd.PersistentFlags())
	context.AddFlagNonce(cmd, cmd.PersistentFlags())

	cmd.AddCommand(createProviderCommand())
	cmd.AddCommand(runCommand())
	cmd.AddCommand(closeFulfillmentCommand())
	cmd.AddCommand(closeLeaseCommand())
	cmd.AddCommand(runManifestServerCommand())

	return cmd
}

func createProviderCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "create <file>",
		Short: "create a provider",
		Args:  cobra.ExactArgs(1),
		RunE:  context.WithContext(context.RequireNode(doCreateProviderCommand)),
	}

	context.AddFlagKeyType(cmd, cmd.Flags())

	return cmd
}

func doCreateProviderCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	kmgr, err := ctx.KeyManager()
	if err != nil {
		return err
	}

	// XXX generate key for provider if doens't exist
	key, err := ctx.Key()
	if err != nil {
		kname := ctx.KeyName()
		ktype, err := ctx.KeyType()
		if err != nil {
			return err
		}

		info, _, err := kmgr.Create(kname, constants.Password, ktype)
		if err != nil {
			return err
		}

		key, err = kmgr.Get(kname)
		if err != nil {
			return err
		}

		fmt.Printf("Key created: %v\n", X(info.Address()))
	}

	txclient, err := ctx.TxClient()
	if err != nil {
		return err
	}

	nonce, err := txclient.Nonce()
	if err != nil {
		return err
	}

	prov := &provider.Provider{}
	err = prov.Parse(args[0])
	if err != nil {
		return err
	}

	result, err := txclient.BroadcastTxCommit(&types.TxCreateProvider{
		Owner:      key.Address(),
		HostURI:    prov.HostURI,
		Attributes: prov.Attributes,
		Nonce:      nonce,
	})

	if err != nil {
		return err
	}

	fmt.Println(X(result.DeliverTx.Data))

	return nil
}

func runCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <provider>",
		Short: "respond to chain events",
		Args:  cobra.ExactArgs(1),
		RunE:  context.WithContext(context.RequireNode(doProviderRunCommand)),
	}
	return cmd
}

func doProviderRunCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	txclient, err := ctx.TxClient()
	if err != nil {
		return err
	}

	provider, err := base.DecodeString(args[0])
	if err != nil {
		return err
	}

	deployments := make(map[string][]types.LeaseID)

	handler := marketplace.NewBuilder().
		OnTxCreateOrder(func(tx *types.TxCreateOrder) {

			price, err := getPrice(ctx, tx.OrderID)
			if err != nil {
				ctx.Log().Error("error getting price", "error", err)
				return
			}

			// randomize price
			price = uint32(rand.Int31n(int32(price) + 1))

			ordertx := &types.TxCreateFulfillment{
				FulfillmentID: types.FulfillmentID{
					Deployment: tx.Deployment,
					Group:      tx.Group,
					Order:      tx.Seq,
					Provider:   provider,
				},
				Price: price,
			}

			fmt.Printf("Bidding on order: %v\n",
				keys.OrderID(tx.OrderID).Path())

			fmt.Printf("Fulfillment: %v\n",
				keys.FulfillmentID(ordertx.FulfillmentID).Path())

			_, err = txclient.BroadcastTxCommit(ordertx)
			if err != nil {
				ctx.Log().Error("error broadcasting tx", "error", err)
				return
			}

		}).
		OnTxCreateLease(func(tx *types.TxCreateLease) {
			leaseProvider, _ := tx.Provider.Marshal()
			if bytes.Equal(leaseProvider, provider) {
				leases, _ := deployments[tx.Deployment.EncodeString()]
				deployments[tx.Deployment.EncodeString()] = append(leases, tx.LeaseID)
				fmt.Printf("Won lease for order: %v\n",
					keys.LeaseID(tx.LeaseID).Path())
			}
		}).
		OnTxCloseDeployment(func(tx *types.TxCloseDeployment) {
			leases, ok := deployments[tx.Deployment.EncodeString()]
			if ok {
				for _, lease := range leases {
					fmt.Printf("Closed lease %v\n", keys.LeaseID(lease).Path())
				}
			}
		}).Create()
	return common.MonitorMarketplace(ctx.Log(), ctx.Client(), handler)
}

func getPrice(ctx context.Context, id types.OrderID) (uint32, error) {
	// get deployment group
	price := uint32(0)
	path := qp.DeploymentGroupPath(id.GroupID())
	group := new(types.DeploymentGroup)
	result, err := query.Query(ctx, path)
	if err != nil {
		return 0, err
	}
	group.Unmarshal(result.Response.Value)
	for _, group := range group.GetResources() {
		price += group.Price
	}
	return price, nil
}

func closeFulfillmentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "closef",
		Short: "close an open fulfillment",
		Args:  cobra.ExactArgs(1),
		RunE:  context.WithContext(context.RequireNode(doCloseFulfillmentCommand)),
	}

	context.AddFlagKeyType(cmd, cmd.Flags())

	return cmd
}

func doCloseFulfillmentCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	txclient, err := ctx.TxClient()
	if err != nil {
		return err
	}

	key, err := keys.ParseFulfillmentPath(args[0])
	if err != nil {
		return err
	}

	_, err = txclient.BroadcastTxCommit(&types.TxCloseFulfillment{
		FulfillmentID: key.ID(),
	})
	return err
}

func closeLeaseCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "closel <deployment> <group> <order> <provider>",
		Short: "close an active lease",
		Args:  cobra.ExactArgs(4),
		RunE:  context.WithContext(context.RequireNode(doCloseLeaseCommand)),
	}

	context.AddFlagKeyType(cmd, cmd.Flags())

	return cmd
}

func doCloseLeaseCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	txclient, err := ctx.TxClient()
	if err != nil {
		return err
	}

	key, err := keys.ParseLeasePath(args[0])
	if err != nil {
		return err
	}

	_, err = txclient.BroadcastTxCommit(&types.TxCloseLease{
		LeaseID: key.ID(),
	})

	return err
}

func runManifestServerCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "servmani [port] [loglevel = (debug|info|warn|error|fatal|panic)]",
		Short: "receive deployment manifest",
		Args:  cobra.RangeArgs(0, 2),
		RunE:  context.WithContext(context.RequireNode(doRunManifestServerCommand)),
	}

	context.AddFlagKeyType(cmd, cmd.Flags())

	return cmd
}

func doRunManifestServerCommand(ctx context.Context, cmd *cobra.Command, args []string) error {

	port := "3001"
	if len(args) == 1 {
		port = args[0]
	}

	return common.RunForever(func(gctx gcontext.Context) error {
		return manifest.RunServer(gctx, ctx.Log(), port)
	})
}
