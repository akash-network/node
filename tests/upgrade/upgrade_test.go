//go:build e2e.upgrade

package upgrade

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-github/v52/github"
	"github.com/gregjones/httpcache"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"

	sdk "github.com/cosmos/cosmos-sdk/types"

	// init sdk config
	_ "github.com/akash-network/akash-api/go/sdkutil"

	"github.com/akash-network/node/pubsub"
)

const (
	blockTimeWindow = 7 * time.Second
)

type nodeEvent int
type watchdogCtrl int
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
	testStageMapStr = map[testStage]string{
		testStagePreUpgrade:  "preupgrade",
		testStageUpgrade:     "upgrade",
		testStagePostUpgrade: "postupgrade",
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

type launcherParams struct {
	home          string
	homeDir       string
	chainID       string
	upgradeName   string
	upgradeHeight int64
}
type launcher struct {
	t                 *testing.T
	ctx               context.Context
	cancel            context.CancelFunc
	group             *errgroup.Group
	cosmovisor        string
	upgradeInfo       string
	params            launcherParams
	tConfig           testCases
	upgradeSuccessful chan struct{}
	testErrs          []string
}

type upgradeInfo struct {
	Binaries map[string]string `json:"binaries"`
}

var (
	homedir        = flag.String("home", "", "akash home")
	cosmovisor     = flag.String("cosmovisor", "", "path to cosmovisor")
	genesisBinary  = flag.String("genesis-binary", "", "path to the akash binary with version prior the upgrade")
	upgradeVersion = flag.String("upgrade-version", "local", "akash release to download. local if it is built locally")
	upgradeName    = flag.String("upgrade-name", "", "name of the upgrade")
	chainID        = flag.String("chain-id", "", "chain-id")
	testCasesFile  = flag.String("test-cases", "", "")
)

func TestUpgrade(t *testing.T) {
	ctx := context.Background()

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM)
	defer stop()

	t.Log("detecting arguments")

	require.NotEqual(t, "", *homedir, "empty homedir flag")
	require.NotEqual(t, "", *cosmovisor, "empty cosmovisor flag")
	require.NotEqual(t, "", *genesisBinary, "empty genesis-binary flag")
	require.NotEqual(t, "", *upgradeName, "empty upgrade-name flag")
	require.NotEqual(t, "", *upgradeVersion, "empty upgrade-version flag")
	require.NotEqual(t, "", *chainID, "empty chain-id flag")
	require.NotEqual(t, "", *testCasesFile, "empty test-cases flag")

	if *upgradeVersion != "local" && !semver.IsValid(*upgradeVersion) {
		require.Fail(t, "upgrade-name contains invalid value. expected local|<semver>")
	}

	info, err := os.Stat(*homedir)
	require.NoError(t, err)
	require.True(t, info.IsDir(), "homedir flag is not a dir")

	info, err = os.Stat(*cosmovisor)
	require.NoError(t, err)
	require.False(t, info.IsDir(), "value in cosmovisor flag is not a file")
	require.True(t, isOwnerExecutable(info.Mode()), "cosmovisor must be executable file")

	info, err = os.Stat(*genesisBinary)
	require.NoError(t, err)
	require.False(t, info.IsDir(), "value in genesis-binary flag is not a file")
	require.True(t, isOwnerExecutable(info.Mode()), "akash must be executable file")

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

	l := newLauncher(ctx, t)

	if *upgradeVersion != "local" {
		t.Logf("generating upgradeinfo from release %s", *upgradeVersion)
		l.upgradeInfo, err = generateUpgradeInfo(ctx, *upgradeVersion)
		require.NoError(t, err)
		require.NotEqual(t, "", l.upgradeInfo)
	}

	l.cosmovisor = *cosmovisor
	l.tConfig = tConfig
	l.params = launcherParams{
		home:          *homedir,
		homeDir:       fmt.Sprintf("%s/.akash", *homedir),
		chainID:       *chainID,
		upgradeName:   *upgradeName,
		upgradeHeight: 0,
	}

	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error {
		t.Log("starting cosmovisor")
		return l.run()
	})

	err = group.Wait()
	require.NoError(t, err)

	if len(l.testErrs) > 0 {
		for _, msg := range l.testErrs {
			t.Log(msg)
		}

		t.Fail()
	}
}

func newLauncher(ctx context.Context, t *testing.T) *launcher {
	ctx, cancel := context.WithCancel(ctx)
	group, ctx := errgroup.WithContext(ctx)
	return &launcher{
		t:                 t,
		ctx:               ctx,
		cancel:            cancel,
		group:             group,
		upgradeSuccessful: make(chan struct{}, 1),
	}
}

func isOwnerExecutable(mode os.FileMode) bool {
	return mode&0100 != 0
}

func generateUpgradeInfo(ctx context.Context, tag string) (string, error) {
	tc := &http.Client{
		Transport: &oauth2.Transport{
			Base: httpcache.NewMemoryCacheTransport(),
			Source: oauth2.StaticTokenSource(
				&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
			),
		},
	}

	gh := github.NewClient(tc)

	rel, resp, err := gh.Repositories.GetReleaseByTag(ctx, "akash-network", "node", tag)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("no release for tag %s", tag)
	}

	sTag := strings.TrimPrefix(tag, "v")
	checksumsAsset := fmt.Sprintf("akash_%s_checksums.txt", sTag)
	var checksumsID int64
	for _, asset := range rel.Assets {
		if asset.GetName() == checksumsAsset {
			checksumsID = asset.GetID()
		}
	}

	body, _, err := gh.Repositories.DownloadReleaseAsset(ctx, "akash-network", "node", checksumsID, http.DefaultClient)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = body.Close()
	}()

	info := &upgradeInfo{
		Binaries: make(map[string]string),
	}

	urlBase := fmt.Sprintf("https://github.com/akash-network/node/releases/download/%s", tag)
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		tuple := strings.Split(scanner.Text(), "  ")
		if len(tuple) != 2 {
			return "", fmt.Errorf("invalid checksum format")
		}

		switch tuple[1] {
		case "akash_linux_amd64.zip":
			info.Binaries["linux/amd64"] = fmt.Sprintf("%s/%s?checksum=sha256:%s", urlBase, tuple[1], tuple[0])
		case "akash_linux_arm64.zip":
			info.Binaries["linux/arm64"] = fmt.Sprintf("%s/%s?checksum=sha256:%s", urlBase, tuple[1], tuple[0])
		case "akash_darwin_all.zip":
			info.Binaries["darwin/amd64"] = fmt.Sprintf("%s/%s?checksum=sha256:%s", urlBase, tuple[1], tuple[0])
			info.Binaries["darwin/arm64"] = fmt.Sprintf("%s/%s?checksum=sha256:%s", urlBase, tuple[1], tuple[0])
		}
	}

	res, err := json.Marshal(info)
	if err != nil {
		return "", err
	}

	return string(res), nil
}

func executeCommand(ctx context.Context, env []string, cmd string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	c.Env = env

	return c.CombinedOutput()
}

func (l *launcher) submitUpgradeProposal() error {
	var err error

	defer func() {
		if err != nil {
			l.t.Logf("submitUpgradeProposal finished with error: %s", err.Error())
		}
	}()

	env := []string{
		fmt.Sprintf("HOME=%s", *homedir),
		fmt.Sprintf("AKASH_HOME=%s", l.params.homeDir),
		fmt.Sprintf("AKASH_KEYRING_BACKEND=test"),
		fmt.Sprintf("AKASH_BROADCAST_MODE=block"),
		fmt.Sprintf("AKASH_CHAIN_ID=localakash"),
		fmt.Sprintf("AKASH_FROM=validator"),
		fmt.Sprintf("AKASH_GAS=auto"),
		fmt.Sprintf("AKASH_YES=true"),
	}

	cmd := fmt.Sprintf(`%s status`, *genesisBinary)

	var statusResp nodeStatus

	var cmdRes []byte

	for {
		cmdRes, err = executeCommand(l.ctx, env, "bash", "-c", cmd)
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

	cmd = fmt.Sprintf("%s query gov params --output=json", *genesisBinary)
	l.t.Logf("executing cmd: %s\n", cmd)
	cmdRes, err = executeCommand(l.ctx, env, "bash", "-c", cmd)
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

	cmdRes, err = executeCommand(l.ctx, env, "bash", "-c", cmd)
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

	cmd = fmt.Sprintf(`%s tx gov submit-proposal software-upgrade %s --title=%[2]s --description="%[2]s" --upgrade-height=%d --deposit=%s`,
		*genesisBinary,
		l.params.upgradeName,
		upgradeHeight,
		params.DepositParams.MinDeposit[0].String(),
	)

	if l.upgradeInfo != "" {
		cmd += fmt.Sprintf(` --upgrade-info='%s'`, l.upgradeInfo)
	}

	l.t.Logf("executing cmd: %s\n", cmd)
	cmdRes, err = executeCommand(l.ctx, env, "bash", "-c", cmd)
	if err != nil {
		l.t.Logf("executing cmd failed: %s\n", string(cmdRes))
		return err
	}

	cmd = fmt.Sprintf(`%s query gov proposals --output=json`, *genesisBinary)
	l.t.Logf("executing cmd: %s\n", cmd)
	cmdRes, err = executeCommand(l.ctx, env, "bash", "-c", cmd)
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
		if proposals.Proposals[i].Content.Title == l.params.upgradeName {
			propID = proposals.Proposals[i].ProposalID
			break
		}
	}

	if propID == "" {
		return fmt.Errorf(`unable to find proposal with title "%s"`, l.params.upgradeName)
	}

	cmd = fmt.Sprintf(`%s tx gov vote %s yes`, *genesisBinary, propID)
	l.t.Logf("executing cmd: %s\n", cmd)
	cmdRes, err = executeCommand(l.ctx, env, "bash", "-c", cmd)
	if err != nil {
		l.t.Logf("executing cmd failed: %s\n", string(cmdRes))
		return err
	}

	return nil
}

func (l *launcher) run() error {
	lStdout, err := os.Create(fmt.Sprintf("%s/stdout.log", l.params.home))
	if err != nil {
		return err
	}

	defer func() {
		_ = lStdout.Close()
	}()

	lStderr, err := os.Create(fmt.Sprintf("%s/stderr.log", l.params.home))
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

	cmd := exec.CommandContext(l.ctx, l.cosmovisor, "run", "start", fmt.Sprintf("--home=%s", l.params.homeDir))

	cmd.Stdout = io.MultiWriter(lStdout, wStdout)
	cmd.Stderr = io.MultiWriter(lStderr)

	cmd.Env = []string{
		fmt.Sprintf("HOME=%s", l.params.home),
		fmt.Sprintf("DAEMON_NAME=akash"),
		fmt.Sprintf("DAEMON_HOME=%s", l.params.homeDir),
		fmt.Sprintf("DAEMON_RESTART_AFTER_UPGRADE=true"),
		fmt.Sprintf("DAEMON_ALLOW_DOWNLOAD_BINARIES=true"),
		fmt.Sprintf("UNSAFE_SKIP_BACKUP=true"),
		fmt.Sprintf("AKASH_HOME=%s", l.params.homeDir),
		fmt.Sprintf("AKASH_KEYRING_BACKEND=test"),
		fmt.Sprintf("AKASH_FAST_SYNC=false"),
		fmt.Sprintf("AKASH_P2P_PEX=false"),
		fmt.Sprintf("AKASH_LOG_COLOR=false"),
		fmt.Sprintf("AKASH_LOG_TIMESTAMP="),
		fmt.Sprintf("AKASH_LOG_FORMAT=plain"),
		fmt.Sprintf("AKASH_STATESYNC_ENABLE=false"),
		fmt.Sprintf("AKASH_CHAIN_ID=%s", l.params.chainID),
		fmt.Sprintf("AKASH_TX_INDEX_INDEXER=kv"),
	}

	bus := pubsub.NewBus()

	err = cmd.Start()
	if err != nil {
		return err
	}

	l.group.Go(func() error {
		return l.scanner(rStdout, bus)
	})

	l.group.Go(func() error {
		sub, err := bus.Subscribe()
		if err != nil {
			return err
		}

		return l.watchTestCases(sub)
	})

	l.group.Go(func() error {
		<-l.ctx.Done()
		_ = rStdout.Close()
		bus.Close()
		return l.ctx.Err()
	})

	l.group.Go(func() error {
		sub, err := bus.Subscribe()
		if err != nil {
			return err
		}

		return l.blocksWatchdog(l.ctx, sub)
	})

	// state machine
	l.group.Go(func() error {
		return l.stateMachine(bus)
	})

	err = cmd.Wait()
	l.t.Log("cosmovisor stopped")
	l.cancel()

	l.t.Log("waiting for workers to finish")
	_ = l.group.Wait()

	select {
	case <-l.upgradeSuccessful:
		err = nil
	default:
		l.t.Log("cosmovisor finished with error. check stderr")
	}

	return err
}

func (l *launcher) stateMachine(bus pubsub.Bus) error {
	var err error

	var sub pubsub.Subscriber

	sub, err = bus.Subscribe()
	if err != nil {
		return err
	}

	blocksCount := 0
	replayDone := false
	stage := testStagePreUpgrade

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
					l.t.Logf("[%s]: node started", testStageMapStr[stage])
					if stage == testStageUpgrade {
						stage = testStagePostUpgrade
						blocksCount = 0
						replayDone = false
					}
				case nodeEventReplayBlocksStart:
					l.t.Logf("[%s]: node started replaying blocks", testStageMapStr[stage])
				case nodeEventReplayBlocksDone:
					l.t.Logf("[%s]: node done replaying blocks", testStageMapStr[stage])
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
						l.t.Logf("[%s]: node started producing blocks", testStageMapStr[stage])
					}

					if stage == testStagePreUpgrade && blocksCount == 1 {
						l.group.Go(func() error {
							return l.submitUpgradeProposal()
						})
					} else if stage == testStagePostUpgrade && blocksCount == 10 {
						l.t.Logf("[%s]: counted 10 blocks. signaling to finish the test", testStageMapStr[stage])
						l.upgradeSuccessful <- struct{}{}
						l.cancel()
					}
				case nodeEventUpgradeDetected:
					l.t.Logf("[%s]: node detected upgrade", testStageMapStr[stage])
					stage = testStageUpgrade
					wdCtrl(l.ctx, watchdogCtrlPause)
				}
			}
		}
	}

	return err
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

func (l *launcher) watchTestCases(subs pubsub.Subscriber) error {
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
		if module.status == testModuleStatusChecked {
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
		} else {
			merr := fmt.Sprintf("detected unexpected pmigration in module (%s)", name)
			errs = append(errs, merr)
		}
	}

	l.testErrs = errs

	return nil
}

func (l *launcher) blocksWatchdog(ctx context.Context, sub pubsub.Subscriber) error {
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

				req.resp <- struct{}{}
			}
		}
	}

	return err
}

func (l *launcher) scanner(stdout io.Reader, p publisher) error {
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
