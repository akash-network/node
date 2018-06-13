package query

import (
	"encoding/json"
	"os"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/spf13/cobra"
)

func QueryCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "query [something]",
		Short: "query something",
		Args:  cobra.ExactArgs(1),
	}

	session.AddFlagNode(cmd, cmd.PersistentFlags())

	cmd.AddCommand(
		queryAccountCommand(),
		queryDeploymentCommand(),
		queryDeploymentGroupCommand(),
		queryProviderCommand(),
		queryOrderCommand(),
		queryFulfillmentCommand(),
		queryLeaseCommand(),
	)

	return cmd
}

func handleMessage(obj proto.Message, err error) error {
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	os.Stdout.Write(data)
	os.Stdout.Write([]byte("\n"))
	return nil
}
