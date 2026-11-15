package relayer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/gagliardetto/solana-go"
)

const (
	mainnetGenesis     = "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d"
	developmentProgram = "FfFAPUKqjf68ggj1HvpyzMHFDRnT2cFAdZYYQb7mK9i5"
	maxSafeInteger     = 1<<53 - 1
	encoderNotice      = "headerEncoder is deprecated and ignored: header encoding is built into akt-relayer"
)

type config struct {
	akashRPC             string
	chainID              string
	solanaRPC            string
	genesis              *solana.Hash
	program              solana.PublicKey
	pinnedClient         *solana.PublicKey
	payerPath            string
	stateDir             string
	maxLamports          int64
	maxSignatures        int
	refreshFraction      float64
	retryMaxSeconds      uint64
	pruneIntervalSeconds uint64
	maxAge               time.Duration
	poll                 time.Duration
	local                bool
	notices              []string
}

type configFields map[string]json.RawMessage

func decodeConfigFields(data []byte) (configFields, error) {
	var fields configFields
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return nil, fmt.Errorf("config must be a JSON object")
	}

	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("config must contain one JSON object")
	}

	for name := range fields {
		switch name {
		case "akashRpc", "akashChainId", "solanaRpc", "solanaGenesisHash", "programId",
			"payerKeyFile", "stateDir", "lightClientProgram", "maxLamportsPerTransfer",
			"maxHeaderSignatures", "refreshFraction", "retryMaxSeconds",
			"pruneIntervalSeconds", "maxAgeSeconds", "pollSeconds", "headerEncoder":
		default:
			return nil, fmt.Errorf("unknown config field %s", name)
		}
	}

	return fields, nil
}

func (f configFields) text(name string) (string, error) {
	raw, ok := f[name]
	if !ok {
		return "", fmt.Errorf("missing config field %s", name)
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil || value == "" {
		return "", fmt.Errorf("invalid %s", name)
	}

	return value, nil
}

func (f configFields) number(name string, fallback float64) (float64, error) {
	raw, ok := f[name]
	if !ok {
		return fallback, nil
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("invalid %s", name)
	}

	number, ok := value.(float64)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("invalid %s", name)
	}

	return number, nil
}

func (f configFields) integer(name string, min, max, fallback uint64) (uint64, error) {
	number, err := f.number(name, float64(fallback))
	if err != nil {
		return 0, err
	}

	if math.Trunc(number) != number || number < float64(min) || number > float64(max) || number > maxSafeInteger {
		return 0, fmt.Errorf("invalid %s", name)
	}

	return uint64(number), nil
}

func (f configFields) endpoint(name string) (string, error) {
	value, err := f.text(name)
	if err != nil {
		return "", err
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("invalid %s protocol or host", name)
	}

	if port := parsed.Port(); port != "" {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return "", fmt.Errorf("invalid %s port", name)
		}
	}

	if parsed.User != nil {
		password, _ := parsed.User.Password()
		if parsed.User.Username() != "" || password != "" {
			return "", fmt.Errorf("use a local authenticated proxy instead of credentials in RPC URLs")
		}
	}

	return value, nil
}

func (f configFields) publicKey(name string) (solana.PublicKey, error) {
	value, err := f.text(name)
	if err != nil {
		return solana.PublicKey{}, err
	}

	key, err := solana.PublicKeyFromBase58(value)
	if err != nil || key.String() != value {
		return solana.PublicKey{}, fmt.Errorf("invalid or noncanonical %s", name)
	}

	return key, nil
}

func configPath(fields configFields, name, base string) (string, error) {
	value, err := fields.text(name)
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(value) {
		value = filepath.Join(base, value)
	}

	return filepath.Clean(value), nil
}

func parseConfig(fields configFields, base string) (config, error) {
	var cfg config
	var err error

	if cfg.akashRPC, err = fields.endpoint("akashRpc"); err != nil {
		return cfg, err
	}
	if cfg.solanaRPC, err = fields.endpoint("solanaRpc"); err != nil {
		return cfg, err
	}

	if cfg.chainID, err = fields.text("akashChainId"); err != nil {
		return cfg, err
	}
	if len(utf16.Encode([]rune(cfg.chainID))) > 64 {
		return cfg, fmt.Errorf("invalid akashChainId")
	}

	genesis, err := fields.publicKey("solanaGenesisHash")
	if err != nil {
		return cfg, err
	}
	hash := solana.Hash(genesis)
	cfg.genesis = &hash

	if cfg.program, err = fields.publicKey("programId"); err != nil {
		return cfg, err
	}
	if _, ok := fields["lightClientProgram"]; ok {
		pin, err := fields.publicKey("lightClientProgram")
		if err != nil {
			return cfg, err
		}
		cfg.pinnedClient = &pin
	}

	if cfg.payerPath, err = configPath(fields, "payerKeyFile", base); err != nil {
		return cfg, err
	}
	if cfg.stateDir, err = configPath(fields, "stateDir", base); err != nil {
		return cfg, err
	}

	if _, ok := fields["headerEncoder"]; ok {
		if _, err = configPath(fields, "headerEncoder", base); err != nil {
			return cfg, err
		}
		cfg.notices = append(cfg.notices, encoderNotice)
	}

	maxLamports, err := fields.integer("maxLamportsPerTransfer", 1, 1_000_000_000, 5_000_000)
	if err != nil {
		return cfg, err
	}
	cfg.maxLamports = int64(maxLamports)

	maxSignatures, err := fields.integer("maxHeaderSignatures", 1, 128, 100)
	if err != nil {
		return cfg, err
	}
	cfg.maxSignatures = int(maxSignatures)

	if cfg.refreshFraction, err = fields.number("refreshFraction", 0.2); err != nil {
		return cfg, err
	}
	if cfg.refreshFraction <= 0 || cfg.refreshFraction >= 1 {
		return cfg, fmt.Errorf("invalid refreshFraction")
	}

	if cfg.retryMaxSeconds, err = fields.integer("retryMaxSeconds", 6, 86_400, 900); err != nil {
		return cfg, err
	}
	if cfg.pruneIntervalSeconds, err = fields.integer("pruneIntervalSeconds", 60, 86_400, 600); err != nil {
		return cfg, err
	}

	maxAge, err := fields.integer("maxAgeSeconds", 10, 600, 60)
	if err != nil {
		return cfg, err
	}
	cfg.maxAge = time.Duration(maxAge) * time.Second

	poll, err := fields.integer("pollSeconds", 1, 600, 3)
	if err != nil {
		return cfg, err
	}
	cfg.poll = time.Duration(poll) * time.Second

	return cfg, nil
}

func loadSourceConfig(path, working string) (config, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(working, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return config{}, err
	}

	fields, err := decodeConfigFields(data)
	if err != nil {
		return config{}, err
	}

	return parseConfig(fields, filepath.Dir(path))
}

func loadConfig(path, network, releaseDir string) (config, error) {
	if !filepath.IsAbs(path) {
		return config{}, fmt.Errorf("--config must be an absolute path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return config{}, err
	}

	fields, err := decodeConfigFields(data)
	if err != nil {
		return config{}, err
	}

	cfg, err := parseConfig(fields, filepath.Dir(path))
	if err != nil {
		return cfg, err
	}

	switch network {
	case "mainnet":
		if cfg.chainID != "akashnet-2" || cfg.genesis.String() != mainnetGenesis {
			return cfg, fmt.Errorf("production requires akashnet-2 and Solana mainnet-beta")
		}
		if cfg.program.String() == developmentProgram {
			return cfg, fmt.Errorf("replace the development AKT program ID")
		}
	case "testnet":
		if cfg.chainID == "akashnet-2" || cfg.genesis.String() == mainnetGenesis {
			return cfg, fmt.Errorf("testnet mode refuses Akash or Solana mainnet")
		}
	default:
		return cfg, fmt.Errorf("network must be mainnet or testnet")
	}

	for _, name := range []string{"payerKeyFile", "stateDir", "headerEncoder"} {
		if _, present := fields[name]; name == "headerEncoder" && !present {
			continue
		}
		value, err := fields.text(name)
		if err != nil || !filepath.IsAbs(value) {
			return cfg, fmt.Errorf("%s must be an absolute path", name)
		}
	}

	for _, part := range strings.Split(cfg.stateDir, string(filepath.Separator)) {
		if part == ".cache" {
			return cfg, fmt.Errorf("production state must survive release/cache cleanup")
		}
	}

	fromRelease, err := filepath.Rel(releaseDir, cfg.stateDir)
	if err != nil {
		return cfg, err
	}
	if !strings.HasPrefix(fromRelease, ".."+string(filepath.Separator)) && !filepath.IsAbs(fromRelease) {
		return cfg, fmt.Errorf("production state must live outside the release directory")
	}

	return cfg, nil
}

func localNumber(getenv func(string) (string, bool), name string, fallback float64) (float64, error) {
	value, present := getenv(name)
	if !present {
		return fallback, nil
	}

	// Match ECMAScript WhiteSpace and LineTerminator, not Go's Unicode space set.
	value = strings.TrimFunc(value, func(char rune) bool {
		switch char {
		case '\t', '\n', '\v', '\f', '\r', ' ', '\u00a0', '\u1680',
			'\u2028', '\u2029', '\u202f', '\u205f', '\u3000', '\ufeff':
			return true
		}

		return char >= '\u2000' && char <= '\u200a'
	})
	if value == "" {
		return 0, nil
	}

	// Go accepts digit separators and hexadecimal floats; Number(string) does not.
	if strings.ContainsAny(value, "_pP") {
		return 0, fmt.Errorf("invalid %s", name)
	}

	if len(value) > 2 && value[0] == '0' && strings.ContainsAny(value[1:2], "xXbBoO") {
		number, err := strconv.ParseUint(value, 0, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid %s", name)
		}
		return float64(number), nil
	}

	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
		return 0, fmt.Errorf("invalid %s", name)
	}

	return number, nil
}

func localInteger(getenv func(string) (string, bool), name string, min, max, fallback uint64) (uint64, error) {
	value, err := localNumber(getenv, name, float64(fallback))
	if err != nil {
		return 0, err
	}

	if value < float64(min) || value > float64(max) || math.Trunc(value) != value {
		return 0, fmt.Errorf("invalid %s", name)
	}

	return uint64(value), nil
}

func localNetworkConfig(getenv func(string) (string, bool), workspace string) (config, error) {
	profile, present := getenv("AKT_LOCAL_PROFILE")
	if !present {
		profile = "single"
	}
	if profile != "single" && profile != "testnetify" {
		return config{}, fmt.Errorf("unknown local profile")
	}
	run, _ := getenv("AKT_LOCAL_RUN")
	if len(run) > 32 || (run != "" && !validLocalRun(run)) {
		return config{}, fmt.Errorf("invalid isolated run name")
	}

	prefix, chain := "live-stack-v2", "akash-bridge-v2-local"
	akashPort, grpcPort, restPort, solanaPort := uint64(26657), uint64(9090), uint64(1318), uint64(8899)
	if profile == "testnetify" {
		prefix, chain = "testnetify-stack-v2", "akash-bridge-v2-testnetify"
		akashPort, grpcPort, restPort, solanaPort = 27657, 9190, 1317, 18999
	}
	if run != "" {
		prefix += "-" + run
		chain += "-" + run
	}

	ports := []struct {
		name  string
		value *uint64
	}{
		{
			name:  "AKT_AKASH_RPC_PORT",
			value: &akashPort,
		},
		{
			name:  "AKT_AKASH_GRPC_PORT",
			value: &grpcPort,
		},
		{
			name:  "AKT_AKASH_REST_PORT",
			value: &restPort,
		},
		{
			name:  "AKT_SOLANA_RPC_PORT",
			value: &solanaPort,
		},
	}
	for _, port := range ports {
		value, err := localInteger(getenv, port.name, 1025, 65534, *port.value)
		if err != nil {
			return config{}, err
		}
		*port.value = value
	}

	root := filepath.Join(workspace, ".cache", "solana", "run", prefix)
	cfg := config{
		akashRPC:  fmt.Sprintf("http://127.0.0.1:%d", akashPort),
		chainID:   chain + "-1",
		solanaRPC: fmt.Sprintf("http://127.0.0.1:%d", solanaPort),
		program:   solana.MustPublicKeyFromBase58(developmentProgram),
		payerPath: filepath.Join(root, "solana-keypair.json"),
		stateDir:  root,
		maxAge:    time.Minute,
		poll:      3 * time.Second,
		local:     true,
	}

	return cfg, nil
}

func localConfig(getenv func(string) (string, bool), workspace string) (config, error) {
	cfg, err := localNetworkConfig(getenv, workspace)
	if err != nil {
		return cfg, err
	}

	if cfg.refreshFraction, err = localNumber(getenv, "AKT_RELAY_REFRESH_FRACTION", 0.2); err != nil {
		return cfg, err
	}
	if cfg.refreshFraction <= 0 || cfg.refreshFraction >= 1 {
		return cfg, fmt.Errorf("AKT_RELAY_REFRESH_FRACTION must be between 0 and 1")
	}

	if cfg.retryMaxSeconds, err = localInteger(getenv, "AKT_RELAY_RETRY_MAX_SECONDS", 6, maxSafeInteger, 900); err != nil {
		return cfg, err
	}
	if cfg.pruneIntervalSeconds, err = localInteger(getenv, "AKT_RELAY_PRUNE_INTERVAL_SECONDS", 60, maxSafeInteger, 600); err != nil {
		return cfg, err
	}

	maxLamports, err := localInteger(getenv, "AKT_RELAY_MAX_LAMPORTS_PER_TRANSFER", 1, maxSafeInteger, 5_000_000)
	if err != nil {
		return cfg, err
	}
	cfg.maxLamports = int64(maxLamports)

	maxSignatures, err := localInteger(getenv, "AKT_RELAY_MAX_HEADER_SIGNATURES", 1, 128, 100)
	if err != nil {
		return cfg, err
	}
	cfg.maxSignatures = int(maxSignatures)

	return cfg, nil
}

func validLocalRun(value string) bool {
	for i, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && (char != '-' || i == 0) {
			return false
		}
	}
	return true
}
