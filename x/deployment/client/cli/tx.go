package cli

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	"github.com/akash-network/akash-api/go/node/types/constants"

	"github.com/akash-network/node/client/broadcaster"
	"github.com/akash-network/node/cmd/common"
	"github.com/akash-network/node/sdl"
	cutils "github.com/akash-network/node/x/cert/utils"
)

var (
	errDeploymentUpdate              = errors.New("deployment update failed")
	errDeploymentUpdateGroupsChanged = fmt.Errorf("%w: groups are different than existing deployment, you cannot update groups", errDeploymentUpdate)
)

// GetTxCmd returns the transaction commands for this module
func GetTxCmd(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Deployment transaction subcommands",
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		cmdCreate(key),
		cmdUpdate(key),
		cmdDeposit(key),
		cmdClose(key),
		cmdGroup(key),
		cmdAuthz(),
	)
	return cmd
}

func cmdCreate(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [sdl-file]",
		Short: fmt.Sprintf("Create %s", key),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			// first lets validate certificate exists for given account
			if _, err = cutils.LoadAndQueryCertificateForAccount(cmd.Context(), clientCtx, nil); err != nil {
				if os.IsNotExist(err) {
					err = fmt.Errorf("no certificate file found for account %q.\n"+
						"consider creating it as certificate required to submit manifest", clientCtx.FromAddress.String())
				}

				return err
			}

			sdlManifest, err := sdl.ReadFile(args[0])
			if err != nil {
				return err
			}

			groups, err := sdlManifest.DeploymentGroups()
			if err != nil {
				return err
			}

			warnIfGroupVolumesExceeds(clientCtx, groups)

			id, err := DeploymentIDFromFlags(cmd.Flags(), WithOwner(clientCtx.FromAddress))
			if err != nil {
				return err
			}

			// Default DSeq to the current block height
			if id.DSeq == 0 {
				if id.DSeq, err = CurrentBlockHeight(clientCtx); err != nil {
					return err
				}
			}

			version, err := sdl.Version(sdlManifest)
			if err != nil {
				return err
			}

			deposit, err := common.DepositFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			depositorAcc, err := DepositorFromFlags(cmd.Flags(), id.Owner)
			if err != nil {
				return err
			}

			msg := &types.MsgCreateDeployment{
				ID:        id,
				Version:   version,
				Groups:    make([]types.GroupSpec, 0, len(groups)),
				Deposit:   deposit,
				Depositor: depositorAcc,
			}

			for _, group := range groups {
				msg.Groups = append(msg.Groups, *group)
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return broadcaster.BroadcastTX(cmd.Context(), clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddDeploymentIDFlags(cmd.Flags())
	common.AddDepositFlags(cmd.Flags(), DefaultDeposit)
	AddDepositorFlag(cmd.Flags())

	return cmd
}

func cmdDeposit(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deposit <amount>",
		Short: fmt.Sprintf("Deposit %s", key),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			id, err := DeploymentIDFromFlags(cmd.Flags(), WithOwner(clientCtx.FromAddress))
			if err != nil {
				return err
			}

			deposit, err := sdk.ParseCoinNormalized(args[0])
			if err != nil {
				return err
			}

			depositorAcc, err := DepositorFromFlags(cmd.Flags(), id.Owner)
			if err != nil {
				return err
			}

			msg := &types.MsgDepositDeployment{
				ID:        id,
				Amount:    deposit,
				Depositor: depositorAcc,
			}

			return broadcaster.BroadcastTX(cmd.Context(), clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddDeploymentIDFlags(cmd.Flags())
	AddDepositorFlag(cmd.Flags())

	return cmd
}

func cmdClose(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close",
		Short: fmt.Sprintf("Close %s", key),
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			id, err := DeploymentIDFromFlags(cmd.Flags(), WithOwner(clientCtx.FromAddress))
			if err != nil {
				return err
			}

			msg := &types.MsgCloseDeployment{ID: id}

			return broadcaster.BroadcastTX(cmd.Context(), clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddDeploymentIDFlags(cmd.Flags())
	return cmd
}

func cmdUpdate(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [sdl-file]",
		Short: fmt.Sprintf("update %s", key),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			id, err := DeploymentIDFromFlags(cmd.Flags(), WithOwner(clientCtx.FromAddress))
			if err != nil {
				return err
			}

			sdlManifest, err := sdl.ReadFile(args[0])
			if err != nil {
				return err
			}

			version, err := sdl.Version(sdlManifest)
			if err != nil {
				return err
			}

			groups, err := sdlManifest.DeploymentGroups()
			if err != nil {
				return err
			}

			// Query the RPC node to make sure the existing groups are identical
			queryClient := types.NewQueryClient(clientCtx)
			existingDeployment, err := queryClient.Deployment(cmd.Context(), &types.QueryDeploymentRequest{
				ID: id,
			})
			if err != nil {
				return err
			}

			// do not send the transaction if the groups have changed
			existingGroups := existingDeployment.GetGroups()
			if len(existingGroups) != len(groups) {
				return errDeploymentUpdateGroupsChanged
			}

			for i, existingGroup := range existingGroups {
				if reflect.DeepEqual(groups[i], existingGroup.GroupSpec) {
					return errDeploymentUpdateGroupsChanged
				}
			}

			warnIfGroupVolumesExceeds(clientCtx, groups)

			msg := &types.MsgUpdateDeployment{
				ID:      id,
				Version: version,
			}

			return broadcaster.BroadcastTX(cmd.Context(), clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddDeploymentIDFlags(cmd.Flags())

	return cmd
}

func cmdGroup(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Modify a Deployment's specific Group",
	}

	cmd.AddCommand(
		cmdGroupClose(key),
		cmdGroupPause(key),
		cmdGroupStart(key),
	)

	return cmd
}

func cmdGroupClose(_ string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "close",
		Short:   "close a Deployment's specific Group",
		Example: "akash tx deployment group-close --owner=[Account Address] --dseq=[uint64] --gseq=[uint32]",
		Args:    cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			id, err := GroupIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			msg := &types.MsgCloseGroup{
				ID: id,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return broadcaster.BroadcastTX(cmd.Context(), clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddGroupIDFlags(cmd.Flags())
	MarkReqGroupIDFlags(cmd)

	return cmd
}

func cmdGroupPause(_ string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pause",
		Short:   "pause a Deployment's specific Group",
		Example: "akash tx deployment group pause --owner=[Account Address] --dseq=[uint64] --gseq=[uint32]",
		Args:    cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			id, err := GroupIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			msg := &types.MsgPauseGroup{
				ID: id,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return broadcaster.BroadcastTX(cmd.Context(), clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddGroupIDFlags(cmd.Flags())
	MarkReqGroupIDFlags(cmd)

	return cmd
}

func cmdGroupStart(_ string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "start",
		Short:   "start a Deployment's specific Group",
		Example: "akash tx deployment group pause --owner=[Account Address] --dseq=[uint64] --gseq=[uint32]",
		Args:    cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			id, err := GroupIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			msg := &types.MsgStartGroup{
				ID: id,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return broadcaster.BroadcastTX(cmd.Context(), clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddGroupIDFlags(cmd.Flags())
	MarkReqGroupIDFlags(cmd)

	return cmd
}

func cmdAuthz() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "authz",
		Short: "Deployment authorization transaction subcommands",
		Long:  "Authorize and revoke access to pay for deployments on behalf of your address",
	}

	cmd.AddCommand(
		cmdGrantAuthorization(),
		cmdRevokeAuthorization(),
	)

	return cmd
}

func cmdGrantAuthorization() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grant <grantee> <spend_limit> --from <granter>",
		Short: "Grant deposit deployment authorization to an address",
		Long: strings.TrimSpace(
			fmt.Sprintf(`grant authorization to an address to pay for a deployment on your behalf:

Examples:
 $ akash tx %s authz grant akash1skjw.. 50akt --from=akash1skl..
 $ akash tx %s authz grant akash1skjw.. 50akt --from=akash1skl.. --expiration=1661020200
	`, types.ModuleName, types.ModuleName),
		),
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

			spendLimit, err := sdk.ParseCoinNormalized(args[1])
			if err != nil {
				return err
			}
			if spendLimit.IsZero() || spendLimit.IsNegative() {
				return fmt.Errorf("spend-limit should be greater than zero, got: %s", spendLimit)
			}

			exp, err := cmd.Flags().GetInt64(FlagExpiration)
			if err != nil {
				return err
			}

			granter := clientCtx.GetFromAddress()
			authorization := types.NewDepositDeploymentAuthorization(spendLimit)

			msg, err := authz.NewMsgGrant(granter, grantee, authorization, time.Unix(exp, 0))
			if err != nil {
				return err
			}

			return broadcaster.BroadcastTX(cmd.Context(), clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	cmd.Flags().Int64(FlagExpiration, time.Now().AddDate(1, 0, 0).Unix(), "The Unix timestamp. Default is one year.")
	_ = cmd.MarkFlagRequired(flags.FlagFrom)

	return cmd
}

func cmdRevokeAuthorization() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke [grantee] --from=[granter]",
		Short: "Revoke deposit deployment authorization given to an address",
		Long: strings.TrimSpace(
			fmt.Sprintf(`revoke deposit deployment authorization from a granter to a grantee:
Example:
 $ akash tx %s authz revoke akash1skj.. --from=akash1skj..
			`, types.ModuleName),
		),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			grantee, err := sdk.AccAddressFromBech32(args[0])
			if err != nil {
				return err
			}

			granter := clientCtx.GetFromAddress()
			msgTypeURL := types.DepositDeploymentAuthorization{}.MsgTypeURL()
			msg := authz.NewMsgRevoke(granter, grantee, msgTypeURL)

			return broadcaster.BroadcastTX(cmd.Context(), clientCtx, cmd.Flags(), &msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	_ = cmd.MarkFlagRequired(flags.FlagFrom)

	return cmd
}

func warnIfGroupVolumesExceeds(cctx client.Context, dgroups []*types.GroupSpec) {
	for _, group := range dgroups {
		for _, resources := range group.GetResources() {
			if len(resources.Resources.Storage) > constants.DefaultMaxGroupVolumes {
				_ = cctx.PrintString(fmt.Sprintf("amount of volumes for service exceeds recommended value (%v > %v)\n"+
					"there may no providers on network to bid", len(resources.Resources.Storage), constants.DefaultMaxGroupVolumes))
			}
		}
	}
}
