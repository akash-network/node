package context

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/ovrclk/akash/cmd/akash/constants"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/query"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tendermint/go-crypto/keys"
	"github.com/tendermint/go-crypto/keys/words"
	tmclient "github.com/tendermint/tendermint/rpc/client"
	tmdb "github.com/tendermint/tmlibs/db"
	"github.com/tendermint/tmlibs/log"
)

type Context interface {
	RootDir() string
	KeyManager() (keys.Keybase, error)
	Node() string
	Client() *tmclient.HTTP
	KeyName() string
	KeyType() (keys.CryptoAlgo, error)
	Key() (keys.Info, error)
	Nonce() (uint64, error)
	Log() log.Logger
	Signer() (txutil.Signer, keys.Info, error)
	Wait() bool
}

type cmdRunner func(cmd *cobra.Command, args []string) error
type Runner func(ctx Context, cmd *cobra.Command, args []string) error

func WithContext(fn Runner) cmdRunner {
	return func(cmd *cobra.Command, args []string) error {
		ctx := newContext(cmd)
		defer ctx.shutdown()
		return fn(ctx, cmd, args)
	}
}

func RequireRootDir(fn Runner) Runner {
	return func(ctx Context, cmd *cobra.Command, args []string) error {
		if root := ctx.RootDir(); root == "" {
			return errors.New("root directory unset")
		}
		return fn(ctx, cmd, args)
	}
}

func RequireKeyManager(fn Runner) Runner {
	return RequireRootDir(func(ctx Context, cmd *cobra.Command, args []string) error {
		if _, err := ctx.KeyManager(); err != nil {
			return err
		}
		return fn(ctx, cmd, args)
	})
}

func RequireNode(fn Runner) Runner {
	return func(ctx Context, cmd *cobra.Command, args []string) error {
		if node := ctx.Node(); node == "" {
			return fmt.Errorf("node required")
		}
		return fn(ctx, cmd, args)
	}
}

func RequireKey(fn Runner) Runner {
	return func(ctx Context, cmd *cobra.Command, args []string) error {
		if _, err := ctx.Key(); err != nil {
			return err
		}
		return fn(ctx, cmd, args)
	}
}

func newContext(cmd *cobra.Command) *context {
	return &context{cmd: cmd, mtx: sync.Mutex{}}
}

type context struct {
	cmd  *cobra.Command
	kmgr keys.Keybase
	kdb  tmdb.DB
	log  log.Logger
	mtx  sync.Mutex
}

func (ctx *context) shutdown() {
	ctx.mtx.Lock()
	defer ctx.mtx.Unlock()
	if ctx.kdb != nil {
		ctx.kdb.Close()
	}
}

func (ctx *context) Log() log.Logger {
	ctx.mtx.Lock()
	defer ctx.mtx.Unlock()

	if ctx.log != nil {
		return ctx.log
	}

	ctx.log = common.NewLogger(os.Stdout).With("app", "akash")
	return ctx.log
}

func (ctx *context) RootDir() string {
	root, _ := ctx.cmd.Flags().GetString(constants.FlagRootDir)
	return root
}

func (ctx *context) KeyManager() (keys.Keybase, error) {
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

func (ctx *context) Node() string {
	if ctx.cmd.Flag(constants.FlagNode).Value.String() != ctx.cmd.Flag(constants.FlagNode).DefValue {
		return ctx.cmd.Flag(constants.FlagNode).Value.String()
	}
	return viper.GetString(constants.FlagNode)
}

func (ctx *context) Client() *tmclient.HTTP {
	return tmclient.NewHTTP(ctx.Node(), "/websocket")
}

func (ctx *context) KeyName() string {
	val, _ := ctx.cmd.Flags().GetString(constants.FlagKey)
	return val
}

func (ctx *context) KeyType() (keys.CryptoAlgo, error) {
	return parseFlagKeyType(ctx.cmd.Flags())
}

func (ctx *context) Key() (keys.Info, error) {
	kmgr, err := ctx.KeyManager()
	if err != nil {
		return keys.Info{}, err
	}

	kname := ctx.KeyName()
	if kname == "" {
		return keys.Info{}, errors.New("no key specified")
	}

	info, err := kmgr.Get(kname)
	if err != nil {
		return keys.Info{}, err
	}
	return info, nil
}

func (ctx *context) Password() (string, error) {
	return constants.Password, nil
}

func (ctx *context) Signer() (txutil.Signer, keys.Info, error) {
	kmgr, err := ctx.KeyManager()
	if err != nil {
		return nil, keys.Info{}, err
	}

	key, err := ctx.Key()
	if err != nil {
		return nil, keys.Info{}, err
	}

	password, err := ctx.Password()
	if err != nil {
		return nil, key, err
	}

	signer := txutil.NewKeystoreSigner(kmgr, key.Name, password)

	return signer, key, nil
}

func (ctx *context) Nonce() (uint64, error) {
	nonce, err := ctx.cmd.Flags().GetUint64(constants.FlagNonce)
	if err != nil || nonce == uint64(0) {
		res := new(types.Account)
		client := ctx.Client()
		key, _ := ctx.Key()
		queryPath := query.AccountPath(key.Address())
		result, err := client.ABCIQuery(queryPath, nil)
		if err != nil {
			return 0, err
		}
		res.Unmarshal(result.Response.Value)
		nonce = res.Nonce + 1
	}
	return nonce, nil
}

func (ctx *context) Wait() bool {
	val, _ := ctx.cmd.Flags().GetBool(constants.FlagWait)
	return val
}

func loadKeyManager(root string) (keys.Keybase, tmdb.DB, error) {
	codec, err := words.LoadCodec(constants.Codec)
	if err != nil {
		return nil, nil, err
	}

	db := tmdb.NewDB(constants.KeyDir, tmdb.GoLevelDBBackend, root)
	manager := keys.New(db, codec)

	return manager, db, nil
}
