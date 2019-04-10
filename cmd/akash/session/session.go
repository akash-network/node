package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/query"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/util/uiutil"

	"github.com/cosmos/cosmos-sdk/crypto/keys"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	tmdb "github.com/tendermint/tendermint/libs/db"
	"github.com/tendermint/tendermint/libs/log"
	tmclient "github.com/tendermint/tendermint/rpc/client"
)

// KeybaseName is the default name of the Keybase
var KeybaseName = "akash"

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
	Printer() uiutil.Printer
	Mode() Mode
}

type cmdRunner func(cmd *cobra.Command, args []string) error
type Runner func(sess Session, cmd *cobra.Command, args []string) error

func WithSession(fn Runner) cmdRunner {
	return func(cmd *cobra.Command, args []string) error {
		return common.RunForever(func(ctx context.Context) error {
			session := newSession(ctx, cmd)
			defer session.shutdown()
			mtypeS, err := session.cmd.Flags().GetString(flagMode)
			if err != nil {
				return err
			}
			session.mode, err = NewMode(ModeType(mtypeS))
			if err != nil {
				return err
			}
			if err := fn(session, cmd, args); err != context.Canceled {
				return err
			}
			return nil
		})
	}
}

func WithPrinter(fn Runner) Runner {
	return func(session Session, cmd *cobra.Command, args []string) error {
		defer session.Printer().Flush()
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

func newSession(ctx context.Context, cmd *cobra.Command) *session {
	return &session{ctx: ctx, cmd: cmd, mtx: sync.Mutex{}}
}

type session struct {
	cmd     *cobra.Command
	kmgr    keys.Keybase
	kdb     tmdb.DB
	log     log.Logger
	ctx     context.Context
	mtx     sync.Mutex
	printer uiutil.Printer
	mode    Mode
}

func (s *session) shutdown() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.kdb != nil {
		s.kdb.Close()
	}
}

func (s *session) Log() log.Logger {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.log != nil {
		return s.log
	}

	s.log = common.NewLogger(os.Stdout).With("app", "akash")
	s.log = log.NewFilter(s.log, log.AllowAll())
	return s.log
}

func (s *session) RootDir() string {
	root, _ := s.cmd.Flags().GetString(flagRootDir)
	return root
}

func (s *session) KeyManager() (keys.Keybase, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.kmgr != nil {
		return s.kmgr, nil
	}

	root := s.RootDir()
	if root == "" {
		return nil, errors.New("root directory unset")
	}

	var err error
	s.kmgr, s.kdb, err = loadKeyManager(root)

	return s.kmgr, err
}

func (s *session) Node() string {
	if s.cmd.Flag(flagNode).Value.String() != s.cmd.Flag(flagNode).DefValue {
		return s.cmd.Flag(flagNode).Value.String()
	}
	return viper.GetString(flagNode)
}

func (s *session) Client() *tmclient.HTTP {
	return tmclient.NewHTTP(s.Node(), "/websocket")
}

func (s *session) TxClient() (txutil.Client, error) {
	signer, key, err := s.Signer()
	if err != nil {
		return nil, err
	}
	nonce, err := s.cmd.Flags().GetUint64(flagNonce)
	if err != nil {
		nonce = 0
	}
	return txutil.NewClient(s.Client(), signer, key, nonce), nil
}

func (s *session) QueryClient() query.Client {
	return query.NewClient(s.Client())
}

func (s *session) KeyName() string {
	val, _ := s.cmd.Flags().GetString(flagKey)
	return val
}

func (s *session) KeyType() (keys.SigningAlgo, error) {
	return parseFlagKeyType(s.cmd.Flags())
}

func (s *session) Key() (keys.Info, error) {
	kmgr, err := s.KeyManager()
	if err != nil {
		return nil, err
	}

	kname := s.KeyName()
	if kname == "" {
		return nil, errors.New("no key specified")
	}

	info, err := kmgr.Get(kname)
	if err != nil {
		return nil, err
	}

	return info, nil
}

func (s *session) Password() (string, error) {
	return viper.GetString(flagPassword), nil
}

func (s *session) Signer() (txutil.Signer, keys.Info, error) {
	kmgr, err := s.KeyManager()
	if err != nil {
		return nil, nil, err
	}

	key, err := s.Key()
	if err != nil {
		return nil, nil, err
	}

	password, err := s.Password()
	if err != nil {
		return nil, key, err
	}

	signer := txutil.NewKeystoreSigner(kmgr, key.GetName(), password)

	return signer, key, nil
}

func (s *session) Nonce() (uint64, error) {
	txclient, err := s.TxClient()
	if err != nil {
		return 0, err
	}
	return txclient.Nonce()
}

func (s *session) NoWait() bool {
	val, _ := s.cmd.Flags().GetBool(flagNoWait)
	return val
}

func (s *session) Ctx() context.Context {
	return s.ctx
}

func loadKeyManager(root string) (keys.Keybase, tmdb.DB, error) {
	db := tmdb.NewDB(keyDir, tmdb.GoLevelDBBackend, root)
	manager := keys.New(KeybaseName, path.Join(root, keyDir))
	return manager, db, nil
}

func (s *session) Host() string {
	if s.cmd.Flag(flagHost).Value.String() != s.cmd.Flag(flagHost).DefValue {
		return s.cmd.Flag(flagHost).Value.String()
	}
	return viper.GetString(flagHost)
}

func (s *session) Mode() Mode {
	return s.mode
}

func (s *session) Printer() uiutil.Printer {
	if s.printer == nil {
		s.printer = uiutil.NewPrinter(nil)
	}
	return s.printer
}
