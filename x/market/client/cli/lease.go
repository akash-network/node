package cli

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/spf13/cobra"
)

func cmdGetLeases() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Query for all leases",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)
			clientCtx, err := client.ReadQueryCommandFlags(clientCtx, cmd.Flags())
			if err != nil {
				return err
			}

			queryClient := types.NewQueryClient(clientCtx)

			lfilters, state, err := LeaseFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			// checking state flag
			stateVal, ok := types.Lease_State_value[state]

			if (!ok && (state != "")) || state == "invalid" {
				return ErrStateValue
			}

			lfilters.State = types.Lease_State(stateVal)

			pageReq, err := client.ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			params := &types.QueryLeasesRequest{
				Filters:    lfilters,
				Pagination: pageReq,
			}

			res, err := queryClient.Leases(context.Background(), params)
			if err != nil {
				return err
			}

			return clientCtx.PrintOutput(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	flags.AddPaginationFlagsToCmd(cmd, "leases")
	AddLeaseFilterFlags(cmd.Flags())

	return cmd
}

func cmdGetLease() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Query order",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)
			clientCtx, err := client.ReadQueryCommandFlags(clientCtx, cmd.Flags())
			if err != nil {
				return err
			}

			queryClient := types.NewQueryClient(clientCtx)

			bidID, err := BidIDFromFlagsWithoutCtx(cmd.Flags())
			if err != nil {
				return err
			}

			res, err := queryClient.Lease(context.Background(), &types.QueryLeaseRequest{ID: types.MakeLeaseID(bidID)})
			if err != nil {
				return err
			}

			return clientCtx.PrintOutput(&res.Lease)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	AddQueryBidIDFlags(cmd.Flags())
	MarkReqBidIDFlags(cmd)

	return cmd
}
