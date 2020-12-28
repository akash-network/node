package cli

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/ovrclk/akash/x/cert/types"
)

func GetQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Certificate query commands",
		DisableFlagParsing:         true,
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
		Use:   "list",
		Short: "Query for all certificates",
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
				Filter: types.CertificateFilter{
					Owner:  "",
					Serial: cmd.Flag("serial").Value.String(),
					State:  "",
				},
				Pagination: pageReq,
			}

			if value := cmd.Flag("owner").Value.String(); value != "" {
				var owner sdk.Address
				if owner, err = sdk.AccAddressFromBech32(value); err != nil {
					return err
				}

				params.Filter.Owner = owner.String()
			}

			if value := cmd.Flag("state").Value.String(); value != "" {
				if value != "valid" && value != "revoked" {
					return errors.Errorf("invalid value in --state flag. expected valid|revoked")
				}

				params.Filter.State = value
			}

			res, err := queryClient.Certificates(context.Background(), params)
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
