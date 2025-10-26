package network

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	tmrand "github.com/cometbft/cometbft/libs/rand"
	"github.com/cometbft/cometbft/node"
	tmclient "github.com/cometbft/cometbft/rpc/client"

	"cosmossdk.io/log"
	"cosmossdk.io/math"
	pruningtypes "cosmossdk.io/store/pruning/types"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	srvconfig "github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	cflags "pkg.akt.dev/go/cli/flags"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/app"
)

const (
	portsPerValidator = 2
)

// package-wide network lock to only allow one test network at a time
var (
	lock = new(sync.Mutex)
)

type TestnetFixtureOptions struct {
	EncCfg sdkutil.EncodingConfig
}

type TestnetFixtureOption func(*TestnetFixtureOptions)

func WithEncodingConfig(val sdkutil.EncodingConfig) TestnetFixtureOption {
	return func(opts *TestnetFixtureOptions) {
		opts.EncCfg = val
	}
}

// AppConstructor defines a function which accepts a network configuration and
// creates an ABCI Application to provide to Tendermint.
type (
	AppConstructor     = func(val ValidatorI) servertypes.Application
	TestFixtureFactory = func(opts ...TestnetFixtureOption) TestFixture
)

type TestFixture struct {
	AppConstructor AppConstructor
	GenesisState   map[string]json.RawMessage
	EncodingConfig sdkutil.EncodingConfig
}

// Config defines the necessary configuration used to bootstrap and start an
// in-process local testing network.
type Config struct {
	Codec             codec.Codec
	LegacyAmino       *codec.LegacyAmino // TODO: Remove!
	InterfaceRegistry codectypes.InterfaceRegistry
	TxConfig          sdkclient.TxConfig
	AccountRetriever  sdkclient.AccountRetriever
	AppConstructor    AppConstructor             // the ABCI application constructor
	GenesisState      map[string]json.RawMessage // custom genesis state to provide
	TimeoutCommit     time.Duration              // the consensus commitment timeout
	ChainID           string                     // the network chain-id
	NumValidators     int                        // the total number of validators to create and bond
	Mnemonics         []string                   // custom user-provided validator operator mnemonics
	BondDenom         string                     // the staking bond denomination
	Denoms            []string                   // list of additional denoms could be used on network
	MinGasPrices      string                     // the minimum gas prices each validator will accept
	AccountTokens     math.Int                   // the amount of unique validator tokens (e.g. 1000node0)
	StakingTokens     math.Int                   // the amount of tokens each validator has available to stake
	BondedTokens      math.Int                   // the amount of tokens each validator stakes
	PruningStrategy   string                     // the pruning strategy each validator will have
	EnableLogging     bool                       // enable Tendermint logging to STDOUT
	CleanupDir        bool                       // remove base temporary directory during cleanup
	SigningAlgo       string                     // signing algorithm for keys
	KeyringOptions    []keyring.Option
}

// Network defines a local in-process testing network using SimApp. It can be
// configured to start any number of validators, each with its own RPC and API
// clients. Typically, this test network would be used in client and integration
// testing where user input is expected.
//
// Note, due to Tendermint constraints in regard to RPC functionality, there
// may only be one test network running at a time. Thus, any caller must be
// sure to Cleanup after testing is finished in order to allow other tests
// to create networks. In addition, only the first validator will have a valid
// RPC and API server/client.
type Network struct {
	T          *testing.T
	BaseDir    string
	Validators []*Validator

	Config Config
}

// Validator defines an in-process Tendermint validator node. Through this object,
// a client can make RPC and API calls and interact with any client command
// or handler.
type Validator struct {
	AppConfig  *srvconfig.Config
	ClientCtx  sdkclient.Context
	Ctx        *server.Context
	Dir        string
	NodeID     string
	PubKey     cryptotypes.PubKey
	Moniker    string
	APIAddress string
	RPCAddress string
	P2PAddress string
	Address    sdk.AccAddress
	ValAddress sdk.ValAddress
	RPCClient  tmclient.Client

	app      servertypes.Application
	tmNode   *node.Node
	api      *api.Server
	grpc     *grpc.Server
	grpcWeb  *http.Server
	errGroup *errgroup.Group
	cancelFn context.CancelFunc
}

// ValidatorI expose a validator's context and configuration
type ValidatorI interface {
	GetCtx() *server.Context
	GetAppConfig() *srvconfig.Config
}

func (v Validator) GetCtx() *server.Context {
	return v.Ctx
}

func (v Validator) GetAppConfig() *srvconfig.Config {
	return v.AppConfig
}

// GetFreePorts asks the kernel for free open ports that are ready to use.
func GetFreePorts(count int) ([]int, error) {
	var ports []int

	listeners := make([]*net.TCPListener, 0, count)

	for i := 0; i < count; i++ {
		addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
		if err != nil {
			return nil, err
		}

		l, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return nil, err
		}

		listeners = append(listeners, l)
		ports = append(ports, l.Addr().(*net.TCPAddr).Port)
	}

	for _, l := range listeners {
		if err := l.Close(); err != nil {
			return nil, err
		}
	}

	return ports, nil
}

type freePorts struct {
	lock  sync.Mutex
	idx   int
	ports []int
}

func newFreePorts(ports []int) *freePorts {
	return &freePorts{
		idx:   0,
		ports: ports,
	}
}

func (p *freePorts) mustGetPort() int {
	defer p.lock.Unlock()
	p.lock.Lock()

	if p.idx == len(p.ports) {
		panic("no ports available")
	}

	port := p.ports[p.idx]
	p.idx++

	return port
}

// New creates a new Network for integration tests.
func New(t *testing.T, cfg Config) *Network {
	// only one caller/test can create and use a network at a time
	t.Log("acquiring test network lock")
	lock.Lock()

	baseDir, err := os.MkdirTemp(t.TempDir(), cfg.ChainID)
	require.NoError(t, err)
	t.Logf("created temporary directory: %s", baseDir)

	network := &Network{
		T:          t,
		BaseDir:    baseDir,
		Validators: make([]*Validator, cfg.NumValidators),
		Config:     cfg,
	}

	t.Log("preparing test network...")

	monikers := make([]string, cfg.NumValidators)
	nodeIDs := make([]string, cfg.NumValidators)
	valPubKeys := make([]cryptotypes.PubKey, cfg.NumValidators)

	var genAccounts []authtypes.GenesisAccount
	var genBalances []banktypes.Balance
	var genFiles []string

	buf := bufio.NewReader(os.Stdin)

	allocPortsCount := (portsPerValidator * cfg.NumValidators) + 4

	availablePorts, err := GetFreePorts(allocPortsCount)
	require.NoError(t, err)
	require.Equal(t, allocPortsCount, len(availablePorts))

	ports := newFreePorts(availablePorts)

	// generate private keys, node IDs, and initial transactions
	for i := 0; i < cfg.NumValidators; i++ {
		appCfg := srvconfig.DefaultConfig()
		appCfg.Pruning = cfg.PruningStrategy
		appCfg.MinGasPrices = cfg.MinGasPrices
		appCfg.API.Enable = true
		appCfg.API.Swagger = false
		appCfg.Telemetry.Enabled = false
		appCfg.GRPC.Enable = false
		appCfg.GRPCWeb.Enable = false

		ctx := server.NewDefaultContext()
		ctx.Viper.Set(cflags.FlagChainID, cfg.ChainID)

		// Only allow the first validator to expose an RPC, API and gRPC
		// server/client due to Tendermint in-process constraints.
		apiAddr := ""

		tmCfg := ctx.Config
		tmCfg.Consensus.TimeoutCommit = cfg.TimeoutCommit

		tmCfg.RPC.ListenAddress = ""
		tmCfg.ProxyApp = fmt.Sprintf("tcp://127.0.0.1:%d", ports.mustGetPort())
		tmCfg.P2P.ListenAddress = fmt.Sprintf("tcp://127.0.0.1:%d", ports.mustGetPort())
		tmCfg.P2P.AddrBookStrict = false
		tmCfg.P2P.AllowDuplicateIP = true

		if i == 0 {
			apiListenAddr := fmt.Sprintf("tcp://0.0.0.0:%d", ports.mustGetPort())
			appCfg.API.Address = apiListenAddr

			apiURL, err := url.Parse(apiListenAddr)
			require.NoError(t, err)
			apiAddr = fmt.Sprintf("http://%s:%s", apiURL.Hostname(), apiURL.Port())

			tmCfg.RPC.ListenAddress = fmt.Sprintf("tcp://127.0.0.1:%d", ports.mustGetPort())
			appCfg.GRPC.Address = fmt.Sprintf("127.0.0.1:%d", ports.mustGetPort())
			appCfg.GRPC.Enable = true

			appCfg.GRPCWeb.Enable = true
		}

		logger := log.NewNopLogger()
		if cfg.EnableLogging {
			logger = log.NewLogger(os.Stdout)
		}

		ctx.Logger = logger

		nodeDirName := fmt.Sprintf("node%d", i)
		nodeDir := filepath.Join(network.BaseDir, nodeDirName, "simd")
		clientDir := filepath.Join(network.BaseDir, nodeDirName, "simcli")
		gentxsDir := filepath.Join(network.BaseDir, "gentxs")

		require.NoError(t, os.MkdirAll(filepath.Join(nodeDir, "config"), 0o755)) //nolint: gosec
		require.NoError(t, os.MkdirAll(clientDir, 0o755))                        //nolint: gosec

		tmCfg.SetRoot(nodeDir)
		tmCfg.Moniker = nodeDirName
		monikers[i] = nodeDirName

		nodeID, pubKey, err := genutil.InitializeNodeValidatorFiles(tmCfg)
		require.NoError(t, err)
		nodeIDs[i] = nodeID
		valPubKeys[i] = pubKey

		kb, err := keyring.New(sdk.KeyringServiceName(), keyring.BackendTest, clientDir, buf, cfg.Codec, cfg.KeyringOptions...)
		require.NoError(t, err)

		keyringAlgos, _ := kb.SupportedAlgorithms()
		algo, err := keyring.NewSigningAlgoFromString(cfg.SigningAlgo, keyringAlgos)
		require.NoError(t, err)

		var mnemonic string
		if i < len(cfg.Mnemonics) {
			mnemonic = cfg.Mnemonics[i]
		}

		addr, secret, err := testutil.GenerateSaveCoinKey(kb, nodeDirName, mnemonic, true, algo)
		require.NoError(t, err)

		info := map[string]string{"secret": secret}
		infoBz, err := json.Marshal(info)
		require.NoError(t, err)

		// save private key seed words
		require.NoError(t, writeFile(fmt.Sprintf("%v.json", "key_seed"), clientDir, infoBz))

		balances := make(sdk.Coins, 0, len(cfg.Denoms)+1)
		balances = append(balances, sdk.NewCoin(cfg.BondDenom, cfg.StakingTokens))

		for _, denom := range cfg.Denoms {
			balances = append(balances, sdk.NewCoin(denom, cfg.AccountTokens))
		}

		genFiles = append(genFiles, tmCfg.GenesisFile())
		genBalances = append(genBalances, banktypes.Balance{Address: addr.String(), Coins: balances.Sort()})
		genAccounts = append(genAccounts, authtypes.NewBaseAccount(addr, nil, 0, 0))

		commission, err := math.LegacyNewDecFromStr("0.5")
		require.NoError(t, err)

		createValMsg, err := stakingtypes.NewMsgCreateValidator(
			sdk.ValAddress(addr).String(),
			valPubKeys[i],
			sdk.NewCoin(cfg.BondDenom, cfg.BondedTokens),
			stakingtypes.NewDescription(nodeDirName, "", "", "", ""),
			stakingtypes.NewCommissionRates(commission, math.LegacyOneDec(), math.LegacyOneDec()),
			math.OneInt(),
		)
		require.NoError(t, err)

		p2pURL, err := url.Parse(tmCfg.P2P.ListenAddress)
		require.NoError(t, err)

		memo := fmt.Sprintf("%s@%s:%s", nodeIDs[i], p2pURL.Hostname(), p2pURL.Port())
		fee := sdk.NewCoins(sdk.NewCoin(fmt.Sprintf("%stoken", nodeDirName), math.NewInt(0)))
		txBuilder := cfg.TxConfig.NewTxBuilder()
		require.NoError(t, txBuilder.SetMsgs(createValMsg))
		txBuilder.SetFeeAmount(fee)    // Arbitrary fee
		txBuilder.SetGasLimit(1000000) // Need at least 100386
		txBuilder.SetMemo(memo)

		txFactory := tx.Factory{}
		txFactory = txFactory.
			WithChainID(cfg.ChainID).
			WithMemo(memo).
			WithKeybase(kb).
			WithTxConfig(cfg.TxConfig)

		err = tx.Sign(context.Background(), txFactory, nodeDirName, txBuilder, true)
		require.NoError(t, err)

		txBz, err := cfg.TxConfig.TxJSONEncoder()(txBuilder.GetTx())
		require.NoError(t, err)
		require.NoError(t, writeFile(fmt.Sprintf("%v.json", nodeDirName), gentxsDir, txBz))

		srvconfig.WriteConfigFile(filepath.Join(nodeDir, "config/app.toml"), appCfg)

		cctx := sdkclient.Context{}.
			WithKeyringDir(clientDir).
			WithKeyring(kb).
			WithHomeDir(tmCfg.RootDir).
			WithChainID(cfg.ChainID).
			WithInterfaceRegistry(cfg.InterfaceRegistry).
			WithCodec(cfg.Codec).
			WithLegacyAmino(cfg.LegacyAmino).
			WithTxConfig(cfg.TxConfig).
			WithAccountRetriever(cfg.AccountRetriever).
			WithNodeURI(tmCfg.RPC.ListenAddress).
			WithBroadcastMode("block").
			WithSignModeStr("direct").
			WithFromAddress(addr).
			WithSkipConfirmation(true)

		network.Validators[i] = &Validator{
			AppConfig:  appCfg,
			ClientCtx:  cctx,
			Ctx:        ctx,
			Dir:        filepath.Join(network.BaseDir, nodeDirName),
			NodeID:     nodeID,
			PubKey:     pubKey,
			Moniker:    nodeDirName,
			RPCAddress: tmCfg.RPC.ListenAddress,
			P2PAddress: tmCfg.P2P.ListenAddress,
			APIAddress: apiAddr,
			Address:    addr,
			ValAddress: sdk.ValAddress(addr),
		}
	}

	require.NoError(t, initGenFiles(cfg, genAccounts, genBalances, genFiles))
	require.NoError(t, collectGenFiles(cfg, network.Validators, network.BaseDir))

	t.Log("starting test network...")
	for _, v := range network.Validators {
		require.NoError(t, startInProcess(cfg, v))
	}

	t.Log("started test network")

	// Ensure we clean up incase any test was abruptly halted (e.g. SIGINT) as any
	// defer in a test would not be called.
	trapSignal(network.Cleanup)

	return network
}

// trapSignal traps SIGINT and SIGTERM and calls os.Exit once a signal is received.
func trapSignal(cleanupFunc func()) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigs

		if cleanupFunc != nil {
			cleanupFunc()
		}
		exitCode := 128

		switch sig {
		case syscall.SIGINT:
			exitCode += int(syscall.SIGINT)
		case syscall.SIGTERM:
			exitCode += int(syscall.SIGTERM)
		}

		os.Exit(exitCode)
	}()
}

// LatestHeight returns the latest height of the network or an error if the
// query fails or no validators exist.
func (n *Network) LatestHeight() (int64, error) {
	if len(n.Validators) == 0 {
		return 0, errors.New("no validators available")
	}

	status, err := n.Validators[0].RPCClient.Status(context.Background())
	if err != nil {
		return 0, err
	}

	return status.SyncInfo.LatestBlockHeight, nil
}

// WaitForHeight performs a blocking check where it waits for a block to be
// committed after a given block. If that height is not reached within a timeout,
// an error is returned. Regardless, the latest height queried is returned.
func (n *Network) WaitForHeight(h int64) (int64, error) {
	return n.WaitForHeightWithTimeout(h, 10*time.Second)
}

// WaitForHeightWithTimeout is the same as WaitForHeight except the caller can
// provide a custom timeout.
func (n *Network) WaitForHeightWithTimeout(h int64, t time.Duration) (int64, error) {
	ticker := time.NewTicker(time.Second)
	timeout := time.After(t)

	if len(n.Validators) == 0 {
		return 0, errors.New("no validators available")
	}

	var latestHeight int64
	val := n.Validators[0]

	for {
		select {
		case <-timeout:
			ticker.Stop()
			return latestHeight, errors.New("timeout exceeded waiting for block")
		case <-ticker.C:
			status, err := val.RPCClient.Status(context.Background())
			if err == nil && status != nil {
				latestHeight = status.SyncInfo.LatestBlockHeight
				if latestHeight >= h {
					return latestHeight, nil
				}
			}
		}
	}
}

// WaitForNextBlock waits for the next block to be committed, returning an error
// upon failure.
func (n *Network) WaitForNextBlock() error {
	lastBlock, err := n.LatestHeight()
	if err != nil {
		return err
	}

	_, err = n.WaitForHeight(lastBlock + 1)
	if err != nil {
		return err
	}

	return err
}

// WaitForBlocks waits for the next amount of blocks to be committed, returning an error
// upon failure.
func (n *Network) WaitForBlocks(blocks int64) error {
	lastBlock, err := n.LatestHeight()
	if err != nil {
		return err
	}

	_, err = n.WaitForHeight(lastBlock + blocks)
	if err != nil {
		return err
	}

	return err
}

// Cleanup removes the root testing (temporary) directory and stops both the
// Tendermint and API services. It allows other callers to create and start
// test networks. This method must be called when a test is finished, typically
// in defer.
func (n *Network) Cleanup() {
	defer func() {
		lock.Unlock()
		n.T.Log("released test network lock")
	}()

	n.T.Log("cleaning up test network...")

	for _, v := range n.Validators {
		if v.tmNode != nil && v.tmNode.IsRunning() {
			_ = v.tmNode.Stop()
		}

		if v.api != nil {
			_ = v.api.Close()
		}

		if v.grpc != nil {
			v.grpc.Stop()
			if v.grpcWeb != nil {
				_ = v.grpcWeb.Close()
			}
		}
	}

	if n.Config.CleanupDir {
		_ = os.RemoveAll(n.BaseDir)
	}

	n.T.Log("finished cleaning up test network")
}

// DefaultConfig returns a default configuration suitable for nearly all
// testing requirements.
func DefaultConfig(factory TestFixtureFactory, opts ...ConfigOption) Config {
	fixture := factory()

	cfg := &networkConfigOptions{}
	for _, opt := range opts {
		opt(cfg)
	}

	cdc := fixture.EncodingConfig.Codec
	genesisState := app.NewDefaultGenesisState(cdc)

	if cfg.interceptState != nil {
		for k, v := range genesisState {
			res := cfg.interceptState(cdc, k, v)
			if res != nil {
				genesisState[k] = res
			}
		}
	}

	fixture.GenesisState = genesisState

	const coinDenom = "uakt"
	return Config{
		Codec:             fixture.EncodingConfig.Codec,
		TxConfig:          fixture.EncodingConfig.TxConfig,
		LegacyAmino:       fixture.EncodingConfig.Amino,
		InterfaceRegistry: fixture.EncodingConfig.InterfaceRegistry,
		AccountRetriever:  authtypes.AccountRetriever{},
		AppConstructor:    fixture.AppConstructor,
		GenesisState:      fixture.GenesisState,
		TimeoutCommit:     2 * time.Second,
		ChainID:           "chain-" + tmrand.NewRand().Str(6),
		NumValidators:     4,
		BondDenom:         coinDenom,
		Denoms: []string{
			coinDenom,
			"ibc/12C6A0C374171B595A0A9E18B83FA09D295FB1F2D8C6DAA3AC28683471752D84",
		},
		MinGasPrices:    fmt.Sprintf("0.000006%s", coinDenom),
		AccountTokens:   sdk.TokensFromConsensusPower(1000000000000, sdk.DefaultPowerReduction),
		StakingTokens:   sdk.TokensFromConsensusPower(100000, sdk.DefaultPowerReduction),
		BondedTokens:    sdk.TokensFromConsensusPower(100, sdk.DefaultPowerReduction),
		PruningStrategy: pruningtypes.PruningOptionNothing,
		CleanupDir:      true,
		SigningAlgo:     string(hd.Secp256k1Type),
		KeyringOptions:  []keyring.Option{},
	}
}
