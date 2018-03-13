package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/ovrclk/photon/cmd/common"
	"github.com/ovrclk/photon/cmd/photon/constants"
	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/ovrclk/photon/marketplace"
	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
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
	cmd.AddCommand(createRunCommand())

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
	provider, err := parseProvider(args[0])
	if err != nil {
		return err
	}

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

		fmt.Println("Key created: ", addr)
	}

	nonce, err := ctx.Nonce()
	if err != nil {
		return err
	}

	address := state.ProviderAddress(key.Address, nonce)
	provider.Address = address
	provider.Owner = base.Bytes(key.Address)

	signer := txutil.NewKeystoreSigner(kmgr, key.Name, constants.Password)

	tx, err := txutil.BuildTx(signer, nonce, &types.TxCreateProvider{
		Provider: provider,
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

func parseProvider(file string) (types.Provider, error) {
	// todo: read and parse deployment yaml file

	/* begin stub data */
	providerattribute := &types.ProviderAttribute{
		Name:  "region",
		Value: "us-west",
	}

	attributes := []types.ProviderAttribute{*providerattribute}

	provider := &types.Provider{
		Attributes: attributes,
	}

	/* end stub data */

	return *provider, nil
}

func createRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "run <provider>",
		Args: cobra.ExactArgs(1),
		RunE: context.WithContext(context.RequireNode(doProviderRunCommand)),
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

			ordertx := &types.TxCreateFulfillment{
				Order: &types.Fulfillment{
					Deployment: tx.Order.Deployment,
					Group:      tx.Order.Group,
					Order:      tx.Order.Order,
					Provider:   *provider,
				},
			}

			//time.Sleep(time.Second * 5)
			fmt.Printf("BIDDING ON ORDER: %X/%v/%v\n",
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

		}).Create()

	return common.MonitorMarketplace(ctx.Log(), ctx.Client(), handler)
}
