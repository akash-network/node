//go:build e2e.upgrade

package upgrade

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"
	"golang.org/x/sync/errgroup"

	sdk "github.com/cosmos/cosmos-sdk/types"

	// init sdk config
	_ "pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/pubsub"
	uttypes "pkg.akt.dev/node/tests/upgrade/types"
	"pkg.akt.dev/node/util/cli"
)

const (
	blockTimeWindow = 20 * time.Minute
)

type nodeEvent int
type watchdogCtrl int
type nodeTestStage int
type testStage int
type testModuleStatus int

const (
	nodeEventStart nodeEvent = iota
	nodeEventReplayBlocksStart
	nodeEventReplayBlocksDone
	nodeEventBlockCommited
	nodeEventUpgradeDetected
	nodeEventAddedModule
	nodeEventRemovedModule
	nodeEventModuleMigration
)

const (
	watchdogCtrlStart watchdogCtrl = iota
	watchdogCtrlPause
	watchdogCtrlStop
	watchdogCtrlBlock
)

const (
	nodeTestStagePreUpgrade nodeTestStage = iota
	nodeTestStageUpgrade
	nodeTestStagePostUpgrade1
	nodeTestStagePostUpgrade2
)

const (
	testStagePreUpgrade testStage = iota
	testStageUpgrade
	testStagePostUpgrade
)

const (
	testModuleStatusUnexpected testModuleStatus = iota
	testModuleStatusNotChecked
	testModuleStatusChecked
)

type publisher interface {
	Publish(pubsub.Event) error
}

var (
	nodeTestStageMapStr = map[nodeTestStage]string{
		nodeTestStagePreUpgrade:   "preupgrade",
		nodeTestStageUpgrade:      "upgrade",
		nodeTestStagePostUpgrade1: "postupgrade1",
		nodeTestStagePostUpgrade2: "postupgrade2",
	}

	testModuleStatusMapStr = map[testModuleStatus]string{
		testModuleStatusUnexpected: "unexpected",
		testModuleStatusNotChecked: "notchecked",
		testModuleStatusChecked:    "checked",
	}
)

type eventCtxModule struct {
	name string
}

type eventCtxModuleMigration struct {
	name string
	from string
	to   string
}

type event struct {
	id  nodeEvent
	ctx interface{}
}

type nodePreUpgradeReady struct {
	name string
}

type nodePostUpgradeReady struct {
	name string
}

type postUpgradeTestDone struct{}

type eventShutdown struct{}

type wdReq struct {
	event watchdogCtrl
	resp  chan<- struct{}
}

type testMigration struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type moduleMigrationVersions []testMigration

type testCase struct {
	Modules struct {
		Added   []string `json:"added"`
		Removed []string `json:"removed"`
		Renamed struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"renamed"`
	} `json:"modules"`
	Migrations map[string]moduleMigrationVersions `json:"migrations"`
}

type testCases map[string]testCase

type validatorParams struct {
	home        string
	homedir     string
	name        string
	nodeID      string
	cosmovisor  string
	isRPC       bool
	p2pPort     uint16
	rpcPort     uint16
	upgradeName string
	env         []string
	bus         pubsub.Publisher
}

type validator struct {
	t                 *testing.T
	pubsub            pubsub.Bus
	ctx               context.Context
	cancel            context.CancelFunc
	group             *errgroup.Group
	upgradeInfo       string
	params            validatorParams
	tConfig           testCase
	upgradeSuccessful chan struct{}
	testErrsCh        chan []string
}

type testConfigWork struct {
	Home string `json:"home"`
	Key  string `json:"key"`
}

type testConfig struct {
	ChainID    string         `json:"chain-id"`
	Validators []string       `json:"validators"`
	Work       testConfigWork `json:"work"`
}

type commander struct {
	t   *testing.T
	bin string
	env []string
}

type upgradeTest struct {
	t                 *testing.T
	ctx               context.Context
	cancel            context.CancelFunc
	group             *errgroup.Group
	cmdr              *commander
	cacheDir          string
	upgradeName       string
	upgradeInfo       string
	postUpgradeParams uttypes.TestParams
	validators        map[string]*validator
}

type nodePortsRPC struct {
	port uint16
	grpc uint16
}
type nodeInitParams struct {
	nodeID      string
	homedir     string
	rpc         nodePortsRPC
	p2pPort     uint16
	grpcPort    uint16
	grpcWebPort uint16
	pprofPort   uint16
	apiPort     uint16
}

var (
	workdir        = flag.String("workdir", "", "work directory")
	sourcesdir     = flag.String("sourcesdir", "", "sources directory")
	config         = flag.String("config", "", "config file")
	cosmovisor     = flag.String("cosmovisor", "", "path to cosmovisor")
	upgradeVersion = flag.String("upgrade-version", "local", "akash release to download. local if it is built locally")
	upgradeName    = flag.String("upgrade-name", "", "name of the upgrade")
	testCasesFile  = flag.String("test-cases", "", "")
)

func (cmd *commander) execute(ctx context.Context, args string) ([]byte, error) {
	cmdString := fmt.Sprintf("%s %s", cmd.bin, args)

	cmd.t.Logf("executing cmd: %s\n", cmdString)
	cmdRes, err := executeCommand(ctx, cmd.env, "bash", "-c", cmdString)
	if err != nil {
		cmd.t.Logf("executing cmd failed: %s\n", string(cmdRes))
		return nil, err
	}

	return cmdRes, nil
}

func TestUpgrade(t *testing.T) {
	cores := runtime.NumCPU() - 2
	if cores < 1 {
		cores = 1
	}

	runtime.GOMAXPROCS(cores)

	ctx := context.Background()

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM)
	defer stop()

	t.Log("detecting arguments")

	require.NotEqual(t, "", *workdir, "empty workdir flag")
	require.NotEqual(t, "", *sourcesdir, "empty sourcesdir flag")
	require.NotEqual(t, "", *config, "empty config flag")
	require.NotEqual(t, "", *upgradeVersion, "empty upgrade-version flag")
	require.NotEqual(t, "", *upgradeName, "empty upgrade-name flag")
	require.NotEqual(t, "", *cosmovisor, "empty cosmovisor flag")
	require.NotEqual(t, "", *testCasesFile, "empty test-cases flag")

	if *upgradeVersion != "local" && !semver.IsValid(*upgradeVersion) {
		require.Fail(t, "upgrade-name contains invalid value. expected local|<semver>")
	}

	info, err := os.Stat(*workdir)
	require.NoError(t, err)
	require.True(t, info.IsDir(), "workdir flag is not a dir")

	*workdir = strings.TrimSuffix(*workdir, "/")
	*sourcesdir = strings.TrimSuffix(*sourcesdir, "/")

	info, err = os.Stat(*cosmovisor)
	require.NoError(t, err)
	require.False(t, info.IsDir(), "value in cosmovisor flag is not a file")
	require.True(t, isOwnerExecutable(info.Mode()), "cosmovisor must be executable file")

	var upgradeInfo string

	if *upgradeVersion != "local" {
		t.Logf("generating upgradeinfo from release %s", *upgradeVersion)
		upgradeInfo, err = cli.UpgradeInfoFromTag(ctx, *upgradeVersion, false)
		require.NoError(t, err)
		require.NotEqual(t, "", upgradeInfo)
	}

	var cfg testConfig

	{
		cfgFile, err := os.Open(*config)
		require.NoError(t, err)
		defer func() {
			_ = cfgFile.Close()
		}()
		cfgData, err := io.ReadAll(cfgFile)
		require.NoError(t, err)
		err = json.Unmarshal(cfgData, &cfg)
		require.NoError(t, err)
	}

	var tConfig testCase
	// load testcases config
	{
		tCases := make(testCases)

		tFile, err := os.Open(*testCasesFile)
		require.NoError(t, err)
		defer func() {
			_ = tFile.Close()
		}()

		data, err := io.ReadAll(tFile)
		require.NoError(t, err)

		err = json.Unmarshal(data, &tCases)
		require.NoError(t, err)

		var valid bool
		tConfig, valid = tCases[*upgradeName]
		require.True(t, valid)
	}

	cmdr := &commander{}

	validatorsParams := make(map[string]validatorParams)

	bus := pubsub.NewBus()

	initParams := make(map[string]nodeInitParams)

	postUpgradeParams := uttypes.TestParams{}

	var upgradeCache string
	for idx, name := range cfg.Validators {
		homedir := fmt.Sprintf("%s/%s", *workdir, name)

		if idx == 0 {
			upgradeCache = homedir
		}

		genesisBin := fmt.Sprintf("%s/cosmovisor/genesis/bin/akash", homedir)

		info, err = os.Stat(genesisBin)
		require.NoError(t, err)
		require.False(t, info.IsDir(), "value in genesis-binary flag is not a file")
		require.True(t, isOwnerExecutable(info.Mode()), "akash must be executable file")

		valCmd := &commander{
			t:   t,
			bin: genesisBin,
			env: []string{
				fmt.Sprintf("HOME=%s", *workdir),
				fmt.Sprintf("AKASH_HOME=%s", homedir),
				fmt.Sprintf("AKASH_NODE=tcp://127.0.0.1:26657"),
				fmt.Sprintf("AKASH_KEYRING_BACKEND=test"),
				fmt.Sprintf("AKASH_BROADCAST_MODE=block"),
				fmt.Sprintf("AKASH_CHAIN_ID=%s", cfg.ChainID),
				fmt.Sprintf("AKASH_FROM=%s", cfg.Work.Key),
				fmt.Sprintf("AKASH_GAS_PRICES=0.0025uakt"),
				fmt.Sprintf("AKASH_GAS_ADJUSTMENT=2"),
				fmt.Sprintf("AKASH_P2P_PEX=false"),
				fmt.Sprintf("AKASH_MINIMUM_GAS_PRICES=0.0025uakt"),
				//fmt.Sprintf("AKASH_GAS=auto"),
				// auto is failing with rpc error: code = Unknown desc = unknown query path: unknown request
				fmt.Sprintf("AKASH_GAS=500000"),
				fmt.Sprintf("AKASH_YES=true"),
			},
		}

		if cfg.Work.Home == name {
			cmdr = valCmd

			output, err := cmdr.execute(ctx, fmt.Sprintf("keys show %s -a", cfg.Work.Key))
			require.NoError(t, err)

			addr, err := sdk.AccAddressFromBech32(strings.Trim(string(output), "\n"))
			require.NoError(t, err)

			t.Logf("validator address: \"%s\"", addr.String())

			postUpgradeParams.Home = homedir
			postUpgradeParams.SourceDir = *sourcesdir
			postUpgradeParams.ChainID = cfg.ChainID
			postUpgradeParams.Node = "tcp://127.0.0.1:26657"
			postUpgradeParams.KeyringBackend = "test"
			postUpgradeParams.From = cfg.Work.Key
			postUpgradeParams.FromAddress = addr
		}

		cmdr.env = append(cmdr.env, fmt.Sprintf("AKASH_OUTPUT=json"))

		if *upgradeVersion == "local" {
			upgradeBin := fmt.Sprintf("%s/cosmovisor/upgrades/%s/bin/akash", homedir, *upgradeName)

			info, err = os.Stat(upgradeBin)
			require.NoError(t, err)
			require.False(t, info.IsDir(), "value in upgrade-binary flag is not a file")
			require.True(t, isOwnerExecutable(info.Mode()), "akash must be executable file")
		}

		res, err := valCmd.execute(ctx, "tendermint show-node-id")
		require.NoError(t, err)

		p2pPort := 26656 + uint16(idx*2)

		initParams[name] = nodeInitParams{
			nodeID:  strings.TrimSpace(string(res)),
			homedir: homedir,
			p2pPort: p2pPort,
			rpc: nodePortsRPC{
				port: p2pPort + 1,
				grpc: 9092 + uint16(idx*3),
			},
			grpcPort:    9090 + uint16(idx*3),
			grpcWebPort: 9091 + uint16(idx*3),
			pprofPort:   6060 + uint16(idx),
			apiPort:     1317 + uint16(idx),
		}
	}

	listenAddr := "127.0.0.1"

	for name, params := range initParams {
		unconditionalPeerIDs := make([]string, 0, len(initParams))
		persistentPeers := make([]string, 0, len(initParams))

		for nm1, params1 := range initParams {
			if name == nm1 {
				continue
			}

			unconditionalPeerIDs = append(unconditionalPeerIDs, params1.nodeID)
			persistentPeers = append(persistentPeers, fmt.Sprintf("%s@%s:%d", params1.nodeID, listenAddr, params1.p2pPort))
		}

		validatorsParams[name] = validatorParams{
			home:        *workdir,
			homedir:     params.homedir,
			name:        name,
			nodeID:      params.nodeID,
			cosmovisor:  *cosmovisor,
			isRPC:       cfg.Work.Home == name,
			p2pPort:     params.p2pPort,
			rpcPort:     params.rpc.port,
			upgradeName: *upgradeName,
			bus:         bus,
			env: []string{
				fmt.Sprintf("DAEMON_HOME=%s", params.homedir),
				fmt.Sprintf("HOME=%s", *workdir),
				fmt.Sprintf("AKASH_HOME=%s", params.homedir),
				fmt.Sprintf("AKASH_CHAIN_ID=%s", cfg.ChainID),
				fmt.Sprintf("AKASH_P2P_PERSISTENT_PEERS=%s", strings.Join(persistentPeers, ",")),
				fmt.Sprintf("AKASH_P2P_UNCONDITIONAL_PEER_IDS=%s", strings.Join(unconditionalPeerIDs, ",")),
				fmt.Sprintf("AKASH_P2P_LADDR=tcp://%s:%d", listenAddr, params.p2pPort),
				fmt.Sprintf("AKASH_RPC_LADDR=tcp://%s:%d", listenAddr, params.rpc.port),
				fmt.Sprintf("AKASH_RPC_GRPC_LADDR=tcp://%s:%d", listenAddr, params.rpc.grpc),
				fmt.Sprintf("AKASH_RPC_PPROF_LADDR=%s:%d", listenAddr, params.pprofPort),
				fmt.Sprintf("AKASH_GRPC_ADDRESS=%s:%d", listenAddr, params.grpcPort),
				fmt.Sprintf("AKASH_GRPC_WEB_ADDRESS=%s:%d", listenAddr, params.grpcWebPort),
				fmt.Sprintf("AKASH_API_ADDRESS=tcp://%s:%d", listenAddr, params.apiPort),
				"DAEMON_NAME=akash",
				"DAEMON_RESTART_AFTER_UPGRADE=true",
				"DAEMON_ALLOW_DOWNLOAD_BINARIES=true",
				"DAEMON_RESTART_DELAY=3s",
				"COSMOVISOR_COLOR_LOGS=false",
				"UNSAFE_SKIP_BACKUP=true",
				"AKASH_KEYRING_BACKEND=test",
				"AKASH_P2P_PEX=false",
				"AKASH_P2P_ADDR_BOOK_STRICT=false",
				"AKASH_P2P_ALLOW_DUPLICATE_IP=true",
				"AKASH_P2P_SEEDS=",
				"AKASH_MINIMUM_GAS_PRICES=0.0025uakt",
				"AKASH_FAST_SYNC=false",
				"AKASH_LOG_COLOR=false",
				"AKASH_LOG_TIMESTAMP=",
				"AKASH_LOG_FORMAT=plain",
				"AKASH_STATESYNC_ENABLE=false",
				"AKASH_TX_INDEX_INDEXER=null",
				"AKASH_GRPC_ENABLE=true",
				"AKASH_GRPC_WEB_ENABLE=true",
				"AKASH_API_ENABLE=true",
				"AKASH_PRUNING=nothing",
			},
		}
	}

	group, ctx := errgroup.WithContext(ctx)

	validators := make(map[string]*validator)

	for val, params := range validatorsParams {
		validators[val] = newValidator(ctx, t, params, tConfig)
	}

	utester := &upgradeTest{
		t:                 t,
		ctx:               ctx,
		group:             group,
		cmdr:              cmdr,
		cacheDir:          upgradeCache,
		upgradeName:       *upgradeName,
		upgradeInfo:       upgradeInfo,
		postUpgradeParams: postUpgradeParams,
		validators:        validators,
	}

	group.Go(func() error {
		return utester.stateMachine(bus)
	})

	for name := range validators {
		func(nm string) {
			group.Go(func() error {
				return validators[nm].run()
			})
		}(name)
	}

	t.Logf("waiting for validator(s) to complete tasks")
	err = group.Wait()
	t.Logf("all validators finished")
	assert.NoError(t, err)

	fail := false

	for val, vl := range validators {
		for errs := range vl.testErrsCh {
			if len(errs) > 0 {
				for _, msg := range errs {
					t.Logf("[%s] %s", val, msg)
				}

				fail = true
			}
		}
	}

	bus.Close()

	if fail {
		t.Fail()
	}
}

func (l *upgradeTest) stateMachine(bus pubsub.Bus) error {
	var err error

	var sub pubsub.Subscriber

	sub, err = bus.Subscribe()
	if err != nil {
		return err
	}

	nodesCount := len(l.validators)
	stageCount := nodesCount

loop:
	for {
		select {
		case <-l.ctx.Done():
			err = l.ctx.Err()
			break loop
		case ev := <-sub.Events():
			switch ev.(type) {
			case nodePreUpgradeReady:
				stageCount--

				if stageCount == 0 {
					stageCount = nodesCount
					l.t.Log("all nodes started signing blocks. submitting upgrade")
					l.group.Go(func() error {
						return l.submitUpgradeProposal()
					})
				}

			case nodePostUpgradeReady:
				stageCount--

				if stageCount == 0 {
					l.t.Log("all nodes performed upgrade")

					postUpgradeWorker := uttypes.GetPostUpgradeWorker(l.upgradeName)
					if postUpgradeWorker == nil {
						l.t.Log("no post upgrade handlers found. submitting shutdown")
						_ = bus.Publish(postUpgradeTestDone{})

						break
					}

					l.t.Log("running post upgrade test handler")

					l.group.Go(func() error {
						defer func() {
							_ = bus.Publish(postUpgradeTestDone{})
						}()

						result := l.t.Run(l.upgradeName, func(t *testing.T) {
							postUpgradeWorker.Run(l.ctx, l.t, l.postUpgradeParams)
						})

						if !result {
							l.t.Error("post upgrade test FAIL")
							return fmt.Errorf("post-upgrade check failed")
						} else {
							l.t.Log("post upgrade test PASS")
						}

						return nil
					})
				}
			case postUpgradeTestDone:
				l.t.Log("shutting down validator(s)")
				for _, val := range l.validators {
					_ = val.pubsub.Publish(eventShutdown{})
				}

				break loop
			}
		}
	}

	return err
}

type baseAccount struct {
	Address string `json:"address"`
}

type moduleAccount struct {
	BaseAccount baseAccount `json:"base_account"`
}

type accountResp struct {
	Account moduleAccount `json:"account"`
}

func (l *upgradeTest) submitUpgradeProposal() error {
	var err error

	defer func() {
		if err != nil {
			l.t.Logf("submitUpgradeProposal finished with error: %s", err.Error())
		}
	}()

	var statusResp nodeStatus

	var cmdRes []byte

	for {
		cmdRes, err = l.cmdr.execute(l.ctx, "status")
		if err != nil {
			l.t.Logf("node status: %s\n", string(cmdRes))
			return err
		}

		err = json.Unmarshal(cmdRes, &statusResp)
		if err != nil {
			return err
		}

		if !statusResp.SyncInfo.CatchingUp {
			break
		}
	}

	cmdRes, err = l.cmdr.execute(l.ctx, "query auth module-account gov")
	if err != nil {
		l.t.Logf("executing cmd failed: %s\n", string(cmdRes))
		return err
	}

	macc := accountResp{}
	err = json.Unmarshal(cmdRes, &macc)
	if err != nil {
		return err
	}

	//tm := time.NewTimer(30 * time.Second)
	//select {
	//case <-l.ctx.Done():
	//	if !tm.Stop() {
	//		<-tm.C
	//	}
	//	err = l.ctx.Err()
	//	return err
	//case <-tm.C:
	//}

	cmdRes, err = l.cmdr.execute(l.ctx, "query gov params")
	if err != nil {
		l.t.Logf("executing cmd failed: %s\n", string(cmdRes))
		return err
	}

	params := govParams{}

	err = json.Unmarshal(cmdRes, &params)
	if err != nil {
		return err
	}

	votePeriod, err := time.ParseDuration(params.VotingParams.VotingPeriod)
	if err != nil {
		return fmt.Errorf("invalid vote period value (%s)", params.VotingParams.VotingPeriod)
	}

	cmdRes, err = l.cmdr.execute(l.ctx, "status")
	if err != nil {
		l.t.Logf("executing cmd failed: %s\n", string(cmdRes))
		return err
	}

	err = json.Unmarshal(cmdRes, &statusResp)
	if err != nil {
		return err
	}

	upgradeHeight, err := strconv.ParseInt(statusResp.SyncInfo.LatestBlockHeight, 10, 64)
	if err != nil {
		return err
	}

	upgradeHeight += int64(votePeriod/(6*time.Second)) + 10

	upgradeProp := SoftwareUpgradeProposal{
		Type:      "/cosmos.upgrade.v1beta1.MsgSoftwareUpgrade",
		Authority: macc.Account.BaseAccount.Address,
		Plan: upgradetypes.Plan{
			Name:   l.upgradeName,
			Height: upgradeHeight,
			Info:   l.upgradeInfo,
		},
	}

	jup, err := json.Marshal(&upgradeProp)
	if err != nil {
		return err
	}

	prop := &ProposalMsg{
		Messages: []json.RawMessage{
			jup,
		},
		Deposit:   params.DepositParams.MinDeposit[0].String(),
		Title:     l.upgradeName,
		Summary:   l.upgradeName,
		Expedited: false,
	}

	jProp, err := json.Marshal(prop)
	if err != nil {
		return err
	}

	propFile := fmt.Sprintf("%s/upgrade-prop-%s.json", l.cacheDir, l.upgradeName)
	err = os.WriteFile(propFile, jProp, 0644)
	if err != nil {
		return err
	}

	l.t.Logf("voting period: %s, curr height: %s, upgrade height: %d",
		votePeriod,
		statusResp.SyncInfo.LatestBlockHeight,
		upgradeHeight)

	cmd := fmt.Sprintf(`tx gov submit-proposal %s`, propFile)

	cmdRes, err = l.cmdr.execute(l.ctx, cmd)
	if err != nil {
		l.t.Logf("executing cmd failed: %s\n", string(cmdRes))
		return err
	}

	// give it two blocks to make sure a proposal has been commited
	tmctx, cancel := context.WithTimeout(l.ctx, 18*time.Second)
	defer cancel()

	<-tmctx.Done()

	if !errors.Is(tmctx.Err(), context.DeadlineExceeded) {
		l.t.Logf("error waiting for deadline\n")
		return tmctx.Err()
	}

	cmdRes, err = l.cmdr.execute(l.ctx, "query gov proposals --status=voting_period")
	if err != nil {
		l.t.Logf("executing cmd failed: %s\n", string(cmdRes))
		return err
	}

	var proposals proposalsResp

	err = json.Unmarshal(cmdRes, &proposals)
	if err != nil {
		return err
	}

	var propID string
	for i := len(proposals.Proposals) - 1; i >= 0; i-- {
		if proposals.Proposals[i].Title == l.upgradeName {
			propID = proposals.Proposals[i].ID
			break
		}
	}

	if propID == "" {
		return fmt.Errorf(`unable to find proposal with title "%s"`, l.upgradeName)
	}

	cmd = fmt.Sprintf(`tx gov vote %s yes`, propID)
	cmdRes, err = l.cmdr.execute(l.ctx, cmd)
	if err != nil {
		l.t.Logf("executing cmd failed: %s\n", string(cmdRes))
		return err
	}

	return nil
}

func newValidator(ctx context.Context, t *testing.T, params validatorParams, tConfig testCase) *validator {
	ctx, cancel := context.WithCancel(ctx)
	group, ctx := errgroup.WithContext(ctx)

	return &validator{
		t:                 t,
		ctx:               ctx,
		cancel:            cancel,
		pubsub:            pubsub.NewBus(),
		group:             group,
		params:            params,
		tConfig:           tConfig,
		upgradeSuccessful: make(chan struct{}, 1),
		testErrsCh:        make(chan []string, 1),
	}
}

func isOwnerExecutable(mode os.FileMode) bool {
	return mode&0100 != 0
}

func executeCommand(ctx context.Context, env []string, cmd string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	c.Env = env

	return c.CombinedOutput()
}

func (l *validator) run() error {
	lStdout, err := os.Create(fmt.Sprintf("%s/logs/%s-stdout.log", l.params.home, l.params.name))
	if err != nil {
		return err
	}

	defer func() {
		_ = lStdout.Close()
	}()

	lStderr, err := os.Create(fmt.Sprintf("%s/logs/%s-stderr.log", l.params.home, l.params.name))
	if err != nil {
		return err
	}

	defer func() {
		_ = lStderr.Close()
	}()

	rStdout, wStdout := io.Pipe()
	defer func() {
		_ = wStdout.Close()
	}()

	cmd := exec.CommandContext(l.ctx, l.params.cosmovisor, "run", "start", fmt.Sprintf("--home=%s", l.params.homedir))

	cmd.Stdout = io.MultiWriter(wStdout, lStdout)
	cmd.Stderr = io.MultiWriter(lStderr)

	cmd.Env = l.params.env

	err = cmd.Start()
	if err != nil {
		return err
	}

	l.group.Go(func() error {
		defer l.t.Logf("[%s] log scanner finished", l.params.name)
		l.t.Logf("[%s] log scanner started", l.params.name)

		var err error
		defer func() {
			if r := recover(); r != nil {
				l.t.Logf("%s", string(debug.Stack()))
				l.t.Fatal(r)
			}

			if err != nil {
				l.t.Logf("%s", err.Error())
			}
		}()

		err = l.scanner(rStdout, l.pubsub)
		return err
	})

	l.group.Go(func() error {
		defer l.t.Logf("[%s] test case watcher finished", l.params.name)
		l.t.Logf("[%s] test case watcher started", l.params.name)

		defer func() {
			if r := recover(); r != nil {
				l.t.Fatal(r)
			}
		}()

		sub, err := l.pubsub.Subscribe()
		if err != nil {
			return err
		}

		return l.watchTestCases(sub)
	})

	l.group.Go(func() error {
		defer l.t.Logf("[%s] stdout reader finished", l.params.name)
		l.t.Logf("[%s] stdout reader started", l.params.name)

		defer func() {
			if r := recover(); r != nil {
				l.t.Fatal(r)
			}
		}()

		<-l.ctx.Done()
		_ = rStdout.Close()
		l.pubsub.Close()

		return l.ctx.Err()
	})

	l.group.Go(func() error {
		defer l.t.Logf("[%s] blocks watchdog finished", l.params.name)
		l.t.Logf("[%s] blocks watchdog started", l.params.name)

		defer func() {
			if r := recover(); r != nil {
				l.t.Fatal(r)
			}
		}()

		sub, err := l.pubsub.Subscribe()
		if err != nil {
			return err
		}

		defer sub.Close()

		return l.blocksWatchdog(l.ctx, sub)
	})

	// state machine
	l.group.Go(func() error {
		defer l.t.Logf("[%s] state machine finished", l.params.name)
		l.t.Logf("[%s] state machine started", l.params.name)

		defer func() {
			if r := recover(); r != nil {
				l.t.Fatal(r)
			}
		}()

		return l.stateMachine()
	})

	err = cmd.Wait()
	l.t.Logf("[%s] cosmovisor stopped", l.params.name)
	l.cancel()

	l.t.Logf("[%s] waiting for workers to finish", l.params.name)
	_ = l.group.Wait()

	select {
	case <-l.upgradeSuccessful:
		err = nil
		l.t.Logf("[%s] all workers finished", l.params.name)
	default:
		l.t.Logf("[%s] cosmovisor finished with error. check %s", l.params.name, lStderr.Name())
	}

	return err
}

func (l *validator) stateMachine() error {
	defer l.cancel()

	var err error

	var sub pubsub.Subscriber

	sub, err = l.pubsub.Subscribe()
	if err != nil {
		return err
	}

	blocksCount := 0
	replayDone := false
	stage := nodeTestStagePreUpgrade

	wdCtrl := func(ctx context.Context, ctrl watchdogCtrl) {
		resp := make(chan struct{}, 1)
		_ = l.pubsub.Publish(wdReq{
			event: ctrl,
			resp:  resp,
		})

		select {
		case <-ctx.Done():
		case <-resp:
		}
	}

loop:
	for {
		select {
		case <-l.ctx.Done():
			err = l.ctx.Err()
			break loop
		case ev := <-sub.Events():
			switch evt := ev.(type) {
			case event:
				switch evt.id {
				case nodeEventStart:
					l.t.Logf("[%s][%s]: node started", l.params.name, nodeTestStageMapStr[stage])
					if stage == nodeTestStageUpgrade {
						stage = nodeTestStagePostUpgrade1
						blocksCount = 0
						replayDone = false
					}
				case nodeEventReplayBlocksStart:
					l.t.Logf("[%s][%s]: node started replaying blocks", l.params.name, nodeTestStageMapStr[stage])
				case nodeEventReplayBlocksDone:
					l.t.Logf("[%s][%s]: node done replaying blocks", l.params.name, nodeTestStageMapStr[stage])
					wdCtrl(l.ctx, watchdogCtrlStart)
					replayDone = true
				case nodeEventBlockCommited:
					// ignore index events until replay done
					if !replayDone {
						break
					}

					wdCtrl(l.ctx, watchdogCtrlBlock)

					blocksCount++
					if blocksCount == 1 {
						l.t.Logf("[%s][%s]: node started producing blocks", l.params.name, nodeTestStageMapStr[stage])
					}

					if stage == nodeTestStagePreUpgrade && blocksCount == 1 {
						_ = l.params.bus.Publish(nodePreUpgradeReady{
							name: l.params.name,
						})
					} else if stage == nodeTestStagePostUpgrade1 && blocksCount >= 10 {
						stage = nodeTestStagePostUpgrade2
						l.t.Logf("[%s][%s]: counted 10 blocks. signaling has performed upgrade", l.params.name, nodeTestStageMapStr[stage])
						l.upgradeSuccessful <- struct{}{}

						_ = l.params.bus.Publish(nodePostUpgradeReady{
							name: l.params.name,
						})
					}
				case nodeEventUpgradeDetected:
					l.t.Logf("[%s][%s]: node detected upgrade", l.params.name, nodeTestStageMapStr[stage])
					stage = nodeTestStageUpgrade
					wdCtrl(l.ctx, watchdogCtrlPause)
				default:
				}
			case eventShutdown:
				l.t.Logf("[%s][%s]: received shutdown signal", l.params.name, nodeTestStageMapStr[stage])
				wdCtrl(l.ctx, watchdogCtrlStop)
				break loop
			default:
			}
		}
	}

	if err == nil {
		err = context.Canceled
	}

	return err
}

func (l *validator) watchTestCases(subs pubsub.Subscriber) error {
	defer func() {
		close(l.testErrsCh)
	}()

	added := make(map[string]testModuleStatus)
	removed := make(map[string]testModuleStatus)
	migrations := make(map[string]*moduleMigrationStatus)

	for _, name := range l.tConfig.Modules.Added {
		added[name] = testModuleStatusNotChecked
	}

	for _, name := range l.tConfig.Modules.Removed {
		removed[name] = testModuleStatusNotChecked
	}

	for name, vals := range l.tConfig.Migrations {
		migrations[name] = &moduleMigrationStatus{
			status:   testModuleStatusNotChecked,
			expected: vals,
		}
	}

loop:
	for {
		select {
		case <-l.ctx.Done():
			break loop
		case ev := <-subs.Events():
			switch evt := ev.(type) {
			case event:
				switch evt.id {
				case nodeEventAddedModule:
					ctx := evt.ctx.(eventCtxModule)
					if _, exists := added[ctx.name]; !exists {
						added[ctx.name] = testModuleStatusUnexpected
					} else {
						added[ctx.name] = testModuleStatusChecked
					}
				case nodeEventRemovedModule:
					ctx := evt.ctx.(eventCtxModule)
					if _, exists := removed[ctx.name]; !exists {
						removed[ctx.name] = testModuleStatusUnexpected
					} else {
						removed[ctx.name] = testModuleStatusChecked
					}
				case nodeEventModuleMigration:
					ctx := evt.ctx.(eventCtxModuleMigration)
					if _, exists := migrations[ctx.name]; !exists {
						migrations[ctx.name] = &moduleMigrationStatus{status: testModuleStatusUnexpected}
					} else {
						m := migrations[ctx.name]

						m.status = testModuleStatusChecked
						m.actual = append(m.actual, testMigration{
							From: ctx.from,
							To:   ctx.to,
						})
					}
				default:
				}
			}
		}
	}

	errs := make([]string, 0)

	for name, status := range added {
		if status != testModuleStatusChecked {
			merr := fmt.Sprintf("module to add (%s) was not checked. status %s", name, testModuleStatusMapStr[status])
			errs = append(errs, merr)
		}
	}

	for name, status := range removed {
		if status != testModuleStatusChecked {
			merr := fmt.Sprintf("module to remove (%s) was not checked. status %s", name, testModuleStatusMapStr[status])
			errs = append(errs, merr)
		}
	}

	for name, module := range migrations {
		switch module.status {
		case testModuleStatusChecked:
			if !module.expected.compare(module.actual) {
				merr := fmt.Sprintf("migration for module (%s) finished with mismatched versions:\n", name)
				merr += "\texpected:\n"

				for _, m := range module.expected {
					merr += fmt.Sprintf(
						"\t\t- from: %s\n"+
							"\t\t  to:   %s\n", m.From, m.To)
				}

				merr += "\tactual:\n"

				for _, m := range module.actual {
					merr += fmt.Sprintf(
						"\t\t- from: %s\n"+
							"\t\t  to:   %s\n", m.From, m.To)
				}

				errs = append(errs, merr)
			}
		case testModuleStatusNotChecked:
			merr := fmt.Sprintf("required migration for module module (%s) was not detected", name)
			errs = append(errs, merr)
		case testModuleStatusUnexpected:
			merr := fmt.Sprintf("detected unexpected migration in module (%s)", name)
			errs = append(errs, merr)
		}
	}

	l.testErrsCh <- errs

	return nil
}

func (l *validator) blocksWatchdog(ctx context.Context, sub pubsub.Subscriber) error {
	var err error

	defer func() {
		if err != nil {
			l.t.Logf("[%s] %s", l.params.name, err.Error())
		} else {
			l.t.Logf("[%s] blocksWatchdog finished", l.params.name)
		}
	}()

	// first few blocks may take a while to produce.
	// give a watchdog a generous timeout on them

	blockWindow := 180 * time.Minute

	blocksTm := time.NewTicker(blockWindow)
	blocksTm.Stop()

	blocks := 0

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-blocksTm.C:
			err = fmt.Errorf("didn't receive block within specified time")
			break loop
		case evt := <-sub.Events():
			switch req := evt.(type) {
			case wdReq:
				req.resp <- struct{}{}

				switch req.event {
				case watchdogCtrlStart:
					fallthrough
				case watchdogCtrlBlock:
					blocks++

					if blocks > 3 {
						blockWindow = blockTimeWindow
					}

					blocksTm.Reset(blockWindow)
				case watchdogCtrlPause:
					blocksTm.Stop()
					blocks = 0
					blockWindow = 20 * time.Minute
				case watchdogCtrlStop:
					blocks = 0
					blocksTm.Stop()
					blockWindow = 20 * time.Minute
					break loop
				}
			}
		}
	}

	return err
}

func (l *validator) scanner(stdout io.Reader, p publisher) error {
	scanner := bufio.NewScanner(stdout)

	serverStart := "INF starting node with ABCI "
	replayBlocksStart := "INF ABCI Replay Blocks appHeight"
	replayBlocksDone := "INF service start impl=Evidence"
	executedBlock := "INF indexed block "
	executedBlock2 := "INF committed state block_app_hash="
	upgradeNeeded := fmt.Sprintf(`ERR UPGRADE "%s" NEEDED at height:`, l.params.upgradeName)
	addingNewModule := "INF adding a new module: "
	migratingModule := "INF migrating module "

	rServerStart, err := regexp.Compile(`^` + serverStart + `(Tendermint|CometBFT) in-process`)
	if err != nil {
		return err
	}

	rNewModule, err := regexp.Compile(`^` + addingNewModule + `(.+) (.+)$`)
	if err != nil {
		return err
	}

	rModuleMigration, err := regexp.Compile(`^` + migratingModule + `(\w+) from version (\d+) to version (\d+).*`)
	if err != nil {
		return err
	}

scan:
	for scanner.Scan() {
		line := scanner.Text()

		evt := event{}

		if strings.Contains(line, upgradeNeeded) {
			evt.id = nodeEventUpgradeDetected
		} else if strings.Contains(line, serverStart) {
			res := rServerStart.FindAllStringSubmatch(line, -1)
			if len(res) != 1 || len(res[0]) != 2 {
				return fmt.Errorf("line \"%s\" does not match regex \"%s\"", line, rServerStart.String())
			}
			evt.id = nodeEventStart
		} else if strings.Contains(line, replayBlocksStart) {
			evt.id = nodeEventReplayBlocksStart
		} else if strings.Contains(line, replayBlocksDone) {
			evt.id = nodeEventReplayBlocksDone
		} else if strings.Contains(line, executedBlock) || strings.Contains(line, executedBlock2) {
			evt.id = nodeEventBlockCommited
		} else if strings.Contains(line, addingNewModule) {
			evt.id = nodeEventAddedModule
			res := rNewModule.FindAllStringSubmatch(line, -1)
			if len(res) != 1 || len(res[0]) != 3 {
				return fmt.Errorf("line \"%s\" does not match regex \"%s\"", line, rNewModule.String())
			}
			evt.ctx = eventCtxModule{
				name: res[0][1],
			}
		} else if strings.Contains(line, migratingModule) {
			evt.id = nodeEventModuleMigration
			res := rModuleMigration.FindAllStringSubmatch(line, -1)
			if len(res) != 1 || len(res[0]) != 4 {
				return fmt.Errorf("line \"%s\" does not match regex \"%s\"", line, rModuleMigration.String())
			}

			evt.ctx = eventCtxModuleMigration{
				name: res[0][1],
				from: res[0][2],
				to:   res[0][3],
			}
		} else {
			continue scan
		}

		if err = p.Publish(evt); err != nil {
			return err
		}
	}

	return nil
}

type moduleMigrationStatus struct {
	status   testModuleStatus
	expected moduleMigrationVersions
	actual   moduleMigrationVersions
}

func (v moduleMigrationVersions) compare(to moduleMigrationVersions) bool {
	if len(v) != len(to) {
		return false
	}

	for i := range v {
		if (v[i].From != to[i].From) || (v[i].To != to[i].To) {
			return false
		}
	}

	return true
}
