package query

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/ovrclk/akash/util"
	"github.com/spf13/cobra"
)

func queryLeaseCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "lease [deployment]",
		Short: "query lease",
		RunE:  context.WithContext(context.RequireNode(doQueryLeaseCommand)),
	}

	return cmd
}

func doQueryLeaseCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	path := state.LeasePath
	if len(args) > 0 {
		structure := new(types.Lease)
		path += args[0]
		return doQuery(ctx, path, structure)
	} else {
		structure := new(types.Leases)
		return doQuery(ctx, path, structure)
	}
}

func LeasesForDeployment(ctx context.Context, deployment *base.Bytes) (*types.Leases, error) {
	leases := &types.Leases{}
	result, err := Query(ctx, util.X(*deployment))
	if err != nil {
		return nil, err
	}
	if err := proto.Unmarshal(result.Response.Value, leases); err != nil {
		return nil, err
	}
	return leases, nil
}
