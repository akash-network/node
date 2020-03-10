package main

import (
	"encoding/json"
	"io"
	"io/ioutil"

	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/x/auth"
	genutilcli "github.com/cosmos/cosmos-sdk/x/genutil/client/cli"
	"github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/spf13/cobra"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/cli"
	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
	dbm "github.com/tendermint/tm-db"
)

func main() {
	common.InitSDKConfig()

	cdc := app.MakeCodec()
	ctx := server.NewDefaultContext()

	root := &cobra.Command{
		Use:               "akashd",
		Long:              "Akash Daemon CLI Utility.\n\nAkash is a peer-to-peer marketplace for computing resources and \na deployment platform for heavily distributed applications. \nFind out more at https://akash.network",
		PersistentPreRunE: server.PersistentPreRunEFn(ctx),
	}

	root.AddCommand(
		genutilcli.InitCmd(ctx, cdc, app.ModuleBasics(), common.DefaultNodeHome()),

		genutilcli.CollectGenTxsCmd(ctx, cdc, auth.GenesisAccountIterator{}, common.DefaultNodeHome()),

		genutilcli.GenTxCmd(
			ctx, cdc,
			app.ModuleBasics(),
			staking.AppModuleBasic{},
			auth.GenesisAccountIterator{},
			common.DefaultNodeHome(),
			common.DefaultCLIHome(),
		),

		genutilcli.ValidateGenesisCmd(ctx, cdc, app.ModuleBasics()),
		AddGenesisAccountCmd(ctx, cdc, common.DefaultNodeHome(), common.DefaultCLIHome()),
	)

	server.AddCommands(ctx, cdc, root, newApp, exportAppStateAndTMValidators)

	executor := cli.PrepareBaseCmd(root, "AKASHD", common.DefaultNodeHome())
	err := executor.Execute()
	if err != nil {
		panic(err)
	}

}

func newApp(logger log.Logger, db dbm.DB, tio io.Writer) abci.Application {
	return app.NewApp(logger, db, tio)
}

func exportAppStateAndTMValidators(
	logger log.Logger, db dbm.DB, tio io.Writer, height int64, forZeroHeight bool, jailWhiteList []string,
) (json.RawMessage, []tmtypes.GenesisValidator, error) {

	app := app.NewApp(logger, db, ioutil.Discard)

	if height != -1 {
		err := app.LoadHeight(height)
		if err != nil {
			return nil, nil, err
		}
		return app.ExportAppStateAndValidators(forZeroHeight, jailWhiteList)
	}

	return app.ExportAppStateAndValidators(forZeroHeight, jailWhiteList)
}
