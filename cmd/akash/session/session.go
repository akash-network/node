package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/query"
	"github.com/ovrclk/akash/txutil"

	"github.com/cosmos/cosmos-sdk/crypto/keys"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	tmdb "github.com/tendermint/tendermint/libs/db"
	"github.com/tendermint/tendermint/libs/log"
	tmclient "github.com/tendermint/tendermint/rpc/client"
)

type Session interface {
	RootDir() string
	KeyManager() (keys.Keybase, error)
	Node() string
	Client() *tmclient.HTTP
	TxClient() (txutil.Client, error)
	QueryClient() query.Client
	KeyName() string
	KeyType() (keys.SigningAlgo, error)
	Key() (keys.Info, error)
	Nonce() (uint64, error)
	Log() log.Logger
	Signer() (txutil.Signer, keys.Info, error)
	Ctx() context.Context
	NoWait() bool
	Host() string
	Password() (string, error)
}

type cmdRunner func(cmd *cobra.Command, args []string) error
type Runner func(sess Session, cmd *cobra.Command, args []string) error

func WithSession(fn Runner) cmdRunner {
	return func(cmd *cobra.Command, args []string) error {
		session := newSession(cmd)
		defer session.shutdown()
		return fn(session, cmd, args)
	}
}

func RequireHost(fn Runner) Runner {
	return func(session Session, cmd *cobra.Command, args []string) error {
		if host := session.Host(); host == "" {
			return errors.New("host unset")
		}
		return fn(session, cmd, args)
	}
}

func RequireRootDir(fn Runner) Runner {
	return func(session Session, cmd *cobra.Command, args []string) error {
		if root := session.RootDir(); root == "" {
			return errors.New("root directory unset")
		}
		return fn(session, cmd, args)
	}
}

func RequireKeyManager(fn Runner) Runner {
	return RequireRootDir(func(session Session, cmd *cobra.Command, args []string) error {
		if _, err := session.KeyManager(); err != nil {
			return err
		}
		return fn(session, cmd, args)
	})
}

func RequireNode(fn Runner) Runner {
	return func(session Session, cmd *cobra.Command, args []string) error {
		if node := session.Node(); node == "" {
			return fmt.Errorf("node required")
		}
		return fn(session, cmd, args)
	}
}

func RequireKey(fn Runner) Runner {
	return func(session Session, cmd *cobra.Command, args []string) error {
		if _, err := session.Key(); err != nil {
			return err
		}
		return fn(session, cmd, args)
	}
}

func newSession(cmd *cobra.Command) *session {
	return &session{cmd: cmd, mtx: sync.Mutex{}}
}

type session struct {
	cmd  *cobra.Command
	kmgr keys.Keybase
	kdb  tmdb.DB
	log  log.Logger
	mtx  sync.Mutex
}

func (ctx *session) shutdown() {
	ctx.mtx.Lock()
	defer ctx.mtx.Unlock()
	if ctx.kdb != nil {
		ctx.kdb.Close()
	}
}

func (ctx *session) Log() log.Logger {
	ctx.mtx.Lock()
	defer ctx.mtx.Unlock()

	if ctx.log != nil {
		return ctx.log
	}

	ctx.log = common.NewLogger(os.Stdout).With("app", "akash")
	ctx.log = log.NewFilter(ctx.log, log.AllowAll())
	return ctx.log
}

func (ctx *session) RootDir() string {
	root, _ := ctx.cmd.Flags().GetString(flagRootDir)
	return root
}

func (ctx *session) KeyManager() (keys.Keybase, error) {
	ctx.mtx.Lock()
	defer ctx.mtx.Unlock()

	if ctx.kmgr != nil {
		return ctx.kmgr, nil
	}

	root := ctx.RootDir()
	if root == "" {
		return nil, errors.New("root directory unset")
	}

	var err error
	ctx.kmgr, ctx.kdb, err = loadKeyManager(root)

	return ctx.kmgr, err
}

func (ctx *session) Node() string {
	if ctx.cmd.Flag(flagNode).Value.String() != ctx.cmd.Flag(flagNode).DefValue {
		return ctx.cmd.Flag(flagNode).Value.String()
	}
	return viper.GetString(flagNode)
}

func (ctx *session) Client() *tmclient.HTTP {
	return tmclient.NewHTTP(ctx.Node(), "/websocket")
}

func (ctx *session) TxClient() (txutil.Client, error) {
	signer, key, err := ctx.Signer()
	if err != nil {
		return nil, err
	}
	nonce, err := ctx.cmd.Flags().GetUint64(flagNonce)
	if err != nil {
		nonce = 0
	}
	return txutil.NewClient(ctx.Client(), signer, key, nonce), nil
}

func (ctx *session) QueryClient() query.Client {
	return query.NewClient(ctx.Client())
}

func (ctx *session) KeyName() string {
	val, _ := ctx.cmd.Flags().GetString(flagKey)
	return val
}

func (ctx *session) KeyType() (keys.SigningAlgo, error) {
	return parseFlagKeyType(ctx.cmd.Flags())
}

func (ctx *session) Key() (keys.Info, error) {
	kmgr, err := ctx.KeyManager()
	if err != nil {
		return nil, err
	}

	kname := ctx.KeyName()
	if kname == "" {
		return nil, errors.New("no key specified")
	}

	info, err := kmgr.Get(kname)
	if err != nil {
		return nil, err
	}

	return info, nil
}

func (ctx *session) Password() (string, error) {
	return viper.GetString(flagPassword), nil
}

func (ctx *session) Signer() (txutil.Signer, keys.Info, error) {
	kmgr, err := ctx.KeyManager()
	if err != nil {
		return nil, nil, err
	}

	key, err := ctx.Key()
	if err != nil {
		return nil, nil, err
	}

	password, err := ctx.Password()
	if err != nil {
		return nil, key, err
	}

	signer := txutil.NewKeystoreSigner(kmgr, key.GetName(), password)

	return signer, key, nil
}

func (ctx *session) Nonce() (uint64, error) {
	txclient, err := ctx.TxClient()
	if err != nil {
		return 0, err
	}
	return txclient.Nonce()
}

func (ctx *session) NoWait() bool {
	val, _ := ctx.cmd.Flags().GetBool(flagNoWait)
	return val
}

func (ctx *session) Ctx() context.Context {
	return context.Background()
}

func loadKeyManager(root string) (keys.Keybase, tmdb.DB, error) {

	db := tmdb.NewDB(keyDir, tmdb.GoLevelDBBackend, root)
	manager := keys.New(db)

	return manager, db, nil
}

func (ctx *session) Host() string {
	if ctx.cmd.Flag(flagHost).Value.String() != ctx.cmd.Flag(flagHost).DefValue {
		return ctx.cmd.Flag(flagHost).Value.String()
	}
	return viper.GetString(flagHost)
}
