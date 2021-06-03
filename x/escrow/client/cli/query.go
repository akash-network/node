package cli

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

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

// Calculate blocks per day by using 6.5 seconds as average block-time
const (
	secondsPerDay   = 24 * 60 * 60
	secondsPerBlock = 6.5
	blocksPerDay    = secondsPerDay / secondsPerBlock
)

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

			// owner, err := cmd.Flags().GetString("owner")
			// if err != nil {
			// 	return err
			// }

			// dseq, err := cmd.Flags().GetUint64("dseq")
			// if err != nil {
			// 	return err
			// }

			marketClient := marketTypes.NewQueryClient(clientCtx)
			ctx := context.Background()

			id, err := cli.DeploymentIDFromFlags(cmd.Flags(), "")
			if err != nil {
				return err
			}

			// res, err := marketClient.Deployment(ctx, &marketTypes.QueryLeasesRequest{  })
			// if err != nil {
			// 	return err
			// }

			// Fetch leases matching owner & dseq
			leaseRequest := marketTypes.QueryLeasesRequest{
				Filters: marketTypes.LeaseFilters{
					Owner:    id.Owner,
					DSeq:     id.DSeq,
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

				amount := lease.Price.Amount
				totalLeaseAmount = totalLeaseAmount.Add(amount)

			}
			res, err := deploymentClient.Deployment(ctx, &deploymentTypes.QueryDeploymentRequest{
				ID: deploymentTypes.DeploymentID{Owner: id.Owner, DSeq: id.DSeq},
			})
			if err != nil {
				return err
			}
			balance := res.EscrowAccount.Balance.Amount
			settledAt := res.EscrowAccount.SettledAt
			balanceRemain := float64(balance.Int64() - ((int64(blockchainHeight) - settledAt) * (totalLeaseAmount.Int64())))
			blocksRemain := (balanceRemain / float64(totalLeaseAmount.Int64()))
			output := struct {
				BalanceRemain       float64 `json:"balance_remaining" yaml:"balance_remaining"`
				BlocksRemain        float64 `json:"blocks_remaining" yaml:"blocks_remaining"`
				EstimatedTimeRemain string  `json:"estimated_time_remaining" yaml:"estimated_time_remaining"`
			}{
				BalanceRemain:       balanceRemain,
				BlocksRemain:        blocksRemain,
				EstimatedTimeRemain: (time.Duration(math.Floor(secondsPerBlock*blocksRemain)) * time.Second).String(),
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
