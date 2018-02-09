package commands

import (
	"encoding/hex"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cmn "github.com/tendermint/tmlibs/common"

	"github.com/cosmos/cosmos-sdk/client/commands"
	"github.com/cosmos/cosmos-sdk/client/commands/query"
	"github.com/cosmos/cosmos-sdk/stack"

	"github.com/ovrclk/photon/plugins/accounts"
)

const flagHeight = "height"

// command to query raw data
var AccountsQueryCmd = &cobra.Command{
	Use:   "accounts [address]",
	Short: "Get data stored under key in accounts",
	RunE:  commands.RequireInit(accountsQueryCmd),
}

func init() {
	AccountsQueryCmd.Flags().String(flagHeight, "", "Block height of data to query (Default is 0 for latest)")
}

func accountsQueryCmd(cmd *cobra.Command, args []string) error {
	var res accounts.Data

	// get value of key to query
	arg, err := commands.GetOneArg(args, "address")
	if err != nil {
		return err
	}
	// key is hex so must strip and decode to get string value
	key, err := hex.DecodeString(cmn.StripHex(arg))
	if err != nil {
		return err
	}

	// get value of block height to do the query at
	height := viper.GetInt64(flagHeight)

	// append the module name (accounts.Name) to the key that is being looked up
	key = stack.PrefixedKey(accounts.Name, key)
	prove := !viper.GetBool(commands.FlagTrustNode)
	height, err = query.GetParsed(key, &res, height, prove)
	if err != nil {
		return err
	}

	return query.OutputProof(res, height)
}
