package context

import (
	"errors"
	"fmt"
	"path"

	"github.com/ovrclk/photon/cmd/photon/constants"
	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/types"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tendermint/go-crypto/keys"
	"github.com/tendermint/go-crypto/keys/cryptostore"
	"github.com/tendermint/go-crypto/keys/storage/filestorage"
	tmclient "github.com/tendermint/tendermint/rpc/client"
)

type Context interface {
	RootDir() string
	KeyManager() (keys.Manager, error)
	Node() string
	Key() (keys.Info, error)
	Nonce() uint64
}

type cmdRunner func(cmd *cobra.Command, args []string) error
type ctxRunner func(ctx Context, cmd *cobra.Command, args []string) error

func WithContext(fn ctxRunner) cmdRunner {
	return func(cmd *cobra.Command, args []string) error {
		ctx := NewContext(cmd)
		return fn(ctx, cmd, args)
	}
}

func RequireRootDir(fn ctxRunner) ctxRunner {
	return func(ctx Context, cmd *cobra.Command, args []string) error {
		if root := ctx.RootDir(); root == "" {
			return errors.New("root directory unset")
		}
		return fn(ctx, cmd, args)
	}
}

func RequireKeyManager(fn ctxRunner) ctxRunner {
	return RequireRootDir(func(ctx Context, cmd *cobra.Command, args []string) error {
		if _, err := ctx.KeyManager(); err != nil {
			return err
		}
		return fn(ctx, cmd, args)
	})
}

func RequireNode(fn ctxRunner) ctxRunner {
	return func(ctx Context, cmd *cobra.Command, args []string) error {
		if node := ctx.Node(); node == "" {
			return fmt.Errorf("node required")
		}
		return fn(ctx, cmd, args)
	}
}

func RequireKey(fn ctxRunner) ctxRunner {
	return func(ctx Context, cmd *cobra.Command, args []string) error {
		if _, err := ctx.Key(); err != nil {
			return err
		}
		return fn(ctx, cmd, args)
	}
}

func NewContext(cmd *cobra.Command) Context {
	return &context{cmd: cmd}
}

type context struct {
	cmd  *cobra.Command
	kmgr keys.Manager
}

func (ctx *context) RootDir() string {
	root, _ := ctx.cmd.Flags().GetString(constants.FlagRootDir)
	return root
}

func (ctx *context) KeyManager() (keys.Manager, error) {

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
	val := viper.GetString(constants.FlagNode)
	return val
}

func (ctx *context) Nonce() uint64 {
	nonce, err := ctx.cmd.Flags().GetUint64(constants.FlagNonce)
	if err != nil || nonce == uint64(0) {
		res := new(types.Account)
		client := tmclient.NewHTTP(ctx.Node(), "/websocket")
		key, _ := ctx.Key()
		queryPath := state.AccountPath + key.Address.String()
		result, _ := client.ABCIQuery(queryPath, nil)
		res.Unmarshal(result.Response.Value)
		nonce = res.Nonce + 1
	}
	return nonce
}

func (ctx *context) Key() (keys.Info, error) {
	kmgr, err := ctx.KeyManager()
	if err != nil {
		return keys.Info{}, err
	}

	kname, err := ctx.cmd.Flags().GetString(constants.FlagKey)
	if err != nil {
		return keys.Info{}, err
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
