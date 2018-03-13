package main

import (
	"errors"
	"os"
	"sync"

	"github.com/ovrclk/akash/cmd/common"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	tmconfig "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tmlibs/log"
)

const (
	flagRootDir = "data"
)

type cmdRunner func(cmd *cobra.Command, args []string) error
type ctxRunner func(ctx Context, cmd *cobra.Command, args []string) error

func withContext(fn ctxRunner) cmdRunner {
	return func(cmd *cobra.Command, args []string) error {
		ctx := newContext(cmd)
		return fn(ctx, cmd, args)
	}
}

func requireRootDir(fn ctxRunner) ctxRunner {
	return func(ctx Context, cmd *cobra.Command, args []string) error {
		if root := ctx.RootDir(); root == "" {
			return errors.New("root directory unset")
		}
		return fn(ctx, cmd, args)
	}
}

type Context interface {
	RootDir() string
	TMConfig() (*tmconfig.Config, error)
	Log() log.Logger
}

type context struct {
	cmd   *cobra.Command
	tmcfg *tmconfig.Config
	log   log.Logger
	mtx   sync.Mutex
}

func newContext(cmd *cobra.Command) Context {
	return &context{cmd: cmd, mtx: sync.Mutex{}}
}

func (ctx *context) RootDir() string {
	root, _ := ctx.cmd.Flags().GetString(flagRootDir)
	return root
}

func (ctx *context) Log() log.Logger {
	ctx.mtx.Lock()
	defer ctx.mtx.Unlock()

	if ctx.log != nil {
		return ctx.log
	}

	ctx.log = common.NewLogger(os.Stdout)
	return ctx.log
}

func (ctx *context) TMConfig() (*tmconfig.Config, error) {
	ctx.mtx.Lock()
	defer ctx.mtx.Unlock()

	if ctx.tmcfg != nil {
		return ctx.tmcfg, nil
	}

	root := ctx.RootDir()

	if root == "" {
		return nil, errors.New("root dir required")
	}

	cfg := tmconfig.DefaultConfig()

	if err := viper.Unmarshal(cfg); err != nil {
		return nil, err
	}

	cfg.SetRoot(root)

	if val := viper.GetString("genesis"); val != "" {
		cfg.Genesis = val
	}

	if val := viper.GetString("validator"); val != "" {
		cfg.PrivValidator = val
	}

	if val := viper.GetString("moniker"); val != "" {
		cfg.Moniker = val
	}

	tmconfig.EnsureRoot(root)
	return cfg, nil
}
