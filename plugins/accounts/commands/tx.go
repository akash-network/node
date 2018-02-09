package commands

import (
	"github.com/cosmos/cosmos-sdk/client/commands"
	"github.com/cosmos/cosmos-sdk/client/commands/txs"
	"github.com/ovrclk/photon/plugins/accounts"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// create account
var CreateTxCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a Photon account",
	RunE:  commands.RequireInit(createTxCmd),
}

var UpdateTxCmd = &cobra.Command{
	Use:   "update",
	Short: "Update a Photon datacenter account",
	RunE:  commands.RequireInit(updateTxCmd),
}

const (
	// FlagType is the cli flag to set the type
	FlagType      = "type"
	FlagResources = "resources"
)

func init() {
	CreateTxCmd.Flags().String(FlagType, "", "Account type. user or datacenter")
	UpdateTxCmd.Flags().String(FlagResources, "", "Datacenter resources")
}

// ParseFlag takes a flag name and gets the viper contents
// todo: cosmos-sdk should support this but does not
func parseFlag(flag string) ([]byte, error) {
	arg := viper.GetString(flag)
	if arg == "" {
		return nil, errors.Errorf("No such flag: %s", flag)
	}
	return []byte(arg), nil
}

// createTxCmd creates a CreateTx, wraps, signs, and delivers it
func createTxCmd(cmd *cobra.Command, args []string) error {

	accountType, err := parseFlag(FlagType)
	if err != nil {
		return err
	}

	actor := txs.GetSignerAct()

	tx := accounts.NewCreateTx(accountType, actor)
	return txs.DoTx(tx)
}

// updateTxCmd creates a CreateTx, wraps, signs, and delivers it
func updateTxCmd(cmd *cobra.Command, args []string) error {
	resources, err := parseFlag(FlagResources)
	if err != nil {
		return err
	}
	actor := txs.GetSignerAct()
	tx := accounts.NewUpdateTx(resources, actor)
	return txs.DoTx(tx)
}
