package main

import (
	"errors"
	"fmt"

	"github.com/ovrclk/photon/cmd/photon/constants"
	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/spf13/cobra"
)

func deploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "deploy [file]",
		Short: "post a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: context.WithContext(
			context.RequireKey(context.RequireNode(doDeployCommand))),
	}

	context.AddFlagNode(cmd, cmd.Flags())
	context.AddFlagKey(cmd, cmd.Flags())
	context.AddFlagNonce(cmd, cmd.Flags())

	return cmd
}

func doDeployCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	kmgr, _ := ctx.KeyManager()
	key, _ := ctx.Key()

	nonce, err := ctx.Nonce()
	if err != nil {
		return err
	}

	hash := state.DeploymentAddress(key.Address, nonce)

	deployment, _ := parseDeployment(args[0], hash)
	deployment.Tenant = base.Bytes(key.Address)

	signer := txutil.NewKeystoreSigner(kmgr, key.Name, constants.Password)

	tx, err := txutil.BuildTx(signer, nonce, &types.TxCreateDeployment{
		Deployment: &deployment,
	})
	if err != nil {
		return err
	}

	res, err := ctx.Client().BroadcastTxCommit(tx)
	if err != nil {
		ctx.Log().Error("error sending tx", err)
		return err
	}
	if !res.CheckTx.IsOK() {
		ctx.Log().Error("error delivering tx", "err", res.CheckTx.GetLog())
		return errors.New(res.CheckTx.GetLog())
	}
	if !res.DeliverTx.IsOK() {
		ctx.Log().Error("error delivering tx", "err", res.DeliverTx.GetLog())
		return errors.New(res.DeliverTx.GetLog())
	}
	fmt.Printf("Created deployment: %X\n", deployment.Address)

	return nil
}

func parseDeployment(file string, hash []byte) (types.Deployment, error) {
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

	requirements := []types.ProviderAttribute{*providerattribute}
	resources := []types.ResourceGroup{*resourcegroup}

	deploymentgroup := &types.DeploymentGroup{
		Requirements: requirements,
		Resources:    resources,
	}

	groups := []types.DeploymentGroup{*deploymentgroup}

	deployment := &types.Deployment{
		Address: hash,
		Groups:  groups,
	}
	/* end stub data */

	return *deployment, nil
}
