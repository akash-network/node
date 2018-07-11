package main

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/ovrclk/akash/cmd/common"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	tmconfig "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/log"
)

const (
	flagRootDir = "data"
)

type cmdRunner func(cmd *cobra.Command, args []string) error
type sessionRunner func(session Session, cmd *cobra.Command, args []string) error

func withSession(fn sessionRunner) cmdRunner {
	return func(cmd *cobra.Command, args []string) error {
		session := newSession(cmd)
		return fn(session, cmd, args)
	}
}

func requireRootDir(fn sessionRunner) sessionRunner {
	return func(session Session, cmd *cobra.Command, args []string) error {
		if root := session.RootDir(); root == "" {
			return errors.New("root directory unset")
		}
		return fn(session, cmd, args)
	}
}

type Session interface {
	RootDir() string
	TMConfig() (*tmconfig.Config, error)
	Log() log.Logger
	Context() context.Context
	Cancel()
}

type session struct {
	cmd    *cobra.Command
	tmcfg  *tmconfig.Config
	log    log.Logger
	mtx    sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

func newSession(cmd *cobra.Command) Session {
	ctx, cancel := context.WithCancel(context.Background())
	return &session{
		cmd:    cmd,
		mtx:    sync.Mutex{},
		cancel: cancel,
		ctx:    ctx,
	}
}

func (session *session) Context() context.Context {
	return session.ctx
}

func (session *session) Cancel() {
	session.cancel()
}

func (session *session) RootDir() string {
	root, _ := session.cmd.Flags().GetString(flagRootDir)
	return root
}

func (session *session) Log() log.Logger {
	session.mtx.Lock()
	defer session.mtx.Unlock()

	if session.log != nil {
		return session.log
	}

	session.log = common.NewLogger(os.Stdout)
	return session.log
}

func (session *session) TMConfig() (*tmconfig.Config, error) {
	session.mtx.Lock()
	defer session.mtx.Unlock()

	if session.tmcfg != nil {
		return session.tmcfg, nil
	}

	root := session.RootDir()

	if root == "" {
		return nil, errors.New("root dir required")
	}

	cfg := tmconfig.DefaultConfig()
	// cfg.P2P.AuthEnc = false

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
