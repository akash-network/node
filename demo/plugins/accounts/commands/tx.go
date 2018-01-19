package commands

import (
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client/commands"
	"github.com/cosmos/cosmos-sdk/client/commands/txs"
	"github.com/ovrclk/photon/demo/plugins/accounts"
)

// SetTxCmd is CLI command to set data
var SetTxCmd = &cobra.Command{
	Use:   "set",
	Short: "Sets a key value pair",
	RunE:  commands.RequireInit(setTxCmd),
}

// RemoveTxCmd is CLI command to remove data
var RemoveTxCmd = &cobra.Command{
	Use:   "remove",
	Short: "Removes a key value pair",
	RunE:  commands.RequireInit(removeTxCmd),
}

// create account
// var CreateTxCmd = &cobra.Command{
// 	Use:   "create",
// 	Short: "Create a Photon account",
// 	RunE:  commands.RequireInit(createTxCmd),
// }

const (
	// FlagKey is the cli flag to set the key
	FlagKey = "key"
	// FlagValue is the cli flag to set the value
	FlagValue = "value"
	// FlagType is the cli flag to set the type
	// FlagType = "type"
)

func init() {
	SetTxCmd.Flags().String(FlagKey, "", "Key to store data under (hex)")
	SetTxCmd.Flags().String(FlagValue, "", "Data to store (hex)")
	RemoveTxCmd.Flags().String(FlagKey, "", "Key under which to remove data (hex)")
}

// setTxCmd creates a SetTx, wraps, signs, and delivers it
func setTxCmd(cmd *cobra.Command, args []string) error {
	key, err := commands.ParseHexFlag(FlagKey)
	if err != nil {
		return err
	}
	value, err := commands.ParseHexFlag(FlagValue)
	if err != nil {
		return err
	}

	tx := accounts.NewSetTx(key, value)
	return txs.DoTx(tx)
}

// removeTxCmd creates a RemoveTx, wraps, signs, and delivers it
func removeTxCmd(cmd *cobra.Command, args []string) error {
	key, err := commands.ParseHexFlag(FlagKey)
	if err != nil {
		return err
	}

	tx := accounts.NewRemoveTx(key)
	return txs.DoTx(tx)
}

// // createTxCmd creates a CreateTx, wraps, signs, and delivers it
// func createTxCmd(cmd *cobra.Command, args []string) error {
// 	accountType, err := commands.ParseHexFlag(FlagType)
// 	if err != nil {
// 		return err
// 	}
// 	tx := accounts.NewCreateTx(accountType)
// 	return txs.DoTx(tx)
// }
