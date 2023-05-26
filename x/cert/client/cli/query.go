package cli

import (
	"fmt"
	"math/big"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	types "github.com/akash-network/akash-api/go/node/cert/v1beta3"
)

const (
	stateValid   = "valid"
	stateRevoked = "revoked"
)

func GetQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Certificate query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		cmdGetCertificates(),
	)

	return cmd
}

func cmdGetCertificates() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "list",
		Short:        "Query for all certificates",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cctx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			queryClient := types.NewQueryClient(cctx)

			pageReq, err := client.ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			params := &types.QueryCertificatesRequest{
				Pagination: pageReq,
			}

			if value := cmd.Flag("owner").Value.String(); value != "" {
				var owner sdk.Address
				if owner, err = sdk.AccAddressFromBech32(value); err != nil {
					return err
				}

				params.Filter.Owner = owner.String()
			}

			if value := cmd.Flag("serial").Value.String(); value != "" {
				if params.Filter.Owner == "" {
					return fmt.Errorf("--serial flag requires --owner to be set")
				}
				val, valid := new(big.Int).SetString(value, 10)
				if !valid {
					return errInvalidSerialFlag
				}

				params.Filter.Serial = val.String()
			}

			if value := cmd.Flag("state").Value.String(); value != "" {
				if value != stateValid && value != stateRevoked {
					return fmt.Errorf("invalid value of --state flag. expected valid|revoked")
				}

				params.Filter.State = value
			}

			res, err := queryClient.Certificates(cmd.Context(), params)
			if err != nil {
				return err
			}

			return cctx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	flags.AddPaginationFlagsToCmd(cmd, "certificates")

	cmd.Flags().String("serial", "", "filter certificates by serial number")
	cmd.Flags().String("owner", "", "filter certificates by owner")
	cmd.Flags().String("state", "", "filter certificates by valid|revoked")

	return cmd
}
