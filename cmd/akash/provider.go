package main

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"

	"github.com/ovrclk/akash/cmd/akash/constants"
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/cmd/akash/query"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/marketplace"
	qp "github.com/ovrclk/akash/query"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
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
		Attributes: attributes,
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

func parseProvider(file string, nonce uint64) ([]types.ProviderAttribute, error) {
	// todo: read and parse deployment yaml file

	/* begin stub data */
	provider := testutil.Provider(*new(base.Bytes), nonce)
	/* end stub data */

	return provider.Attributes, nil
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

	deployments := make(map[string]struct{})

	handler := marketplace.NewBuilder().
		OnTxCreateOrder(func(tx *types.TxCreateOrder) {

			nonce, err := ctx.Nonce()
			if err != nil {
				ctx.Log().Error("error getting nonce", "error", err)
				return
			}

			price, err := getPrice(ctx, tx.Order.Deployment, tx.Order.Group)
			if err != nil {
				ctx.Log().Error("error getting price", "error", err)
				return
			}

			// randomize price
			price = uint32(rand.Int31n(int32(price) + 1))

			ordertx := &types.TxCreateFulfillment{
				Fulfillment: &types.Fulfillment{
					Deployment: tx.Order.Deployment,
					Group:      tx.Order.Group,
					Order:      tx.Order.Order,
					Provider:   *provider,
					Price:      price,
				},
			}

			fmt.Printf("Bidding on order: %v/%v/%v\n",
				X(tx.Order.Deployment), tx.Order.Group, tx.Order.Order)

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
			leaseProvider, _ := tx.Lease.Provider.Marshal()
			if bytes.Equal(leaseProvider, *provider) {
				deployments[tx.Lease.Deployment.EncodeString()] = struct{}{}
				fmt.Printf("Won lease for order: %v/%v/%v\n",
					X(tx.Lease.Deployment), tx.Lease.Group, tx.Lease.Order)
			}
		}).
		OnTxDeploymentClosed(func(tx *types.TxDeploymentClosed) {
			_, ok := deployments[tx.Deployment.EncodeString()]
			if ok {
				fmt.Printf("Closed lease for deployment: %v\n", X(tx.Deployment))
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
