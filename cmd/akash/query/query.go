package query

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/spf13/cobra"
	tmclient "github.com/tendermint/tendermint/rpc/client"
	core_types "github.com/tendermint/tendermint/rpc/core/types"
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
		queryLeaseCommand(),
	)

	return cmd
}

func Query(ctx context.Context, path string) (*core_types.ResultABCIQuery, error) {
	client := tmclient.NewHTTP(ctx.Node(), "/websocket")
	result, err := client.ABCIQuery(path, nil)
	if err != nil {
		return result, err
	}
	if result.Response.IsErr() {
		return result, errors.New(result.Response.GetLog())
	}
	return result, nil
}

func doQuery(ctx context.Context, path string, obj proto.Message) error {
	result, err := Query(ctx, path)
	if err != nil {
		return err
	}

	if err := proto.Unmarshal(result.Response.Value, obj); err != nil {
		return err
	}

	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}
