package main

import (
	"encoding/json"
	"io"
	"io/ioutil"

	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/x/bank"
	genutilcli "github.com/cosmos/cosmos-sdk/x/genutil/client/cli"
	"github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/cli"
	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
	dbm "github.com/tendermint/tm-db"
)

const flagInvCheckPeriod = "inv-check-period"

var invCheckPeriod uint

func main() {
	common.InitSDKConfig()

	appCodec, cdc := app.MakeCodecs()
	ctx := server.NewDefaultContext()

	root := &cobra.Command{
		Use:               "akashd",
		Long:              "Akash Daemon CLI Utility.\n\nAkash is a peer-to-peer marketplace for computing resources and \na deployment platform for heavily distributed applications. \nFind out more at https://akash.network",
		PersistentPreRunE: server.PersistentPreRunEFn(ctx),
	}

	root.AddCommand(
		genutilcli.InitCmd(ctx, cdc, app.ModuleBasics(), common.DefaultNodeHome()),

		genutilcli.CollectGenTxsCmd(ctx, cdc, bank.GenesisBalancesIterator{}, common.DefaultNodeHome()),

		genutilcli.GenTxCmd(
			ctx, cdc,
			app.ModuleBasics(),
			staking.AppModuleBasic{},
			bank.GenesisBalancesIterator{},
			common.DefaultNodeHome(),
			common.DefaultCLIHome(),
		),

		genutilcli.ValidateGenesisCmd(ctx, cdc, app.ModuleBasics()),
		AddGenesisAccountCmd(ctx, cdc, appCodec, common.DefaultNodeHome(), common.DefaultCLIHome()),
	)

	server.AddCommands(ctx, cdc, root, newApp, exportAppStateAndTMValidators)

	executor := cli.PrepareBaseCmd(root, "AKASHD", common.DefaultNodeHome())
	root.PersistentFlags().UintVar(&invCheckPeriod, flagInvCheckPeriod,
		0, "Assert registered invariants every N blocks")
	err := executor.Execute()
	if err != nil {
		panic(err)
	}

}

func newApp(logger log.Logger, db dbm.DB, tio io.Writer) abci.Application {
	skipUpgradeHeights := make(map[int64]bool)
	for _, h := range viper.GetIntSlice(server.FlagUnsafeSkipUpgrades) {
		skipUpgradeHeights[int64(h)] = true
	}

	return app.NewApp(logger, db, tio, true, invCheckPeriod, skipUpgradeHeights, viper.GetString(flags.FlagHome))
}

func exportAppStateAndTMValidators(
	logger log.Logger, db dbm.DB, tio io.Writer, height int64, forZeroHeight bool, jailWhiteList []string,
) (json.RawMessage, []tmtypes.GenesisValidator, *abci.ConsensusParams, error) {

	if height != -1 {
		app := app.NewApp(logger, db, ioutil.Discard, false, uint(1), map[int64]bool{}, "")
		err := app.LoadHeight(height)
		if err != nil {
			return nil, nil, nil, err
		}

		return app.ExportAppStateAndValidators(forZeroHeight, jailWhiteList)
	}

	app := app.NewApp(logger, db, ioutil.Discard, true, uint(1), map[int64]bool{}, "")
	return app.ExportAppStateAndValidators(forZeroHeight, jailWhiteList)
}
