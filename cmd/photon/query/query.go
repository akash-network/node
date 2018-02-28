package query

import (
	"encoding/json"
	"errors"

	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/ovrclk/photon/types"
	"github.com/spf13/cobra"
	tmclient "github.com/tendermint/tendermint/rpc/client"
)

func QueryCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "query [something]",
		Short: "query something",
		Args:  cobra.ExactArgs(1),
	}

	context.AddFlagNode(cmd, cmd.PersistentFlags())

	cmd.AddCommand(
		queryAccountCommand(),
		queryDeploymentCommand(),
		queryDatacenterCommand())

	return cmd
}

func doQuery(ctx context.Context, path string, structure interface{}) error {

	client := tmclient.NewHTTP(ctx.Node(), "/websocket")
	result, _ := client.ABCIQuery(path, nil)
	if result.Response.IsErr() {
		return errors.New(result.Response.Error())
	}

	switch s := structure.(type) {
	case *types.Account:
		s.Unmarshal(result.Response.Value)
	case *types.Deployment:
		s.Unmarshal(result.Response.Value)
	case *types.Deployments:
		s.Unmarshal(result.Response.Value)
	case *types.Datacenter:
		s.Unmarshal(result.Response.Value)
	case *types.Datacenters:
		s.Unmarshal(result.Response.Value)
	default:
		return errors.New("Unknown query value structure")
	}

	data, _ := json.MarshalIndent(structure, "", "  ")

	println("path: " + path)
	println("response: " + string(data))

	return nil
}
