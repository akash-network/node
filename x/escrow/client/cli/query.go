package cli

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	cosmosTypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/deployment/client/cli"
	deploymentTypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/escrow/types"
	marketTypes "github.com/ovrclk/akash/x/market/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func GetQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Escrow query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		cmdBlocksRemaining(),
	)

	return cmd
}

var errNoLeaseMatches = errors.New("leases for deployment do not exist")

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

			owner, err := cmd.Flags().GetString("owner")
			if err != nil {
				return err
			}

			dseq, err := cmd.Flags().GetUint64("dseq")
			if err != nil {
				return err
			}

			marketClient := marketTypes.NewQueryClient(clientCtx)
			ctx := context.Background()

			// Fetch leases matching owner & dseq
			leaseRequest := marketTypes.QueryLeasesRequest{
				Filters: marketTypes.LeaseFilters{
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
			totalLeaseAmount := cosmosTypes.NewInt(0)
			blockchainHeight, err := cli.CurrentBlockHeight(clientCtx)
			if err != nil {
				return err
			}
			if 0 == len(leases) {
				return errNoLeaseMatches
			}
			for _, lease := range leases {
				//  Fetch the time of last settlement

				amount := lease.Price.Amount
				totalLeaseAmount = totalLeaseAmount.Add(amount)

			}
			res, err := deploymentClient.Deployment(ctx, &deploymentTypes.QueryDeploymentRequest{
				ID: deploymentTypes.DeploymentID{Owner: owner, DSeq: dseq},
			})
			if err != nil {
				return err
			}
			balance := res.EscrowAccount.Balance.Amount
			settledAt := res.EscrowAccount.SettledAt
			if err != nil {
				return err
			}
			balanceRemain := balance.Int64() - ((int64(blockchainHeight) - settledAt) * (totalLeaseAmount.Int64()))
			blocksRemain := balanceRemain / totalLeaseAmount.Int64()
			const secondsPerDay = 24 * 60 * 60
			const secondsPerBlock = 6.5
			// Calculate blocks per day by using 6.5 seconds as average block-time
			blocksPerDay := (secondsPerDay / secondsPerBlock)
			estimatedDaysRemain := blocksRemain / int64(blocksPerDay)
			output := struct {
				BalanceRemain       int64 `json:"balance_remaining" yaml:"balance_remaining"`
				BlocksRemain        int64 `json:"blocks_remaining" yaml:"blocks_remaining"`
				EstimatedDaysRemain int64 `json:"estimated_days_remaining" yaml:"estimated_days_remaining"`
			}{
				BalanceRemain:       balanceRemain,
				BlocksRemain:        blocksRemain,
				EstimatedDaysRemain: estimatedDaysRemain,
			}

			outputType, err := cmd.Flags().GetString("output")
			if err != nil {
				return err
			}

			var data []byte
			if outputType == "json" {
				data, err = json.MarshalIndent(output, " ", "\t")
			} else {
				data, err = yaml.Marshal(output)
			}

			if err != nil {
				return err
			}

			return clientCtx.PrintBytes(data)

		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	cli.AddDeploymentIDFlags(cmd.Flags())
	cli.MarkReqDeploymentIDFlags(cmd)
	return cmd
}
