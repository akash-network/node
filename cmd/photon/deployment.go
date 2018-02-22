package main

import (
	"fmt"

	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	tmclient "github.com/tendermint/tendermint/rpc/client"
)

func deploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "deploy [file]",
		Short: "post a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: withContext(
			requireKey(requireNode(doDeployCommand))),
	}

	cmd.Flags().StringP(flagNode, "n", defaultNode, "node host")
	viper.BindPFlag(flagNode, cmd.Flags().Lookup(flagNode))

	cmd.Flags().StringP(flagKey, "k", "", "key name (required)")
	cmd.MarkFlagRequired(flagKey)

	cmd.Flags().Uint64(flagNonce, 0, "nonce (optional)")

	return cmd
}

func doDeployCommand(ctx Context, cmd *cobra.Command, args []string) error {
	kmgr, _ := ctx.KeyManager()
	key, _ := ctx.Key()

	deployment, _ := parseDeployment(args[0])

	nonce := ctx.Nonce()

	tx, err := txutil.BuildTx(kmgr, key.Name, password, nonce, &types.TxDeployment{
		From:       base.Bytes(key.Address),
		Deployment: &deployment,
	})
	if err != nil {
		return err
	}

	// todo: this should be abstracted, each tx will have this same code
	client := tmclient.NewHTTP(ctx.Node(), "/websocket")

	result, err := client.BroadcastTxCommit(tx)
	if err != nil {
		return err
	}
	fmt.Println(result)

	return nil
}

func parseDeployment(file string) (types.Deployment, error) {
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
		Groups: groups,
	}
	/* end stub data */

	return *deployment, nil
}
