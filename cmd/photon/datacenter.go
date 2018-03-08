package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/ovrclk/photon/cmd/photon/constants"
	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/spf13/cobra"
	"github.com/tendermint/go-wire/data"
	tmclient "github.com/tendermint/tendermint/rpc/client"
)

func datacenterCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "datacenter",
		Short: "manage datacenter",
		Args:  cobra.ExactArgs(1),
	}

	cmd.AddCommand(createDatacenterCommand())

	return cmd
}

func createDatacenterCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "create [file] [flags]",
		Short: "create a datacenter",
		Args:  cobra.ExactArgs(1),
		RunE:  context.WithContext(context.RequireNode(doCreateDatacenterCommand)),
	}

	context.AddFlagKeyType(cmd, cmd.Flags())
	context.AddFlagNode(cmd, cmd.Flags())
	context.AddFlagKey(cmd, cmd.Flags())
	context.AddFlagNonce(cmd, cmd.Flags())

	return cmd
}

func doCreateDatacenterCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	datacenter, err := parseDatacenter(args[0])
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

	address := doHash(key.Address, nonce)
	datacenter.Address = address
	datacenter.Owner = base.Bytes(key.Address)

	tx, err := txutil.BuildTx(kmgr, key.Name, constants.Password, nonce, &types.TxCreateDatacenter{
		Datacenter: datacenter,
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

	fmt.Println("Created datacenter: " + strings.ToUpper(hex.EncodeToString(datacenter.Address)))

	return nil
}

func parseDatacenter(file string) (types.Datacenter, error) {
	// todo: read and parse deployment yaml file

	/* begin stub data */
	resourceunit := &types.ResourceUnit{
		Cpu:    1,
		Memory: 1,
		Disk:   1,
	}

	resourcegroup := &types.ResourceGroup{
		Unit:  *resourceunit,
		Count: 1,
		Price: 1,
	}

	providerattribute := &types.ProviderAttribute{
		Name:  "region",
		Value: "us-west",
	}

	attributes := []types.ProviderAttribute{*providerattribute}
	resources := []types.ResourceGroup{*resourcegroup}

	datacenter := &types.Datacenter{
		Resources:  resources,
		Attributes: attributes,
	}

	/* end stub data */

	return *datacenter, nil
}
