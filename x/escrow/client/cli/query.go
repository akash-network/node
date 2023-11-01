package cli

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	deploymentTypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	types "github.com/akash-network/akash-api/go/node/escrow/v1beta3"

	marketTypes "github.com/akash-network/akash-api/go/node/market/v1beta4"

	netutil "github.com/akash-network/node/util/network"
	"github.com/akash-network/node/x/deployment/client/cli"
	"github.com/akash-network/node/x/escrow/client/util"
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

			marketClient := marketTypes.NewQueryClient(clientCtx)
			ctx := context.Background()

			id, err := cli.DeploymentIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

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

			if len(leasesResponse.Leases) == 0 {
				return errNoLeaseMatches
			}

			// Fetch the balance of the escrow account
			deploymentClient := deploymentTypes.NewQueryClient(clientCtx)
			totalLeaseAmount := leasesResponse.TotalPriceAmount()
			blockchainHeight, err := cli.CurrentBlockHeight(clientCtx)
			if err != nil {
				return err
			}

			res, err := deploymentClient.Deployment(ctx, &deploymentTypes.QueryDeploymentRequest{
				ID: deploymentTypes.DeploymentID{Owner: id.Owner, DSeq: id.DSeq},
			})
			if err != nil {
				return err
			}

			balanceRemain := util.LeaseCalcBalanceRemain(res.EscrowAccount.TotalBalance().Amount,
				int64(blockchainHeight),
				res.EscrowAccount.SettledAt,
				totalLeaseAmount)

			blocksRemain := util.LeaseCalcBlocksRemain(balanceRemain, totalLeaseAmount)

			output := struct {
				BalanceRemain       float64       `json:"balance_remaining" yaml:"balance_remaining"`
				BlocksRemain        int64         `json:"blocks_remaining" yaml:"blocks_remaining"`
				EstimatedTimeRemain time.Duration `json:"estimated_time_remaining" yaml:"estimated_time_remaining"`
			}{
				BalanceRemain:       balanceRemain,
				BlocksRemain:        blocksRemain,
				EstimatedTimeRemain: netutil.AverageBlockTime * time.Duration(blocksRemain),
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
