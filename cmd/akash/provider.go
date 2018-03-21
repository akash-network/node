package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/ovrclk/akash/cmd/akash/constants"
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/cmd/akash/query"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/marketplace"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/spf13/cobra"
	"github.com/tendermint/go-wire/data"
	tmclient "github.com/tendermint/tendermint/rpc/client"
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

	key, err := ctx.Key()
	if err != nil {
		kname, _ := cmd.Flags().GetString(constants.FlagKey)
		ktype, err := cmd.Flags().GetString(constants.FlagKeyType)
		if err != nil {
			return err
		}

		info, _, err := kmgr.Create(kname, constants.Password, ktype)
		if err != nil {
			return err
		}

		addr, err := data.ToText(info.Address)
		if err != nil {
			return err
		}
		key, err = kmgr.Get(kname)
		if err != nil {
			return err
		}

		fmt.Println("Key created:", addr)
	}

	nonce, err := ctx.Nonce()
	if err != nil {
		return err
	}

	provider, err := parseProvider(args[0], key.Address, nonce)
	if err != nil {
		return err
	}

	signer := txutil.NewKeystoreSigner(kmgr, key.Name, constants.Password)

	tx, err := txutil.BuildTx(signer, nonce, &types.TxCreateProvider{
		Provider: *provider,
	})
	if err != nil {
		return err
	}

	client := tmclient.NewHTTP(ctx.Node(), "/websocket")

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

	fmt.Println("Created provider: " + strings.ToUpper(hex.EncodeToString(provider.Address)))

	return nil
}

func parseProvider(file string, tenant []byte, nonce uint64) (*types.Provider, error) {
	// todo: read and parse deployment yaml file

	/* begin stub data */
	provider := testutil.Provider(tenant, nonce)
	/* end stub data */

	return provider, nil
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
	kmgr, _ := ctx.KeyManager()

	client := ctx.Client()

	provider := new(base.Bytes)

	if err := provider.DecodeString(args[0]); err != nil {
		return err
	}

	key, err := ctx.Key()
	if err != nil {
		return err
	}

	signer := txutil.NewKeystoreSigner(kmgr, key.Name, constants.Password)

	handler := marketplace.NewBuilder().
		OnTxCreateOrder(func(tx *types.TxCreateOrder) {

			nonce, err := ctx.Nonce()
			if err != nil {
				ctx.Log().Error("error getting nonce", err)
				return
			}

			price, err := getPrice(ctx, tx.Order.Deployment, tx.Order.Group)
			if err != nil {
				ctx.Log().Error("error getting price", err)
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

			fmt.Printf("Bidding on order: %X/%v/%v\n",
				tx.Order.Deployment, tx.Order.Group, tx.Order.Order)

			txbuf, err := txutil.BuildTx(signer, nonce, ordertx)
			if err != nil {
				ctx.Log().Error("error building tx", err)
				return
			}

			resp, err := client.BroadcastTxCommit(txbuf)
			if err != nil {
				ctx.Log().Error("error broadcasting tx", err)
				return
			}
			if resp.CheckTx.IsErr() {
				ctx.Log().Error("CheckTx error", "err", resp.CheckTx.Log)
				return
			}
			if resp.DeliverTx.IsErr() {
				ctx.Log().Error("DeliverTx error", "err", resp.DeliverTx.Log)
				return
			}

		}).
		OnTxCreateLease(func(tx *types.TxCreateLease) {
			leaseProvider, _ := tx.Lease.Provider.Marshal()
			if bytes.Equal(leaseProvider, *provider) {
				fmt.Printf("Won lease for order: %X/%v/%v\n",
					tx.Lease.Deployment, tx.Lease.Group, tx.Lease.Order)
			}
		}).Create()

	return common.MonitorMarketplace(ctx.Log(), ctx.Client(), handler)
}

func getPrice(ctx context.Context, addr base.Bytes, seq uint64) (uint32, error) {
	// get deployment group
	price := uint32(0)
	path := state.DeploymentGroupPath + hex.EncodeToString(state.DeploymentGroupID(addr, seq))
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
