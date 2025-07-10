package cli

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/spf13/cobra"

	"gopkg.in/yaml.v3"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	"pkg.akt.dev/go/cli"
	cflags "pkg.akt.dev/go/cli/flags"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dv1beta4 "pkg.akt.dev/go/node/deployment/v1beta4"
	etypes "pkg.akt.dev/go/node/escrow/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mv1beta5 "pkg.akt.dev/go/node/market/v1beta5"

	netutil "pkg.akt.dev/node/util/network"
	"pkg.akt.dev/node/x/escrow/client/util"
)

func GetQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        etypes.ModuleName,
		Short:                      "Escrow query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			cctx, err := sdkclient.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			qq, err := cli.DiscoverQueryClient(ctx, cctx)
			if err != nil {
				return err
			}

			id, err := cflags.DeploymentIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			// Fetch leases matching owner & dseq
			leaseRequest := mv1beta5.QueryLeasesRequest{
				Filters: mv1.LeaseFilters{
					Owner:    id.Owner,
					DSeq:     id.DSeq,
					GSeq:     0,
					OSeq:     0,
					Provider: "",
					State:    mv1.LeaseActive.String(),
				},
				Pagination: nil,
			}

			leasesResponse, err := qq.Query().Market().Leases(ctx, &leaseRequest)
			if err != nil {
				return err
			}

			if len(leasesResponse.Leases) == 0 {
				return errNoLeaseMatches
			}

			// Fetch the balance of the escrow account
			totalLeaseAmount := leasesResponse.TotalPriceAmount()
			blockchainHeight, err := qq.Node().CurrentBlockHeight(ctx)
			if err != nil {
				return err
			}

			res, err := qq.Query().Deployment().Deployment(cmd.Context(), &dv1beta4.QueryDeploymentRequest{
				ID: dv1.DeploymentID{Owner: id.Owner, DSeq: id.DSeq},
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

			return qq.ClientContext().PrintBytes(data)

		},
	}

	cflags.AddQueryFlagsToCmd(cmd)
	cflags.AddDeploymentIDFlags(cmd.Flags())
	cflags.MarkReqDeploymentIDFlags(cmd)

	return cmd
}
