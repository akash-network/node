package main

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"

	"github.com/ovrclk/akash/cmd/akash/constants"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/marketplace"
	"github.com/ovrclk/akash/types"
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

	session.AddFlagNode(cmd, cmd.PersistentFlags())
	session.AddFlagKey(cmd, cmd.PersistentFlags())
	session.AddFlagNonce(cmd, cmd.PersistentFlags())

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
		RunE:  session.WithSession(session.RequireNode(doCreateProviderCommand)),
	}

	session.AddFlagKeyType(cmd, cmd.Flags())

	return cmd
}

func doCreateProviderCommand(session session.Session, cmd *cobra.Command, args []string) error {
	kmgr, err := session.KeyManager()
	if err != nil {
		return err
	}

	// XXX generate key for provider if doens't exist
	key, err := session.Key()
	if err != nil {
		kname := session.KeyName()
		ktype, err := session.KeyType()
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

	txclient, err := session.TxClient()
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
		RunE:  session.WithSession(session.RequireNode(doProviderRunCommand)),
	}
	return cmd
}

func doProviderRunCommand(session session.Session, cmd *cobra.Command, args []string) error {
	txclient, err := session.TxClient()
	if err != nil {
		return err
	}

	key, err := keys.ParseProviderPath(args[0])
	if err != nil {
		return err
	}

	deployments := make(map[string][]types.LeaseID)

	handler := marketplace.NewBuilder().
		OnTxCreateOrder(func(tx *types.TxCreateOrder) {

			price, err := getPrice(session, tx.OrderID)
			if err != nil {
				session.Log().Error("error getting price", "error", err)
				return
			}

			// randomize price
			price = uint32(rand.Int31n(int32(price) + 1))

			ordertx := &types.TxCreateFulfillment{
				FulfillmentID: types.FulfillmentID{
					Deployment: tx.Deployment,
					Group:      tx.Group,
					Order:      tx.Seq,
					Provider:   key.ID(),
				},
				Price: price,
			}

			fmt.Printf("Bidding on order: %v\n",
				keys.OrderID(tx.OrderID).Path())

			fmt.Printf("Fulfillment: %v\n",
				keys.FulfillmentID(ordertx.FulfillmentID).Path())

			_, err = txclient.BroadcastTxCommit(ordertx)
			if err != nil {
				session.Log().Error("error broadcasting tx", "error", err)
				return
			}

		}).
		OnTxCreateLease(func(tx *types.TxCreateLease) {
			leaseProvider, _ := tx.Provider.Marshal()
			if bytes.Equal(leaseProvider, key.ID()) {
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
	return common.MonitorMarketplace(session.Log(), session.Client(), handler)
}

func getPrice(session session.Session, id types.OrderID) (uint32, error) {
	// get deployment group
	price := uint32(0)
	group, err := session.QueryClient().DeploymentGroup(session.Ctx(), id.GroupID())
	if err != nil {
		return 0, err
	}
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
		RunE:  session.WithSession(session.RequireNode(doCloseFulfillmentCommand)),
	}

	session.AddFlagKeyType(cmd, cmd.Flags())

	return cmd
}

func doCloseFulfillmentCommand(session session.Session, cmd *cobra.Command, args []string) error {
	txclient, err := session.TxClient()
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
		RunE:  session.WithSession(session.RequireNode(doCloseLeaseCommand)),
	}

	session.AddFlagKeyType(cmd, cmd.Flags())

	return cmd
}

func doCloseLeaseCommand(session session.Session, cmd *cobra.Command, args []string) error {
	txclient, err := session.TxClient()
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
		RunE:  session.WithSession(session.RequireNode(doRunManifestServerCommand)),
	}

	session.AddFlagKeyType(cmd, cmd.Flags())

	return cmd
}

func doRunManifestServerCommand(session session.Session, cmd *cobra.Command, args []string) error {

	port := "3001"
	if len(args) == 1 {
		port = args[0]
	}

	return common.RunForever(func(ctx context.Context) error {
		return manifest.RunServer(ctx, session.Log(), port)
	})
}
