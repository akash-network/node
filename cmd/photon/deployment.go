package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ovrclk/photon/cmd/photon/constants"
	"github.com/ovrclk/photon/cmd/photon/context"
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
		RunE: context.WithContext(
			context.RequireKey(context.RequireNode(doDeployCommand))),
	}

	cmd.Flags().StringP(constants.FlagNode, "n", constants.DefaultNode, "node host")
	viper.BindPFlag(constants.FlagNode, cmd.Flags().Lookup(constants.FlagNode))

	cmd.Flags().StringP(constants.FlagKey, "k", "", "key name (required)")
	cmd.MarkFlagRequired(constants.FlagKey)

	cmd.Flags().Uint64(constants.FlagNonce, 0, "nonce (optional)")

	return cmd
}

func doDeployCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	kmgr, _ := ctx.KeyManager()
	key, _ := ctx.Key()

	nonce, err := ctx.Nonce()
	if err != nil {
		return err
	}

	hash := doHash(key.Address, nonce)

	deployment, _ := parseDeployment(args[0], hash)

	tx, err := txutil.BuildTx(kmgr, key.Name, constants.Password, nonce, &types.TxDeployment{
		From:       base.Bytes(key.Address),
		Deployment: &deployment,
	})
	if err != nil {
		return err
	}

	client := tmclient.NewHTTP(ctx.Node(), "/websocket")

	result, err := client.BroadcastTxCommit(tx)
	if err != nil {
		return err
	}
	fmt.Println(result)
	fmt.Println("Created deployment: " + strings.ToUpper(hex.EncodeToString(deployment.Address)))

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

func doHash(address []byte, nonce uint64) []byte {
	nbytes := make([]byte, 10)
	binary.LittleEndian.PutUint64(nbytes, nonce)
	data := append(address, nbytes...)
	hash32 := sha256.Sum256(data)
	return hash32[:32]
}
