package relayer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/pflag"

	"pkg.akt.dev/node/v2/internal/relayer/header"
)

type command struct {
	name       string
	network    string
	configPath string
	keyPath    string
	watch      bool
	reset      string
	tamper     bool
	localArgs  []string
	fixture    *fixtureArgs
}

type fixtureArgs struct {
	validators int
	signers    int
	weighted   bool
}

func parseCommand(args []string) (command, error) {
	if len(args) > 0 && args[0] == "header-encode" {
		return parseHeaderCommand(args[1:])
	}
	if len(args) > 0 && args[0] == "local" {
		return parseLocalCommand(args[1:])
	}
	flags := pflag.NewFlagSet("akt-relayer", pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	help := flags.Bool("help", false, "show help")
	version := flags.Bool("version", false, "show build information")
	network := flags.String("network", "mainnet", "mainnet or testnet")
	path := flags.String("config", "", "absolute configuration path")
	out := flags.String("out", "", "new signing key path")
	key := flags.String("key-file", "", "signing key path")
	broadcast := flags.Bool("broadcast", false, "reporter-only option")
	watch := flags.Bool("watch", false, "run continuously")
	reset := flags.String("reset-budget", "", "reset one transfer or all")
	if err := flags.Parse(args); err != nil {
		return command{}, err
	}
	if *network != "mainnet" && *network != "testnet" {
		return command{}, fmt.Errorf("network must be mainnet or testnet")
	}
	if *help || len(args) == 0 {
		return command{name: "help"}, nil
	}
	if *version {
		return command{name: "version"}, nil
	}
	positionals := flags.Args()
	if len(positionals) == 0 {
		return command{}, fmt.Errorf("expected run, check, or keys")
	}
	if positionals[0] == "keys" {
		if len(positionals) != 2 || *path != "" || *broadcast || *watch || *reset != "" {
			return command{}, fmt.Errorf("expected keys create or keys address without service flags")
		}
		switch positionals[1] {
		case "create":
			if *out == "" || *key != "" {
				return command{}, fmt.Errorf("keys create requires --out")
			}
			return command{
				name:    "keys-create",
				keyPath: *out,
			}, nil
		case "address":
			if *key == "" || *out != "" {
				return command{}, fmt.Errorf("keys address requires --key-file")
			}
			return command{
				name:    "keys-address",
				keyPath: *key,
			}, nil
		default:
			return command{}, fmt.Errorf("expected keys create or keys address")
		}
	}
	if len(positionals) != 1 || (positionals[0] != "run" && positionals[0] != "check") {
		return command{}, fmt.Errorf("expected one service command: run or check")
	}
	if !filepath.IsAbs(*path) {
		return command{}, fmt.Errorf("--config must be an absolute path")
	}
	if *out != "" || *key != "" {
		return command{}, fmt.Errorf("set the signing key path in config.json")
	}
	if *broadcast {
		return command{}, fmt.Errorf("relayer always submits proofs; --broadcast is reporter-only")
	}
	if *reset != "" && (positionals[0] != "run" || *watch) {
		return command{}, fmt.Errorf("budget reset requires a one-shot relayer run")
	}
	if positionals[0] == "check" && *watch {
		return command{}, fmt.Errorf("--watch is only for run")
	}
	return command{
		name:       positionals[0],
		network:    *network,
		configPath: *path,
		watch:      *watch,
		reset:      *reset,
	}, nil
}

func parseHeaderCommand(args []string) (command, error) {
	result := command{name: "header-encode"}
	if len(args) == 0 {
		return result, nil
	}
	if (len(args) != 3 && len(args) != 4) || args[0] != "--fixture" {
		return result, fmt.Errorf("usage: header-encode [--fixture VALIDATORS SIGNERS [equal|weighted]]")
	}
	validators, err := strconv.Atoi(args[1])
	if err != nil || validators < 1 || validators > 256 {
		return result, fmt.Errorf("fixture validators must be between 1 and 256")
	}
	signers, err := strconv.Atoi(args[2])
	if err != nil || signers < 0 || signers > validators {
		return result, fmt.Errorf("fixture signers must be between zero and the validator count")
	}
	if len(args) == 4 && args[3] != "equal" && args[3] != "weighted" {
		return result, fmt.Errorf("unknown fixture distribution")
	}
	result.fixture = &fixtureArgs{
		validators: validators,
		signers:    signers,
		weighted:   len(args) == 4 && args[3] == "weighted",
	}
	return result, nil
}

func parseLocalCommand(args []string) (command, error) {
	flags := pflag.NewFlagSet("akt-relayer local", pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	bootstrap := flags.Bool("bootstrap", false, "initialize the local stack")
	invalidHeader := flags.Bool("check-invalid-header", false, "exercise invalid and valid headers")
	tamper := flags.Bool("tamper", false, "local test: alter the header app root")
	watch := flags.Bool("watch", false, "run continuously")
	reset := flags.String("reset-budget", "", "reset one transfer or all")
	flags.String("state", "", "legacy ignored state flag")
	path := flags.String("config", "", "source-tree configuration path")
	if err := flags.Parse(args); err != nil {
		return command{}, err
	}
	positionals := flags.Args()
	if len(positionals) == 0 {
		positionals = []string{"run"}
	}
	result := command{
		name:       "local",
		configPath: *path,
		localArgs:  positionals,
		watch:      *watch,
		reset:      *reset,
		tamper:     *tamper,
	}
	if flags.Changed("config") && (*path == "" || *bootstrap || *invalidHeader || *tamper || len(positionals) != 1 || positionals[0] != "run") {
		return result, fmt.Errorf("source --config only supports run or --reset-budget; bootstrap and diagnostics require the isolated local stack")
	}
	if *tamper && (len(positionals) != 2 || positionals[0] != "update" || *bootstrap || *invalidHeader || *reset != "") {
		return result, fmt.Errorf("--tamper is only for local update HEIGHT")
	}
	// Preserve the original local flag precedence: reset, bootstrap, invalid header.
	if *reset != "" {
		result.localArgs = []string{"reset-budget"}
		return result, nil
	}
	if *bootstrap {
		result.localArgs = []string{"bootstrap"}
		return result, nil
	}
	if *invalidHeader {
		result.localArgs = []string{"check-invalid-header"}
		return result, nil
	}
	switch positionals[0] {
	case "run", "tick", "bootstrap", "check-invalid-header":
		if len(positionals) != 1 {
			return result, fmt.Errorf("unexpected local command arguments")
		}
	case "relay":
		if (len(positionals) != 2 && len(positionals) != 3) || *watch {
			return result, fmt.Errorf("local relay requires an ID and optional header height, and does not watch")
		}
	case "update":
		if len(positionals) != 2 || *watch {
			return result, fmt.Errorf("local relay/update requires one ID/height and does not watch")
		}
	default:
		return result, fmt.Errorf("unknown local command")
	}
	return result, nil
}

const helpText = `akt-relayer run --config /absolute/config.json [--watch]
akt-relayer check --config /absolute/config.json
akt-relayer keys create --out /absolute/key-file
akt-relayer keys address --key-file /absolute/key-file
akt-relayer run --config /absolute/config.json --reset-budget OWNER/HEIGHT/SEQUENCE|all
akt-relayer --version
run/check: --network testnet opts into isolated test networks (default: mainnet).
`

func runOffline(cmd command, build BuildInfo, streams Streams, releaseDir string) (bool, error) {
	switch cmd.name {
	case "help":
		_, err := io.WriteString(streams.Out, helpText)
		return true, err
	case "version":
		info, err := os.ReadFile(filepath.Join(releaseDir, "service-build-info.txt"))
		if err == nil {
			_, err = streams.Out.Write(info)
			return true, err
		}
		if !os.IsNotExist(err) {
			return true, err
		}
		_, err = fmt.Fprintf(streams.Out, "service akt-relayer\nversion %s\ncommit %s\nplatform %s\n", build.Version, build.Commit, build.Platform)
		return true, err
	case "keys-create":
		address, err := createKey(cmd.keyPath)
		if err == nil {
			_, err = fmt.Fprintln(streams.Out, address)
		}
		return true, err
	case "keys-address":
		key, err := loadPayer(cmd.keyPath)
		if err == nil {
			_, err = fmt.Fprintln(streams.Out, key.PublicKey())
		}
		return true, err
	case "check":
		cfg, err := loadConfig(cmd.configPath, cmd.network, releaseDir)
		if err == nil {
			writeNotices(cfg, streams.Err)
			_, err = fmt.Fprintln(streams.Out, "Configuration valid. No RPC request or transaction sent.")
		}
		return true, err
	case "header-encode":
		return true, encodeHeaderCommand(cmd.fixture, streams)
	default:
		return false, nil
	}
}

func writeNotices(cfg config, writer io.Writer) {
	for _, notice := range cfg.notices {
		fmt.Fprintln(writer, notice)
	}
}

func encodeHeaderCommand(fixture *fixtureArgs, streams Streams) error {
	input, err := io.ReadAll(io.LimitReader(streams.In, 4*1024*1024))
	if err != nil {
		return err
	}
	source, err := header.ParseSource(input)
	if err != nil {
		return err
	}
	if fixture != nil {
		source, err = header.Fixture(source, fixture.validators, fixture.signers, fixture.weighted)
		if err != nil {
			return err
		}
	}
	encoded, err := header.Encode(source)
	if err != nil {
		return err
	}
	return json.NewEncoder(streams.Out).Encode(encoded)
}
