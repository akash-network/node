//go:build e2e.upgrade

package upgrade

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"
	"golang.org/x/sync/errgroup"

	sdk "github.com/cosmos/cosmos-sdk/types"

	// init sdk config
	_ "github.com/akash-network/akash-api/go/sdkutil"

	"github.com/akash-network/node/pubsub"
	uttypes "github.com/akash-network/node/tests/upgrade/types"
	"github.com/akash-network/node/util/cli"
)

const (
	blockTimeWindow = 7 * time.Second
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
	nodeEventBlockIndexed
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
	nodeTestStagePostUpgrade
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
		nodeTestStagePreUpgrade:  "preupgrade",
		nodeTestStageUpgrade:     "upgrade",
		nodeTestStagePostUpgrade: "postupgrade",
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

type votingParams struct {
	VotingPeriod string `json:"voting_period"`
}

type depositParams struct {
	MinDeposit sdk.Coins `json:"min_deposit"`
}

type govParams struct {
	VotingParams  votingParams  `json:"voting_params"`
	DepositParams depositParams `json:"deposit_params"`
}

type proposalResp struct {
	ProposalID string `json:"proposal_id"`
	Content    struct {
		Title string `json:"title"`
	} `json:"content"`
}

type proposalsResp struct {
	Proposals []proposalResp `json:"proposals"`
}

type wdReq struct {
	event watchdogCtrl
	resp  chan<- struct{}
}

type nodeStatus struct {
	SyncInfo struct {
		LatestBlockHeight string `json:"latest_block_height"`
		CatchingUp        bool   `json:"catching_up"`
	} `json:"SyncInfo"`
}

type testCases struct {
	Modules struct {
		Added   []string `json:"added"`
		Removed []string `json:"removed"`
		Renamed struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"renamed"`
	} `json:"modules"`
	Migrations map[string]struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"migrations"`
}

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
	pub         pubsub.Publisher
}

type validator struct {
	t                 *testing.T
	pubsub            pubsub.Bus
	ctx               context.Context
	cancel            context.CancelFunc
	group             *errgroup.Group
	upgradeInfo       string
	params            validatorParams
	tConfig           testCases
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
	upgradeName       string
	upgradeInfo       string
	postUpgradeParams uttypes.TestParams
	validators        map[string]*validator
}

type nodeInitParams struct {
	nodeID    string
	homedir   string
	p2pPort   uint16
	rpcPort   uint16
	pprofPort uint16
}

var (
	workdir        = flag.String("workdir", "", "work directory")
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
	ctx := context.Background()

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM)
	defer stop()

	t.Log("detecting arguments")

	require.NotEqual(t, "", *workdir, "empty workdir flag")
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

	var tConfig testCases
	// load testcases config
	{
		tFile, err := os.Open(*testCasesFile)
		require.NoError(t, err)
		defer func() {
			_ = tFile.Close()
		}()

		data, err := io.ReadAll(tFile)
		require.NoError(t, err)

		err = json.Unmarshal(data, &tConfig)
		require.NoError(t, err)
	}

	cmdr := &commander{}

	validatorsParams := make(map[string]validatorParams)

	bus := pubsub.NewBus()

	initParams := make(map[string]nodeInitParams)

	postUpgradeParams := uttypes.TestParams{}

	for idx, name := range cfg.Validators {
		homedir := fmt.Sprintf("%s/%s", *workdir, name)

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
				// auto is failing with rpc error: code = Unknown desc = unknown query path: unknown request
				fmt.Sprintf("AKASH_GAS=500000"),
				fmt.Sprintf("AKASH_YES=true"),
			},
		}

		if cfg.Work.Home == name {
			postUpgradeParams.Home = homedir
			postUpgradeParams.ChainID = cfg.ChainID
			postUpgradeParams.Node = "tcp://127.0.0.1:26657"
			postUpgradeParams.KeyringBackend = "test"

			cmdr = valCmd

			_, err = cmdr.execute(ctx, fmt.Sprintf("keys show %s -a", cfg.Work.Key))
			require.NoError(t, err)
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
			nodeID:    strings.TrimSpace(string(res)),
			homedir:   homedir,
			p2pPort:   p2pPort,
			rpcPort:   p2pPort + 1,
			pprofPort: 6060 + uint16(idx),
		}
	}

	for name, params := range initParams {
		var unconditionalPeerIDs string
		var persistentPeers string

		for nm1, params1 := range initParams {
			if name == nm1 {
				continue
			}

			unconditionalPeerIDs += params1.nodeID + ","
			persistentPeers += fmt.Sprintf("%s@127.0.0.1:%d,", params1.nodeID, params1.p2pPort)
		}

		validatorsParams[name] = validatorParams{
			home:        *workdir,
			homedir:     params.homedir,
			name:        name,
			nodeID:      params.nodeID,
			cosmovisor:  *cosmovisor,
			isRPC:       cfg.Work.Home == name,
			p2pPort:     params.p2pPort,
			rpcPort:     params.rpcPort,
			upgradeName: *upgradeName,
			pub:         bus,
			env: []string{
				fmt.Sprintf("DAEMON_NAME=akash"),
				fmt.Sprintf("DAEMON_HOME=%s", params.homedir),
				fmt.Sprintf("DAEMON_RESTART_AFTER_UPGRADE=true"),
				fmt.Sprintf("DAEMON_ALLOW_DOWNLOAD_BINARIES=true"),
				fmt.Sprintf("DAEMON_RESTART_DELAY=3s"),
				fmt.Sprintf("COSMOVISOR_COLOR_LOGS=false"),
				fmt.Sprintf("UNSAFE_SKIP_BACKUP=true"),
				fmt.Sprintf("HOME=%s", *workdir),
				fmt.Sprintf("AKASH_HOME=%s", params.homedir),
				fmt.Sprintf("AKASH_CHAIN_ID=%s", cfg.ChainID),
				fmt.Sprintf("AKASH_KEYRING_BACKEND=test"),
				fmt.Sprintf("AKASH_P2P_SEEDS=%s", strings.TrimSuffix(persistentPeers, ",")),
				fmt.Sprintf("AKASH_P2P_PERSISTENT_PEERS=%s", strings.TrimSuffix(persistentPeers, ",")),
				fmt.Sprintf("AKASH_P2P_UNCONDITIONAL_PEER_IDS=%s", strings.TrimSuffix(unconditionalPeerIDs, ",")),
				fmt.Sprintf("AKASH_P2P_LADDR=tcp://127.0.0.1:%d", params.p2pPort),
				fmt.Sprintf("AKASH_RPC_LADDR=tcp://127.0.0.1:%d", params.rpcPort),
				fmt.Sprintf("AKASH_RPC_PPROF_LADDR=localhost:%d", params.pprofPort),
				"AKASH_P2P_PEX=true",
				"AKASH_P2P_ADDR_BOOK_STRICT=false",
				"AKASH_P2P_ALLOW_DUPLICATE_IP=true",
				"AKASH_MINIMUM_GAS_PRICES=0.0025uakt",
				"AKASH_FAST_SYNC=false",
				"AKASH_LOG_COLOR=false",
				"AKASH_LOG_TIMESTAMP=",
				"AKASH_LOG_FORMAT=plain",
				"AKASH_STATESYNC_ENABLE=false",
				"AKASH_TX_INDEX_INDEXER=null",
				"AKASH_GRPC_ENABLE=false",
				"AKASH_GRPC_WEB_ENABLE=false",
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

	err = group.Wait()
	assert.NoError(t, err)

	fail := false

	for val, vl := range validators {
		select {
		case errs := <-vl.testErrsCh:
			if len(errs) > 0 {
				for _, msg := range errs {
					t.Logf("[%s] %s", val, msg)
				}

				fail = true
			}

		case <-vl.ctx.Done():
		}
	}

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
							l.t.Error("post upgrade test handler failed")
							return fmt.Errorf("post-upgrade check failed")
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

	tm := time.NewTimer(30 * time.Second)
	select {
	case <-l.ctx.Done():
		if !tm.Stop() {
			<-tm.C
		}
		err = l.ctx.Err()
		return err
	case <-tm.C:
	}

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

	votePeriod, valid := sdk.NewIntFromString(params.VotingParams.VotingPeriod)
	if !valid {
		return fmt.Errorf("invalid vote period value (%s)", params.VotingParams.VotingPeriod)
	}

	votePeriod = votePeriod.QuoRaw(1e9)

	cmdRes, err = l.cmdr.execute(l.ctx, "status")
	if err != nil {
		l.t.Logf("executing cmd failed: %s\n", string(cmdRes))
		return err
	}

	err = json.Unmarshal(cmdRes, &statusResp)
	if err != nil {
		return err
	}

	upgradeHeight, err := strconv.ParseUint(statusResp.SyncInfo.LatestBlockHeight, 10, 64)
	if err != nil {
		return err
	}

	upgradeHeight += (votePeriod.Uint64() / 6) + 10

	l.t.Logf("voting period: %ss, curr height: %s, upgrade height: %d",
		votePeriod,
		statusResp.SyncInfo.LatestBlockHeight,
		upgradeHeight)

	cmd := fmt.Sprintf(`tx gov submit-proposal software-upgrade %s --title=%[1]s --description="%[1]s" --upgrade-height=%d --deposit=%s`,
		l.upgradeName,
		upgradeHeight,
		params.DepositParams.MinDeposit[0].String(),
	)

	if l.upgradeInfo != "" {
		cmd += fmt.Sprintf(` --upgrade-info='%s'`, l.upgradeInfo)
	}

	cmdRes, err = l.cmdr.execute(l.ctx, cmd)
	if err != nil {
		l.t.Logf("executing cmd failed: %s\n", string(cmdRes))
		return err
	}

	cmdRes, err = l.cmdr.execute(l.ctx, "query gov proposals")
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
		if proposals.Proposals[i].Content.Title == l.upgradeName {
			propID = proposals.Proposals[i].ProposalID
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

func newValidator(ctx context.Context, t *testing.T, params validatorParams, tConfig testCases) *validator {
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

	cmd.Stdout = io.MultiWriter(lStdout, wStdout)
	cmd.Stderr = io.MultiWriter(lStderr)

	cmd.Env = l.params.env

	err = cmd.Start()
	if err != nil {
		return err
	}

	l.group.Go(func() error {
		defer func() {
			if r := recover(); r != nil {
				l.t.Fatal(r)
			}
		}()

		return l.scanner(rStdout, l.pubsub)
	})

	l.group.Go(func() error {
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
		defer func() {
			if r := recover(); r != nil {
				l.t.Fatal(r)
			}
		}()

		sub, err := l.pubsub.Subscribe()
		if err != nil {
			return err
		}

		return l.blocksWatchdog(l.ctx, sub)
	})

	// state machine
	l.group.Go(func() error {
		defer func() {
			if r := recover(); r != nil {
				l.t.Fatal(r)
			}
		}()

		return l.stateMachine(l.pubsub)
	})

	err = cmd.Wait()
	l.t.Logf("[%s] cosmovisor stopped", l.params.name)
	l.cancel()

	l.t.Logf("[%s] waiting for workers to finish", l.params.name)
	_ = l.group.Wait()

	select {
	case <-l.upgradeSuccessful:
		err = nil
	default:
		l.t.Logf("[%s] cosmovisor finished with error. check %[1]s-stderr.log", l.params.name)
	}

	return err
}

func (l *validator) stateMachine(bus pubsub.Bus) error {
	defer l.cancel()

	var err error

	var sub pubsub.Subscriber

	sub, err = bus.Subscribe()
	if err != nil {
		return err
	}

	blocksCount := 0
	replayDone := false
	stage := nodeTestStagePreUpgrade

	wdCtrl := func(ctx context.Context, ctrl watchdogCtrl) {
		resp := make(chan struct{}, 1)
		_ = bus.Publish(wdReq{
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
						stage = nodeTestStagePostUpgrade
						blocksCount = 0
						replayDone = false
					}
				case nodeEventReplayBlocksStart:
					l.t.Logf("[%s][%s]: node started replaying blocks", l.params.name, nodeTestStageMapStr[stage])
				case nodeEventReplayBlocksDone:
					l.t.Logf("[%s][%s]: node done replaying blocks", l.params.name, nodeTestStageMapStr[stage])
					wdCtrl(l.ctx, watchdogCtrlStart)
					replayDone = true
				case nodeEventBlockIndexed:
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
						_ = l.params.pub.Publish(nodePreUpgradeReady{
							name: l.params.name,
						})
					} else if stage == nodeTestStagePostUpgrade && blocksCount == 10 {
						l.t.Logf("[%s][%s]: counted 10 blocks. signaling has performed upgrade", l.params.name, nodeTestStageMapStr[stage])
						l.upgradeSuccessful <- struct{}{}

						_ = l.params.pub.Publish(nodePostUpgradeReady{
							name: l.params.name,
						})
					}
				case nodeEventUpgradeDetected:
					l.t.Logf("[%s][%s]: node detected upgrade", l.params.name, nodeTestStageMapStr[stage])
					stage = nodeTestStageUpgrade
					wdCtrl(l.ctx, watchdogCtrlPause)
				}
			case eventShutdown:
				l.t.Logf("[%s][%s]: received shutdown signal", l.params.name, nodeTestStageMapStr[stage])
				wdCtrl(l.ctx, watchdogCtrlStop)
				break loop
			}
		}
	}

	if err == nil {
		err = context.Canceled
	}

	return err
}

func (l *validator) watchTestCases(subs pubsub.Subscriber) error {
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
			status: testModuleStatusNotChecked,
			expected: moduleMigrationVersions{
				from: vals.From,
				to:   vals.To,
			},
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
						m.actual.to = ctx.to
						m.actual.from = ctx.from
					}
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
				merr := fmt.Sprintf("migration for module (%s) finished with mismatched versions:\n"+
					"\texpected:\n"+
					"\t\tfrom: %s\n"+
					"\t\tto:   %s\n"+
					"\tactual:\n"+
					"\t\tfrom: %s\n"+
					"\t\tto:   %s",
					name,
					module.expected.from, module.expected.to,
					module.actual.from, module.actual.to)

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
			l.t.Logf("blocksWatchdog finished with error: %s", err.Error())
		}
	}()

loop:
	for {
		blocksTm := time.NewTicker(blockTimeWindow)
		blocksTm.Stop()

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
					blocksTm.Reset(blockTimeWindow)
				case watchdogCtrlPause:
					blocksTm.Stop()
				case watchdogCtrlStop:
					blocksTm.Stop()
					break loop
				}
			}
		}
	}

	return err
}

func (l *validator) scanner(stdout io.Reader, p publisher) error {
	scanner := bufio.NewScanner(stdout)

	serverStart := "INF starting node with ABCI Tendermint in-process"
	replayBlocksStart := "INF ABCI Replay Blocks appHeight"
	replayBlocksDone := "INF Replay: Done module=consensus"
	executedBlock := "INF indexed block "
	upgradeNeeded := fmt.Sprintf(`ERR UPGRADE "%s" NEEDED at height:`, l.params.upgradeName)
	addingNewModule := "INF adding a new module: "
	migratingModule := "INF migrating module "

	rNewModule, err := regexp.Compile(`^` + addingNewModule + `(\w+)$`)
	if err != nil {
		return err
	}

	rModuleMigration, err := regexp.Compile(`^` + migratingModule + `(\w+) from version (\d+) to version (\d+)$`)
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
			evt.id = nodeEventStart
		} else if strings.Contains(line, replayBlocksStart) {
			evt.id = nodeEventReplayBlocksStart
		} else if strings.Contains(line, replayBlocksDone) {
			evt.id = nodeEventReplayBlocksDone
		} else if strings.Contains(line, executedBlock) {
			evt.id = nodeEventBlockIndexed
		} else if strings.Contains(line, addingNewModule) {
			evt.id = nodeEventAddedModule
			res := rNewModule.FindAllStringSubmatch(line, -1)
			evt.ctx = eventCtxModule{
				name: res[0][1],
			}
		} else if strings.Contains(line, migratingModule) {
			evt.id = nodeEventModuleMigration
			res := rModuleMigration.FindAllStringSubmatch(line, -1)
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

type moduleMigrationVersions struct {
	from string
	to   string
}

type moduleMigrationStatus struct {
	status   testModuleStatus
	expected moduleMigrationVersions
	actual   moduleMigrationVersions
}

func (v moduleMigrationVersions) compare(to moduleMigrationVersions) bool {
	return (v.from == to.from) && (v.to == v.to)
}
