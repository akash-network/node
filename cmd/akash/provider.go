package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"strconv"

	"github.com/ovrclk/akash/cmd/akash/constants"
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/cmd/akash/query"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/marketplace"
	qp "github.com/ovrclk/akash/query"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	. "github.com/ovrclk/akash/util"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
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

	return cmd
}

func createProviderCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "create [file] [flags]",
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

	signer, key, err := ctx.Signer()
	if err != nil {
		return err
	}

	nonce, err := ctx.Nonce()
	if err != nil {
		return err
	}

	attributes, err := parseProvider(args[0], nonce)
	if err != nil {
		return err
	}

	tx, err := txutil.BuildTx(signer, nonce, &types.TxCreateProvider{
		Owner:      key.Address(),
		Attributes: *attributes,
		Nonce:      nonce,
	})
	if err != nil {
		return err
	}

	client := ctx.Client()

	result, err := client.BroadcastTxCommit(tx)
	if err != nil {
		return err
	}
	if result.CheckTx.IsErr() {
		return errors.New(result.CheckTx.GetLog())
	}
	if result.DeliverTx.IsErr() {
		return errors.New(result.DeliverTx.GetLog())
	}

	fmt.Println(X(result.DeliverTx.Data))

	return nil
}

func parseProvider(file string, nonce uint64) (*[]types.ProviderAttribute, error) {

	contents, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	attributes := &[]types.ProviderAttribute{}
	err = yaml.Unmarshal([]byte(contents), attributes)
	if err != nil {
		return nil, err
	}

	return attributes, nil
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
	client := ctx.Client()

	provider := new(base.Bytes)
	if err := provider.DecodeString(args[0]); err != nil {
		return err
	}

	signer, _, err := ctx.Signer()
	if err != nil {
		return err
	}

	deployments := make(map[string][]string)

	handler := marketplace.NewBuilder().
		OnTxCreateOrder(func(tx *types.TxCreateOrder) {

			nonce, err := ctx.Nonce()
			if err != nil {
				ctx.Log().Error("error getting nonce", "error", err)
				return
			}

			price, err := getPrice(ctx, tx.Deployment, tx.Group)
			if err != nil {
				ctx.Log().Error("error getting price", "error", err)
				return
			}

			// randomize price
			price = uint32(rand.Int31n(int32(price) + 1))

			ordertx := &types.TxCreateFulfillment{
				Deployment: tx.Deployment,
				Group:      tx.Group,
				Order:      tx.Seq,
				Provider:   *provider,
				Price:      price,
			}

			fmt.Printf("Bidding on order: %v/%v/%v\n",
				X(tx.Deployment), tx.Group, tx.Seq)

			fmt.Printf("Fulfillment: %v\n",
				X(state.FulfillmentID(tx.Deployment, tx.Group, tx.Seq, *provider)))

			txbuf, err := txutil.BuildTx(signer, nonce, ordertx)
			if err != nil {
				ctx.Log().Error("error building tx", "error", err)
				return
			}

			resp, err := client.BroadcastTxCommit(txbuf)
			if err != nil {
				ctx.Log().Error("error broadcasting tx", "error", err)
				return
			}
			if resp.CheckTx.IsErr() {
				ctx.Log().Error("CheckTx error", "error", resp.CheckTx.Log)
				return
			}
			if resp.DeliverTx.IsErr() {
				ctx.Log().Error("DeliverTx error", "error", resp.DeliverTx.Log)
				return
			}

		}).
		OnTxCreateLease(func(tx *types.TxCreateLease) {
			leaseProvider, _ := tx.Provider.Marshal()
			if bytes.Equal(leaseProvider, *provider) {
				lease := X(state.LeaseID(tx.Deployment, tx.Group, tx.Order, tx.Provider))
				leases, _ := deployments[tx.Deployment.EncodeString()]
				deployments[tx.Deployment.EncodeString()] = append(leases, lease)
				fmt.Printf("Won lease for order: %v/%v/%v\n",
					X(tx.Deployment), tx.Group, tx.Order)
			}
		}).
		OnTxCloseDeployment(func(tx *types.TxCloseDeployment) {
			leases, ok := deployments[tx.Deployment.EncodeString()]
			if ok {
				for _, lease := range leases {
					fmt.Printf("Closed lease %v\n", lease)
				}
			}
		}).Create()
	return common.MonitorMarketplace(ctx.Log(), ctx.Client(), handler)
}

func getPrice(ctx context.Context, addr base.Bytes, seq uint64) (uint32, error) {
	// get deployment group
	price := uint32(0)
	path := qp.DeploymentGroupPath(addr, seq)
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
	signer, _, err := ctx.Signer()
	if err != nil {
		return err
	}

	nonce, err := ctx.Nonce()
	if err != nil {
		return err
	}

	fulfillment := new(base.Bytes)
	if err := fulfillment.DecodeString(args[0]); err != nil {
		return err
	}

	tx, err := txutil.BuildTx(signer, nonce, &types.TxCloseFulfillment{
		Fulfillment: *fulfillment,
	})
	if err != nil {
		return err
	}

	result, err := ctx.Client().BroadcastTxCommit(tx)
	if err != nil {
		return err
	}
	if result.CheckTx.IsErr() {
		return errors.New(result.CheckTx.GetLog())
	}
	if result.DeliverTx.IsErr() {
		return errors.New(result.DeliverTx.GetLog())
	}

	return nil
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
	signer, _, err := ctx.Signer()
	if err != nil {
		return err
	}

	nonce, err := ctx.Nonce()
	if err != nil {
		return err
	}

	deployment := new(base.Bytes)
	if err := deployment.DecodeString(args[0]); err != nil {
		return err
	}

	group, err := strconv.ParseUint(args[1], 10, 64)
	if err != nil {
		return err
	}

	order, err := strconv.ParseUint(args[2], 10, 64)
	if err != nil {
		return err
	}

	provider := new(base.Bytes)
	if err := provider.DecodeString(args[3]); err != nil {
		return err
	}

	lease := state.LeaseID(*deployment, group, order, *provider)

	tx, err := txutil.BuildTx(signer, nonce, &types.TxCloseLease{
		Lease: lease,
	})
	if err != nil {
		return err
	}

	result, err := ctx.Client().BroadcastTxCommit(tx)
	if err != nil {
		return err
	}
	if result.CheckTx.IsErr() {
		return errors.New(result.CheckTx.GetLog())
	}
	if result.DeliverTx.IsErr() {
		return errors.New(result.DeliverTx.GetLog())
	}

	return nil
}
