package main

import (
	"context"
	"fmt"
	"path"

	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/app/market"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/node"
	"github.com/ovrclk/akash/state"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/libs/log"
	tmnode "github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"
	tmtypes "github.com/tendermint/tendermint/types"
)

const (
	dbName = "akash.db"
)

func startCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "start node",
		RunE:  withSession(doStartCommand),
	}
	return cmd
}

func doStartCommand(session Session, cmd *cobra.Command, args []string) error {

	cfg, err := session.TMConfig()
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

	db, err := state.LoadDB(path.Join(session.RootDir(), "data", dbName))
	if err != nil {
		return err
	}

	commitState, cacheState, err := state.LoadState(db, genesis)
	if err != nil {
		return err
	}

	logger := log.NewFilter(session.Log(), log.AllowInfo(),
		log.AllowDebugWith("module", "akash"))
	// logger := log.NewFilter(session.Log(), log.AllowDebug())

	applog := logger.With("module", "akash")

	app, err := app.Create(commitState, cacheState, applog)
	if err != nil {
		return err
	}

	nkey, err := p2p.LoadNodeKey(cfg.NodeKeyFile())
	if err != nil {
		return nil
	}

	pvalidator := privval.LoadOrGenFilePV(cfg.PrivValidatorFile())
	ccreator := proxy.NewLocalClientCreator(app)
	dbprovider := tmnode.DefaultDBProvider
	mprovider := tmnode.DefaultMetricsProvider(cfg.Instrumentation)

	n, err := tmnode.NewNode(cfg,
		pvalidator,
		nkey,
		ccreator,
		gprovider,
		dbprovider,
		mprovider,
		logger)

	if err != nil {
		return err
	}

	actor := market.NewActor(pvalidator.PrivKey)

	fmt.Println("activating market...")

	return common.RunForeverWithContext(session.Context(), func(ctx context.Context) error {

		if err := app.ActivateMarket(actor); err != nil {
			return err
		}

		if err := n.Start(); err != nil {
			return fmt.Errorf("Failed to start node: %v", err)
		}

		applog.Info("Started node", "nodeInfo", n.Switch().NodeInfo())

		<-ctx.Done()
		return n.Stop()
	})
}

func tmgenesisProvider(path string) tmnode.GenesisDocProvider {
	return func() (*tmtypes.GenesisDoc, error) {
		return node.TMGenesisFromFile(path)
	}
}
