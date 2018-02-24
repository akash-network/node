package query

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ovrclk/photon/cmd/photon/constants"
	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/ovrclk/photon/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	tmclient "github.com/tendermint/tendermint/rpc/client"
)

func QueryCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "query [something]",
		Short: "query something",
		Args:  cobra.ExactArgs(1),
	}

	cmd.Flags().StringP(constants.FlagNode, "n", constants.DefaultNode, "node host")
	viper.BindPFlag(constants.FlagNode, cmd.Flags().Lookup(constants.FlagNode))

	cmd.AddCommand(queryAccountCommand(), queryDeploymentCommand())

	return cmd
}

func doQuery(ctx context.Context, path string, structure interface{}) error {

	client := tmclient.NewHTTP(ctx.Node(), "/websocket")
	result, _ := client.ABCIQuery(path, nil)
	if result.Response.IsErr() {
		return result.Response.Error()
	}

	switch s := structure.(type) {
	case *types.Account:
		s.Unmarshal(result.Response.Value)
	case *types.Deployment:
		s.Unmarshal(result.Response.Value)
	case *types.Deployments:
		s.Unmarshal(result.Response.Value)
	default:
		return errors.New("Unknown query value structure")
	}

	data, _ := json.MarshalIndent(structure, "", "  ")

	println("path: " + path)
	println("response: " + string(data))

	return nil
}
