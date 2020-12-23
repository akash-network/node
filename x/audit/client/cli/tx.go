package cli

import (
	"sort"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	akashtypes "github.com/ovrclk/akash/types"
	atypes "github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/x/audit/types"
)

// GetTxCmd returns the transaction commands for audit module
func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Audit transaction subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		cmdAttributes(),
	)

	return cmd
}

func cmdAttributes() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attr",
		Short: "Manage provider attributes",
	}

	cmd.AddCommand(
		cmdCreateProviderAttributes(),
		cmdDeleteProviderAttributes(),
	)

	return cmd
}

func cmdCreateProviderAttributes() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [provider]",
		Short: "Create/update provider attributes",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if ((len(args) - 1) % 2) != 0 {
				return errors.Errorf("attributes must be provided as pairs")
			}

			providerAddress, err := sdk.AccAddressFromBech32(args[0])
			if err != nil {
				return err
			}

			attr, err := readAttributes(args[1:])
			if err != nil {
				return err
			}

			if len(attr) == 0 {
				return errors.Errorf("no attributes provided")
			}

			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			msg := &types.MsgSignProviderAttributes{
				Auditor:    clientCtx.GetFromAddress().String(),
				Owner:      providerAddress.String(),
				Attributes: attr,
			}

			if err = msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	setCmdProviderFlags(cmd)

	return cmd
}

func cmdDeleteProviderAttributes() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [provider]",
		Short: "Delete provider attributes",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			providerAddress, err := sdk.AccAddressFromBech32(args[0])
			if err != nil {
				return err
			}

			keys, err := readKeys(args[1:])
			if err != nil {
				return err
			}

			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			msg := &types.MsgDeleteProviderAttributes{
				Auditor: clientCtx.GetFromAddress().String(),
				Owner:   providerAddress.String(),
				Keys:    keys,
			}

			if err = msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	setCmdProviderFlags(cmd)

	return cmd
}

func setCmdProviderFlags(cmd *cobra.Command) {
	flags.AddTxFlagsToCmd(cmd)

	if err := cmd.MarkFlagRequired(flags.FlagFrom); err != nil {
		panic(err.Error())
	}
}

// readAttributes try read attributes from both cobra arguments and stdin
// len of the args must be even
// read from stdin uses trick to check if it's file descriptor is a pipe
// which happens when some data is piped for example cat attr.yaml | akash ...
func readAttributes(args []string) (akashtypes.Attributes, error) {
	var attr akashtypes.Attributes

	for i := 0; i < len(args); i += 2 {
		attr = append(attr, atypes.Attribute{
			Key:   args[i],
			Value: args[i+1],
		})
	}

	sort.SliceStable(attr, func(i, j int) bool {
		return attr[i].Key < attr[j].Value
	})

	if checkAttributeDuplicates(attr) {
		return nil, errors.Errorf("supplied attributes with duplicate keys")
	}

	return attr, nil
}

func readKeys(args []string) ([]string, error) {
	sort.SliceStable(args, func(i, j int) bool {
		return args[i] < args[j]
	})

	if checkKeysDuplicates(args) {
		return nil, errors.Errorf("supplied attributes with duplicate keys")
	}

	return args, nil
}

func checkAttributeDuplicates(attr akashtypes.Attributes) bool {
	keys := make(map[string]bool)

	for _, entry := range attr {
		if _, value := keys[entry.Key]; !value {
			keys[entry.Key] = true
		} else {
			return true
		}
	}
	return false
}

func checkKeysDuplicates(k []string) bool {
	keys := make(map[string]bool)

	for _, entry := range k {
		if _, value := keys[entry]; !value {
			keys[entry] = true
		} else {
			return true
		}
	}
	return false
}
