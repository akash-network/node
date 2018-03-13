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
		queryProviderCommand(),
		queryOrderCommand(),
	)

	return cmd
}

func doQuery(ctx context.Context, path string, structure interface{}) error {

	client := tmclient.NewHTTP(ctx.Node(), "/websocket")
	result, err := client.ABCIQuery(path, nil)
	if err != nil {
		return err
	}
	if result.Response.IsErr() {
		return errors.New(result.Response.GetLog())
	}

	switch s := structure.(type) {
	case *types.Account:
		s.Unmarshal(result.Response.Value)
	case *types.Deployment:
		s.Unmarshal(result.Response.Value)
	case *types.Deployments:
		s.Unmarshal(result.Response.Value)
	case *types.Provider:
		s.Unmarshal(result.Response.Value)
	case *types.Providers:
		s.Unmarshal(result.Response.Value)
	case *types.Order:
		s.Unmarshal(result.Response.Value)
	case *types.Orders:
		s.Unmarshal(result.Response.Value)
	default:
		return errors.New("Unknown query value structure")
	}

	data, _ := json.MarshalIndent(structure, "", "  ")

	println("path: " + path)
	println("response:\n" + string(data))

	return nil
}
