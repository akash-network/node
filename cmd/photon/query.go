package main

import (
	"errors"
	"fmt"

	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	tmclient "github.com/tendermint/tendermint/rpc/client"
)

func queryCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "query [account]",
		Short: "query account",
		Args:  cobra.ExactArgs(1),
		RunE:  withContext(requireNode(doQueryCommand)),
	}

	cmd.Flags().StringP(flagNode, "n", defaultNode, "node host")
	viper.BindPFlag(flagNode, cmd.Flags().Lookup(flagNode))

	return cmd
}

func doQueryCommand(ctx Context, cmd *cobra.Command, args []string) error {

	res := new(types.Account)

	account := args[0]
	if len(args[0]) < 1 {
		return errors.New("account invalid. Too $hort")
	}

	client := tmclient.NewHTTP(ctx.Node(), "/websocket")

	queryPath := state.AccountPath + account

	result, _ := client.ABCIQuery(queryPath, nil)

	res.Unmarshal(result.Response.Value)

	fmt.Println(string(result.Response.Key))
	fmt.Println(res)

	println(res)

	return nil
}
