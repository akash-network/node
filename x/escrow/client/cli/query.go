package cli

import (
	"context"
	"encoding/json"

	"gopkg.in/yaml.v3"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/ovrclk/akash/x/deployment/client/cli"
	deploymentTypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/escrow/types"
	marketTypes "github.com/ovrclk/akash/x/market/types"
	"github.com/spf13/cobra"
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
			dseq, err := cmd.Flags().GetUint64("dseq")

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
			// fmt.Printf("length of leases, %d \n", len(leases))
			outputSlice := make([]interface{}, 0)
			for _, lease := range leases {
				//  Fetch the time of last settlement
				res, err := deploymentClient.Deployment(ctx, &deploymentTypes.QueryDeploymentRequest{
					ID: lease.LeaseID.DeploymentID(),
				})
				if err != nil {
					return err
				}
				amount := lease.Price.Amount
				balance := res.EscrowAccount.Balance.Amount
				settledAt := res.EscrowAccount.SettledAt
				blockchainHeight, err := cli.CurrentBlockHeight(clientCtx)

				balanceRemain := balance.Int64() - ((int64(blockchainHeight) - settledAt) * (amount.Int64()))
				blocksRemain := balanceRemain / amount.Int64()
				blocksPerDay := 86400 / 6.5
				daysRemain := blocksRemain / int64(blocksPerDay)

				output := struct {
					BalanceRemain int64 `json:"balance_remaining" yaml:"balance_remaining"`
					BlocksRemain  int64 `json:"blocks_remaining" yaml:"blocks_remaining"`
					DaysRemain    int64 `json:"days_remaining" yaml:"days_remaining"`
				}{
					BalanceRemain: balanceRemain,
					BlocksRemain:  blocksRemain,
					DaysRemain:    daysRemain,
				}
				outputSlice = append(outputSlice, output)

			}
			outputType, err := cmd.Flags().GetString("output")
			if outputType == "json" {
				data, err := json.MarshalIndent(outputSlice, " ", "\t")
				if err != nil {
					return err
				}
				clientCtx.PrintBytes(data)
			} else {
				data, err := yaml.Marshal(outputSlice)
				if err != nil {
					return err
				}
				clientCtx.PrintBytes(data)
			}

			if err != nil {
				return err
			}
			return nil
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	cli.AddDeploymentIDFlags(cmd.Flags())
	cli.MarkReqDeploymentIDFlags(cmd)
	return cmd
}
