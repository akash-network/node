package main

import (
	"fmt"
	"path"

	"github.com/ovrclk/photon/app"
	"github.com/ovrclk/photon/app/market"
	"github.com/ovrclk/photon/node"
	"github.com/ovrclk/photon/state"
	"github.com/spf13/cobra"
	tmnode "github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/proxy"
	tmtypes "github.com/tendermint/tendermint/types"
	"github.com/tendermint/tmlibs/log"
)

const (
	dbName = "photon.db"
)

func startCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "start node",
		RunE:  withContext(doStartCommand),
	}
	return cmd
}

func doStartCommand(ctx Context, cmd *cobra.Command, args []string) error {

	cfg, err := ctx.TMConfig()
	if err != nil {
		return err
	}

	gprovider := tmgenesisProvider(cfg.GenesisFile())

	tmgenesis, err := gprovider()
	if err != nil {
		return err
	}

	genesis, err := node.GenesisFromTMGenesis(tmgenesis)
	if err != nil {
		return err
	}

	db, err := state.LoadDB(path.Join(ctx.RootDir(), "data", dbName))
	if err != nil {
		return err
	}

	state, err := state.LoadState(db, genesis)
	if err != nil {
		return err
	}

	logger := log.NewFilter(ctx.Log(), log.AllowError(),
		log.AllowDebugWith("module", "photon"))

	applog := logger.With("module", "photon")

	app, err := app.Create(state, applog)
	if err != nil {
		return err
	}

	pvalidator := tmtypes.LoadOrGenPrivValidatorFS(cfg.PrivValidatorFile())
	ccreator := proxy.NewLocalClientCreator(app)
	dbprovider := tmnode.DefaultDBProvider

	n, err := tmnode.NewNode(cfg, pvalidator, ccreator, gprovider, dbprovider, logger)

	if err != nil {
		return err
	}

	actor := market.NewActor(pvalidator.PrivKey)

	if err := app.ActivateMarket(actor, n.EventBus()); err != nil {
		return err
	}

	if err := n.Start(); err != nil {
		return fmt.Errorf("Failed to start node: %v", err)
	}

	applog.Info("Started node", "nodeInfo", n.Switch().NodeInfo())

	n.RunForever()

	return nil
}

func tmgenesisProvider(path string) tmnode.GenesisDocProvider {
	return func() (*tmtypes.GenesisDoc, error) {
		return node.TMGenesisFromFile(path)
	}
}
