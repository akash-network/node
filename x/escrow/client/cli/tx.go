package cli

import (
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/cosmos/cosmos-sdk/x/authz"

	"github.com/ovrclk/akash/x/escrow/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

const (
	FlagExpiration = "expiration"
)

var (
	ErrNegativeCredits = errors.New("credits should be greater than zero")
)

func GetTxCmd() *cobra.Command {
	return nil
}

func GrantAuthorizationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy-grant <grantee> <credits> --from <granter>",
		Short: "Grant deploy authorization to an address",
		Example: fmt.Sprintf(`$ %s tx %s deploy-grant [grantee address] 1000uakt --from=[granter address]`,
			version.AppName, authz.ModuleName),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			grantee, err := sdk.AccAddressFromBech32(args[0])
			if err != nil {
				return err
			}

			exp, err := cmd.Flags().GetInt64(FlagExpiration)
			if err != nil {
				return err
			}

			credits, err := sdk.ParseCoinNormalized(args[1])
			if err != nil {
				return err
			}

			if !credits.IsPositive() {
				return ErrNegativeCredits
			}

			authorization := types.NewEscrowAuthorization(credits)

			msg, err := authz.NewMsgGrant(clientCtx.GetFromAddress(), grantee, authorization, time.Unix(exp, 0))
			if err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	cmd.Flags().Int64(FlagExpiration, time.Now().AddDate(1, 0, 0).Unix(), "The Unix timestamp. Default is one year.")
	return cmd
}
