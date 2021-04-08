package cli

import (
	"context"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
	"github.com/ovrclk/akash/x/escrow/types"
	"github.com/spf13/viper"

	marketTypes "github.com/ovrclk/akash/x/market/types"
	deploymentTypes "github.com/ovrclk/akash/x/deployment/types"
)

func GetQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                       types.ModuleName,
		Short:                      "Escrow query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		cmdBlocksRemaining(),
	)

	return cmd
}

func cmdBlocksRemaining() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blocks-remaining",
		Short: "Compute the number of blocks remaining for an ecrow account",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			owner := viper.GetString("owner")
			dseq := viper.GetUint64("dseq")
			marketClient := marketTypes.NewQueryClient(clientCtx)
			ctx := context.Background()

			// Fetch leases matching owner & dseq
			leaseRequest := marketTypes.QueryLeasesRequest{
				Filters:    marketTypes.LeaseFilters{
					Owner:    owner,
					DSeq:     dseq,
					GSeq:     0,
					OSeq:     0,
					Provider: "",
					State:    "active",
				},
				Pagination: nil,
			}

			leasesResponse, err := marketClient.Leases(ctx, &leaseRequest)
			if err != nil {
				return err
			}

			leases := make([]marketTypes.Lease, 0)
			for _, lease := range leasesResponse.Leases {
				leases = append(leases, lease.Lease)
			}

			// Fetch the balance of the escrow account
			deploymentClient := deploymentTypes.NewQueryClient(clientCtx)

			for _, lease := range leases {
				//  Fetch the time of last settlement
				res, err := deploymentClient.Deployment(ctx, &deploymentTypes.QueryDeploymentRequest{
					ID: lease.LeaseID.DeploymentID(),
				})
				if err != nil {
					return err
				}
				createdAt := lease.CreatedAt
				balance := res.EscrowAccount.Balance
				settledAt := res.EscrowAccount.SettledAt
				blockchainHeight := clientCtx.Height

				err = clientCtx.PrintString("example")
				if err != nil {
					return err
				}
			}

			return nil
		},
	}

	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}
