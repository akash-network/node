package context

import (
	"errors"
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/ovrclk/akash/cmd/akash/constants"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tendermint/go-crypto/keys"
	"github.com/tendermint/go-crypto/keys/cryptostore"
	"github.com/tendermint/go-crypto/keys/storage/filestorage"
	tmclient "github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tmlibs/log"
)

type Context interface {
	RootDir() string
	KeyManager() (keys.Manager, error)
	Node() string
	Client() *tmclient.HTTP
	KeyName() string
	Key() (keys.Info, error)
	Nonce() (uint64, error)
	Log() log.Logger
}

type cmdRunner func(cmd *cobra.Command, args []string) error
type Runner func(ctx Context, cmd *cobra.Command, args []string) error

func WithContext(fn Runner) cmdRunner {
	return func(cmd *cobra.Command, args []string) error {
		ctx := NewContext(cmd)
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

func NewContext(cmd *cobra.Command) Context {
	return &context{cmd: cmd, mtx: sync.Mutex{}}
}

type context struct {
	cmd  *cobra.Command
	kmgr keys.Manager
	log  log.Logger
	mtx  sync.Mutex
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

func (ctx *context) KeyManager() (keys.Manager, error) {
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
	ctx.kmgr, err = loadKeyManager(root)

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

func (ctx *context) Nonce() (uint64, error) {
	nonce, err := ctx.cmd.Flags().GetUint64(constants.FlagNonce)
	if err != nil || nonce == uint64(0) {
		res := new(types.Account)
		client := tmclient.NewHTTP(ctx.Node(), "/websocket")
		key, _ := ctx.Key()
		queryPath := state.AccountPath + key.Address.String()
		result, err := client.ABCIQuery(queryPath, nil)
		if err != nil {
			return 0, err
		}
		res.Unmarshal(result.Response.Value)
		nonce = res.Nonce + 1
	}
	return nonce, nil
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

func loadKeyManager(root string) (keys.Manager, error) {
	codec, err := keys.LoadCodec(constants.Codec)
	if err != nil {
		return nil, err
	}
	manager := cryptostore.New(
		cryptostore.SecretBox,
		filestorage.New(path.Join(root, constants.KeyDir)),
		codec,
	)
	return manager, nil
}
