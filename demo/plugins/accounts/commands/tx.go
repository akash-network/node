package commands

import (
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client/commands"
	"github.com/cosmos/cosmos-sdk/client/commands/txs"
	"github.com/ovrclk/photon/demo/plugins/accounts"
)

// create account
var CreateTxCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a Photon account",
	RunE:  commands.RequireInit(createTxCmd),
}

const (
	// FlagType is the cli flag to set the type
	FlagType = "type"
)

func init() {
	CreateTxCmd.Flags().String(FlagType, "", "Account type. user or datacenter")
}

// createTxCmd creates a CreateTx, wraps, signs, and delivers it
func createTxCmd(cmd *cobra.Command, args []string) error {

	accountType, err := commands.ParseHexFlag(FlagType)
	if err != nil {
		return err
	}

	actor := txs.GetSignerAct()

	tx := accounts.NewCreateTx(accountType, actor)
	return txs.DoTx(tx)
}
