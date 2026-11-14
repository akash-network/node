package config_test

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/gagliardetto/solana-go"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/config"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

const (
	flagHome     = "--home"
	envHome      = "HOME=/users/x"
	errReporter  = "reporter: invalid reporter address"
	errGasPrices = "gas-prices: must be a single positive uakt price"
	remoteAddr   = "signer:9000"
	keyName      = "reporter"

	errGasAdjustment = "gas-adjustment: must be between"
)

const testGenesis = "4uhcVJyU9pJkvQyS88uRDiswHXSCkY3zQawwpjk2NsNY"

func reporterAddress(t *testing.T) string {
	t.Helper()

	addr, err := bech32.ConvertAndEncode("akash", bytes.Repeat([]byte{7}, 20))
	require.NoError(t, err)

	return addr
}

// valid returns a testnet configuration that satisfies every rule, previewing.
func valid(t *testing.T) config.Config {
	t.Helper()

	program := solana.TokenProgramID

	c := config.Default()
	c.Network = "testnet"
	c.Reporter = reporterAddress(t)
	c.ChainID = "akashnet-test"
	c.Solana.GenesisHash = testGenesis
	c.Solana.ProgramID = program.String()
	c.Solana.Mint = aktsolana.PDA(program, []byte("mint")).String()

	return c
}

// load parses args, writes file to <home>/config/reporter.toml when it is non-empty, and loads.
func load(t *testing.T, file string, environ []string, args ...string) (config.Config, []string, error) {
	t.Helper()

	return loadWith(t, config.Load, file, environ, args...)
}

// loadWith is load with another loader.
func loadWith(
	t *testing.T,
	loader func(*pflag.FlagSet, []string) (config.Config, []string, error),
	file string,
	environ []string,
	args ...string,
) (config.Config, []string, error) {
	t.Helper()

	home := t.TempDir()
	if file != "" {
		require.NoError(t, os.MkdirAll(filepath.Join(home, "config"), 0o750))
		require.NoError(t, os.WriteFile(config.FilePath(home), []byte(file), 0o600))
	}

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.AddFlags(fs)
	require.NoError(t, fs.Parse(append([]string{flagHome, home}, args...)))

	return loader(fs, environ)
}

func TestDefaults(t *testing.T) {
	c := config.Default()

	require.Equal(t, "mainnet", c.Network)
	require.False(t, c.Broadcast)
	require.Empty(t, c.Reporter)
	require.Empty(t, c.ChainID)
	require.Equal(t, "tcp://localhost:26657", c.Node)
	require.Equal(t, "localhost:9090", c.GRPCAddr)
	require.False(t, c.GRPCInsecure)
	require.Equal(t, 60*time.Second, c.MaxAge)
	require.Equal(t, 15*time.Second, c.RequestTimeout)
	require.Empty(t, c.From)
	require.Equal(t, "test", c.KeyringBackend)
	require.Empty(t, c.KeyringDir)
	require.Equal(t, "auto", c.Gas)
	require.Equal(t, 1.5, c.GasAdjustment)
	require.Equal(t, "0.025uakt", c.GasPrices)
	require.Empty(t, c.Fees)
	require.Equal(t, "http://localhost:8899", c.Solana.RPC)
	require.Empty(t, c.Solana.GenesisHash)
	require.Empty(t, c.Solana.ProgramID)
	require.Empty(t, c.Solana.Mint)
	require.Empty(t, c.Solana.LightClientProgram)
	require.Zero(t, c.Solana.MaxSlotLead)
	require.Equal(t, 15*time.Second, c.Scan.Interval)
	require.Equal(t, 100, c.Scan.AuditPerScan)
	require.Equal(t, 500, c.Scan.PageSize)
	require.Equal(t, 16, c.Scan.Workers)
	require.Equal(t, 32, c.Scan.AkashConcurrency)
	require.Equal(t, 8, c.Scan.SolanaConcurrency)
	require.Equal(t, 2, c.Scan.BlockConcurrency)
	require.Equal(t, uint64(20_000_000), c.Tx.MaxGas)
	require.Equal(t, "1000000uakt", c.Tx.MaxFee)
	require.Equal(t, 25, c.Tx.MaxMsgs)
	require.Equal(t, 8, c.Tx.MaxInFlight)
	require.Empty(t, c.Signer.Remote.Address)
	require.Empty(t, c.Signer.Remote.KeyID)
	require.Empty(t, c.Signer.Remote.CAFile)
	require.Empty(t, c.Signer.Remote.ServerName)
	require.Empty(t, c.Signer.Remote.CertFile)
	require.Empty(t, c.Signer.Remote.KeyFile)
	require.Equal(t, 10*time.Second, c.Signer.Remote.Timeout)
	require.Empty(t, c.Metrics.Listen)

	known := config.Known()
	require.Len(t, known, 41)
	require.NotContains(t, known, "tx.timeout")
	require.NotContains(t, known, "tx.batch-window")

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.AddFlags(fs)

	for _, key := range known {
		require.NotNil(t, fs.Lookup(key), "flag --%s", key)
	}

	got, _, err := load(t, "", nil)
	require.NoError(t, err)
	require.Equal(t, 16, got.Scan.Workers)
}

func TestKeysAreDocumented(t *testing.T) {
	out, err := config.Render(config.Default())
	require.NoError(t, err)

	v := viper.New()
	v.SetConfigType("toml")
	require.NoError(t, v.ReadConfig(bytes.NewReader(out)))
	require.ElementsMatch(t, config.Known(), v.AllKeys())

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.AddFlags(fs)

	for _, key := range config.Known() {
		require.NotEmpty(t, fs.Lookup(key).Usage, key)
	}
}

func TestPrecedence(t *testing.T) {
	file := "[scan]\nworkers = 7\n"

	c, _, err := load(t, "", nil)
	require.NoError(t, err)
	require.Equal(t, 16, c.Scan.Workers, "default")

	c, _, err = load(t, file, nil)
	require.NoError(t, err)
	require.Equal(t, 7, c.Scan.Workers, "file")

	env := []string{"AKT_REPORTER_SCAN_WORKERS=8"}

	c, _, err = load(t, file, env)
	require.NoError(t, err)
	require.Equal(t, 8, c.Scan.Workers, "environment")

	c, _, err = load(t, file, env, "--scan.workers", "9")
	require.NoError(t, err)
	require.Equal(t, 9, c.Scan.Workers, "flag")
}

func TestLoadWithoutFile(t *testing.T) {
	file := "retired-key = 1\n[scan]\nworkers = 7\n"

	c, _, err := loadWith(t, config.LoadWithoutFile, file, nil)
	require.NoError(t, err, "the file, with its unknown key, is never read")
	require.Equal(t, 16, c.Scan.Workers, "default, not the file")

	env := []string{"AKT_REPORTER_SCAN_WORKERS=8", "AKT_REPORTER_SERVICE_HOST=1"}

	c, warnings, err := loadWith(t, config.LoadWithoutFile, file, env)
	require.NoError(t, err)
	require.Equal(t, 8, c.Scan.Workers, "environment")
	require.Len(t, warnings, 1)

	c, _, err = loadWith(t, config.LoadWithoutFile, file, env, "--scan.workers", "9")
	require.NoError(t, err)
	require.Equal(t, 9, c.Scan.Workers, "flag")
	require.NotEmpty(t, c.KeyringDir, "keyring-dir still defaults to the home directory")
}

func TestPrecedenceKinds(t *testing.T) {
	c, _, err := load(t, "max-age = \"30s\"\nbroadcast = true\ngas = 200000\ngas-adjustment = 2\n",
		[]string{"AKT_REPORTER_SOLANA_MAX_SLOT_LEAD=500", "AKT_REPORTER_SIGNER_REMOTE_TIMEOUT=5s", "AKT_REPORTER_GRPC_INSECURE=true"},
		"--request-timeout", "20s")
	require.NoError(t, err)
	require.Equal(t, 30*time.Second, c.MaxAge)
	require.True(t, c.Broadcast)
	require.Equal(t, "200000", c.Gas)
	require.Equal(t, 2.0, c.GasAdjustment)
	require.Equal(t, uint64(500), c.Solana.MaxSlotLead)
	require.Equal(t, 5*time.Second, c.Signer.Remote.Timeout)
	require.True(t, c.GRPCInsecure)
	require.Equal(t, 20*time.Second, c.RequestTimeout)
}

func TestHome(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	config.AddFlags(fs)

	home, err := config.ResolveHome(fs, []string{envHome})
	require.NoError(t, err)
	require.Equal(t, "/users/x/.akt-reporter", home)

	home, err = config.ResolveHome(fs, []string{envHome, "AKT_REPORTER_HOME=/env"})
	require.NoError(t, err)
	require.Equal(t, "/env", home)

	require.NoError(t, fs.Parse([]string{flagHome, "/flag"}))

	home, err = config.ResolveHome(fs, []string{envHome, "AKT_REPORTER_HOME=/env"})
	require.NoError(t, err)
	require.Equal(t, "/flag", home)

	c, _, err := config.Load(fs, nil)
	require.NoError(t, err)
	require.Equal(t, "/flag", c.KeyringDir, "keyring-dir defaults to the home directory")
}

func TestUnknownFileKeyFails(t *testing.T) {
	_, _, err := load(t, "[scan]\nwrokers = 3\n", nil)
	require.ErrorContains(t, err, "scan.wrokers")

	for _, key := range []string{"tx.timeout", "tx.batch-window"} {
		table, name, _ := strings.Cut(key, ".")
		_, _, err = load(t, "["+table+"]\n"+name+" = \"10s\"\n", nil)
		require.ErrorContains(t, err, key)
	}

	_, _, err = load(t, "scan.wrokers = 3\n", nil)
	require.ErrorContains(t, err, "scan.wrokers")
}

func TestUnknownEnvWarns(t *testing.T) {
	_, warnings, err := load(t, "", []string{"AKT_REPORTER_SERVICE_HOST=1", "AKT_REPORTER_SCAN_WORKERS=4", "AKT_REPORTER_HOME=/x", "PATH=/bin"})
	require.NoError(t, err)
	require.Equal(t, []string{"ignoring unknown environment variable AKT_REPORTER_SERVICE_HOST"}, warnings)
}

func TestBadValueFails(t *testing.T) {
	_, _, err := load(t, "", []string{"AKT_REPORTER_SCAN_WORKERS=many"})
	require.ErrorContains(t, err, "scan.workers")
}

func TestNaNGasAdjustmentFromEnvFailsResolve(t *testing.T) {
	c, _, err := load(t, "", []string{"AKT_REPORTER_GAS_ADJUSTMENT=NaN"})
	require.NoError(t, err)

	c2 := valid(t)
	c2.GasAdjustment = c.GasAdjustment

	_, err = config.Resolve(c2)
	require.ErrorContains(t, err, errGasAdjustment)
}

func TestIntegerDurationInFileFails(t *testing.T) {
	_, _, err := load(t, "max-age = 60\n", nil)
	require.ErrorContains(t, err, "max-age: must be a duration string")

	_, _, err = load(t, "[scan]\ninterval = 15\n[signer.remote]\ntimeout = 10\n", nil)
	require.ErrorContains(t, err, "scan.interval: must be a duration string")
	require.ErrorContains(t, err, "signer.remote.timeout: must be a duration string")

	c, _, err := load(t, "max-age = \"60s\"\n", nil)
	require.NoError(t, err)
	require.Equal(t, 60*time.Second, c.MaxAge)

	_, _, err = load(t, "", []string{"AKT_REPORTER_MAX_AGE=60"})
	require.Error(t, err, "an integer from the environment lacks a unit")
}

func TestResolveValid(t *testing.T) {
	c := valid(t)
	c.Solana.LightClientProgram = solana.SystemProgramID.String()
	c.Fees = "5000uakt"

	r, err := config.Resolve(c)
	require.NoError(t, err)
	require.Equal(t, solana.TokenProgramID, r.ProgramID)
	require.Equal(t, aktsolana.PDA(solana.TokenProgramID, []byte("mint")), r.Mint)
	require.Equal(t, solana.SystemProgramID, *r.LightClientPin)
	require.Equal(t, bytes.Repeat([]byte{7}, 20), []byte(r.Reporter))
	require.Zero(t, r.GasPerMessage)
	prices, err := sdk.ParseDecCoins("0.025uakt")
	require.NoError(t, err)
	require.Equal(t, prices, r.GasPrices)
	require.Equal(t, "5000uakt", r.Fees.String())
	require.Equal(t, "1000000uakt", r.MaxFee.String())
	require.Equal(t, "testnet", r.Network)

	c.Gas = "300000"
	r, err = config.Resolve(c)
	require.NoError(t, err)
	require.Equal(t, uint64(300000), r.GasPerMessage)

	c.Solana.LightClientProgram = ""
	r, err = config.Resolve(c)
	require.NoError(t, err)
	require.Nil(t, r.LightClientPin)
}

func TestResolveRules(t *testing.T) {
	mainnet := func(c *config.Config) {
		c.Network = "mainnet"
		c.ChainID = config.MainnetChainID
		c.Solana.GenesisHash = config.MainnetGenesis
	}

	devProgram := solana.MustPublicKeyFromBase58(config.DevProgramID)
	otherProgram := solana.SystemProgramID

	cases := []struct {
		name   string
		mutate func(*config.Config)
		want   string // empty means valid
	}{
		{"valid testnet", func(*config.Config) {}, ""},
		{"valid mainnet", func(c *config.Config) {
			mainnet(c)
			c.Solana.ProgramID = otherProgram.String()
			c.Solana.Mint = aktsolana.PDA(otherProgram, []byte("mint")).String()
		}, ""},
		{"network unknown", func(c *config.Config) { c.Network = "devnet" }, "network: must be mainnet or testnet"},
		{"reporter missing", func(c *config.Config) { c.Reporter = "" }, "reporter: is required"},
		{"reporter uppercase", func(c *config.Config) { c.Reporter = strings.ToUpper(c.Reporter) }, errReporter},
		{"reporter wrong prefix", func(c *config.Config) {
			c.Reporter, _ = bech32.ConvertAndEncode("cosmos", bytes.Repeat([]byte{7}, 20))
		}, errReporter},
		{"reporter 32 bytes", func(c *config.Config) {
			c.Reporter, _ = bech32.ConvertAndEncode("akash", bytes.Repeat([]byte{7}, 32))
		}, errReporter},
		{"reporter garbage", func(c *config.Config) { c.Reporter = "akash1nope" }, errReporter},
		{"chain-id missing", func(c *config.Config) { c.ChainID = "" }, "chain-id: is required"},
		{"node websocket", func(c *config.Config) { c.Node = "ws://localhost:26657" }, "node: scheme must be one of"},
		{"node credentials", func(c *config.Config) { c.Node = "http://user:s3cret@localhost:26657" }, "node: use a local authenticated proxy"},
		{"node empty", func(c *config.Config) { c.Node = "" }, "node: is required"},
		{"node https", func(c *config.Config) { c.Node = "https://rpc.example.com:443" }, ""},
		{"grpc-addr without port", func(c *config.Config) { c.GRPCAddr = "localhost" }, "grpc-addr: must be host:port"},
		{"grpc-addr port 0", func(c *config.Config) { c.GRPCAddr = "localhost:0" }, "grpc-addr: invalid port"},
		{"max-age zero", func(c *config.Config) { c.MaxAge = 0 }, "max-age: must be above 0"},
		{"max-age 10m", func(c *config.Config) { c.MaxAge = 10 * time.Minute }, ""},
		{"max-age over 10m", func(c *config.Config) { c.MaxAge = 10*time.Minute + time.Second }, "max-age: must be above 0"},
		{"request-timeout low", func(c *config.Config) { c.RequestTimeout = 999 * time.Millisecond }, "request-timeout: must be between"},
		{"request-timeout high", func(c *config.Config) { c.RequestTimeout = 61 * time.Second }, "request-timeout: must be between"},
		{"keyring-backend unknown", func(c *config.Config) { c.KeyringBackend = "memory" }, "keyring-backend: must be os, file or test"},
		{"keyring-backend os", func(c *config.Config) { c.KeyringBackend = "os" }, ""},
		{"gas zero", func(c *config.Config) { c.Gas = "0" }, "gas: must be auto or a positive integer"},
		{"gas text", func(c *config.Config) { c.Gas = "lots" }, "gas: must be auto or a positive integer"},
		{"gas over max-gas", func(c *config.Config) { c.Gas = "20000001" }, "gas: must be at most tx.max-gas"},
		{"gas at max-gas", func(c *config.Config) { c.Gas = "20000000" }, ""},
		{"gas-adjustment low", func(c *config.Config) { c.GasAdjustment = 0.9 }, errGasAdjustment},
		{"gas-adjustment high", func(c *config.Config) { c.GasAdjustment = 10.1 }, errGasAdjustment},
		{"gas-adjustment NaN", func(c *config.Config) { c.GasAdjustment = math.NaN() }, errGasAdjustment},
		{"gas-adjustment +Inf", func(c *config.Config) { c.GasAdjustment = math.Inf(1) }, errGasAdjustment},
		{"gas-adjustment bounds", func(c *config.Config) { c.GasAdjustment = 10 }, ""},
		{"gas-prices other denom", func(c *config.Config) { c.GasPrices = "0.025uatom" }, errGasPrices},
		{"gas-prices two coins", func(c *config.Config) { c.GasPrices = "0.025uakt,0.1uatom" }, errGasPrices},
		{"gas-prices garbage", func(c *config.Config) { c.GasPrices = "cheap" }, errGasPrices},
		{"gas-prices empty without fees", func(c *config.Config) { c.GasPrices = "" }, "gas-prices: is required when fees is empty"},
		{"gas-prices empty with fees", func(c *config.Config) { c.GasPrices = ""; c.Fees = "5000uakt" }, ""},
		{"fees other denom", func(c *config.Config) { c.Fees = "5000uatom" }, "fees: must be a whole amount of uakt"},
		{"fees fractional", func(c *config.Config) { c.Fees = "1.5uakt" }, "fees: must be a whole amount of uakt"},
		{"fees zero", func(c *config.Config) { c.Fees = "0uakt" }, "fees: must be above 0"},
		{"fees over max-fee", func(c *config.Config) { c.Fees = "1000001uakt" }, "fees: must be at most tx.max-fee"},
		{"fees over lowered max-fee", func(c *config.Config) { c.Fees = "6uakt"; c.Tx.MaxFee = "5uakt" }, "fees: must be at most tx.max-fee"},
		{"fees at max-fee", func(c *config.Config) { c.Fees = "1000000uakt" }, ""},
		{"solana.rpc ftp", func(c *config.Config) { c.Solana.RPC = "ftp://localhost:8899" }, "solana.rpc: scheme must be one of"},
		{"solana.rpc tcp", func(c *config.Config) { c.Solana.RPC = "tcp://localhost:8899" }, "solana.rpc: scheme must be one of"},
		{"solana.rpc credentials", func(c *config.Config) { c.Solana.RPC = "https://key@rpc.example.com" }, "solana.rpc: use a local authenticated proxy"},
		{"genesis missing", func(c *config.Config) { c.Solana.GenesisHash = "" }, "solana.genesis-hash: is required"},
		{"genesis not base58", func(c *config.Config) { c.Solana.GenesisHash = "0OIl" }, "solana.genesis-hash: must be a base58 public key of 32 bytes"},
		{"genesis short", func(c *config.Config) { c.Solana.GenesisHash = "3Bxs" }, "solana.genesis-hash: must be a base58 public key of 32 bytes"},
		{"program missing", func(c *config.Config) { c.Solana.ProgramID = "" }, "solana.program-id: is required"},
		{"program garbage", func(c *config.Config) { c.Solana.ProgramID = "program" }, "solana.program-id: must be a base58 public key"},
		{"mint missing", func(c *config.Config) { c.Solana.Mint = "" }, "solana.mint: is required"},
		{"mint not the PDA", func(c *config.Config) { c.Solana.Mint = solana.SystemProgramID.String() }, "solana.mint: noncanonical mint"},
		{"light client garbage", func(c *config.Config) { c.Solana.LightClientProgram = "x" }, "solana.light-client-program: must be a base58 public key"},
		{"max-slot-lead limit", func(c *config.Config) { c.Solana.MaxSlotLead = 100_000 }, ""},
		{"max-slot-lead over", func(c *config.Config) { c.Solana.MaxSlotLead = 100_001 }, "solana.max-slot-lead: must be between"},
		{"scan.interval low", func(c *config.Config) { c.Scan.Interval = 0 }, "scan.interval: must be between"},
		{"scan.interval high", func(c *config.Config) { c.Scan.Interval = 61 * time.Second }, "scan.interval: must be between"},
		{"scan.audit-per-scan zero", func(c *config.Config) { c.Scan.AuditPerScan = 0 }, "scan.audit-per-scan: must be between"},
		{"scan.audit-per-scan high", func(c *config.Config) { c.Scan.AuditPerScan = 10_001 }, "scan.audit-per-scan: must be between"},
		{"scan.page-size high", func(c *config.Config) { c.Scan.PageSize = 5_001 }, "scan.page-size: must be between"},
		{"scan.workers zero", func(c *config.Config) { c.Scan.Workers = 0 }, "scan.workers: must be between"},
		{"scan.workers high", func(c *config.Config) { c.Scan.Workers = 257 }, "scan.workers: must be between"},
		{"scan.akash-concurrency high", func(c *config.Config) { c.Scan.AkashConcurrency = 257 }, "scan.akash-concurrency: must be between"},
		{"scan.solana-concurrency high", func(c *config.Config) { c.Scan.SolanaConcurrency = 65 }, "scan.solana-concurrency: must be between"},
		{"scan.block-concurrency high", func(c *config.Config) { c.Scan.BlockConcurrency = 17 }, "scan.block-concurrency: must be between"},
		{"tx.max-gas zero", func(c *config.Config) { c.Tx.MaxGas = 0 }, "tx.max-gas: must be between"},
		{"tx.max-gas high", func(c *config.Config) { c.Tx.MaxGas = 20_000_001 }, "tx.max-gas: must be between"},
		{"tx.max-fee other denom", func(c *config.Config) { c.Tx.MaxFee = "1uatom" }, "tx.max-fee: must be a whole amount of uakt"},
		{"tx.max-fee high", func(c *config.Config) { c.Tx.MaxFee = "1000001uakt" }, "tx.max-fee: must be at most 1000000uakt"},
		{"tx.max-msgs zero", func(c *config.Config) { c.Tx.MaxMsgs = 0 }, "tx.max-msgs: must be between"},
		{"tx.max-msgs high", func(c *config.Config) { c.Tx.MaxMsgs = 101 }, "tx.max-msgs: must be between"},
		{"tx.max-in-flight high", func(c *config.Config) { c.Tx.MaxInFlight = 65 }, "tx.max-in-flight: must be between"},
		{"remote address without port", func(c *config.Config) { c.Signer.Remote.Address = "signer"; c.Signer.Remote.KeyID = "k" }, "signer.remote.address: must be host:port"},
		{"remote address without key-id", func(c *config.Config) { c.Signer.Remote.Address = remoteAddr }, "signer.remote.key-id: is required"},
		{"remote cert without key", func(c *config.Config) { c.Signer.Remote.CertFile = "/c.pem" }, "signer.remote.cert-file: set signer.remote.cert-file and signer.remote.key-file together"},
		{"remote key without cert", func(c *config.Config) { c.Signer.Remote.KeyFile = "/k.pem" }, "signer.remote.cert-file: set"},
		{"remote cert and key", func(c *config.Config) { c.Signer.Remote.CertFile = "/c.pem"; c.Signer.Remote.KeyFile = "/k.pem" }, ""},
		{"remote timeout low", func(c *config.Config) { c.Signer.Remote.Timeout = 0 }, "signer.remote.timeout: must be between"},
		{"remote timeout high", func(c *config.Config) { c.Signer.Remote.Timeout = 121 * time.Second }, "signer.remote.timeout: must be between"},
		{"metrics.listen no port", func(c *config.Config) { c.Metrics.Listen = "0.0.0.0" }, "metrics.listen: must be host:port"},
		{"metrics.listen", func(c *config.Config) { c.Metrics.Listen = "127.0.0.1:9100" }, ""},
		{"mainnet wrong chain-id", func(c *config.Config) {
			mainnet(c)
			c.ChainID = "akashnet-test"
			c.Solana.ProgramID = otherProgram.String()
			c.Solana.Mint = aktsolana.PDA(otherProgram, []byte("mint")).String()
		}, "network: mainnet requires chain-id akashnet-2"},
		{"mainnet wrong genesis", func(c *config.Config) {
			mainnet(c)
			c.Solana.GenesisHash = testGenesis
			c.Solana.ProgramID = otherProgram.String()
			c.Solana.Mint = aktsolana.PDA(otherProgram, []byte("mint")).String()
		}, "network: mainnet requires solana.genesis-hash " + config.MainnetGenesis},
		{"mainnet dev program", func(c *config.Config) {
			mainnet(c)
			c.Solana.ProgramID = devProgram.String()
			c.Solana.Mint = aktsolana.PDA(devProgram, []byte("mint")).String()
		}, "network: mainnet refuses the development program ID"},
		{"testnet mainnet chain-id", func(c *config.Config) { c.ChainID = config.MainnetChainID }, "network: testnet refuses the mainnet chain-id"},
		{"testnet mainnet genesis", func(c *config.Config) { c.Solana.GenesisHash = config.MainnetGenesis }, "network: testnet refuses the mainnet solana.genesis-hash"},
		{"testnet dev program", func(c *config.Config) {
			c.Solana.ProgramID = devProgram.String()
			c.Solana.Mint = aktsolana.PDA(devProgram, []byte("mint")).String()
		}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := valid(t)
			tc.mutate(&c)

			r, err := config.Resolve(c)
			if tc.want == "" {
				require.NoError(t, err)

				return
			}

			require.ErrorContains(t, err, tc.want)
			require.Equal(t, config.Resolved{}, r)
		})
	}
}

func TestResolveNeverEchoesCredentials(t *testing.T) {
	c := valid(t)
	c.Node = "http://user:s3cret@localhost:26657"
	c.Solana.RPC = "https://token-in-userinfo@rpc.example.com"

	_, err := config.Resolve(c)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "s3cret")
	require.NotContains(t, err.Error(), "token-in-userinfo")
}

func TestResolveReportsEveryRule(t *testing.T) {
	_, err := config.Resolve(config.Default())
	require.Error(t, err)

	for _, want := range []string{"reporter: is required", "chain-id: is required", "solana.genesis-hash: is required", "solana.program-id: is required", "solana.mint: is required"} {
		require.ErrorContains(t, err, want)
	}
}

func TestBroadcastNeedsExactlyOneSigner(t *testing.T) {
	c := valid(t)
	c.Broadcast = true

	_, err := config.Resolve(c)
	require.ErrorContains(t, err, "broadcast needs exactly one of from and signer.remote.address")

	c.From = keyName
	_, err = config.Resolve(c)
	require.NoError(t, err)

	c.Signer.Remote.Address = remoteAddr
	c.Signer.Remote.KeyID = keyName
	_, err = config.Resolve(c)
	require.ErrorContains(t, err, "configure from or signer.remote.address, not both")

	c.From = ""
	_, err = config.Resolve(c)
	require.NoError(t, err)

	c.Broadcast = false
	c.From = keyName
	_, err = config.Resolve(c)
	require.NoError(t, err, "preview does not need a signer")
}

func TestBroadcastAgeBudget(t *testing.T) {
	remote := valid(t)
	remote.Broadcast = true
	remote.Signer.Remote.Address = remoteAddr
	remote.Signer.Remote.KeyID = keyName
	remote.Signer.Remote.Timeout = 10 * time.Second

	remote.MaxAge = 70 * time.Second
	_, err := config.Resolve(remote)
	require.NoError(t, err)

	remote.MaxAge = 71 * time.Second
	_, err = config.Resolve(remote)
	require.ErrorContains(t, err, "max-age")
	require.ErrorContains(t, err, "signer.remote.timeout")

	keyring := valid(t)
	keyring.Broadcast = true
	keyring.From = keyName
	keyring.Signer.Remote.Timeout = 120 * time.Second // ignored: the remote signer is not in use

	keyring.MaxAge = 80 * time.Second
	_, err = config.Resolve(keyring)
	require.NoError(t, err)

	keyring.MaxAge = 81 * time.Second
	_, err = config.Resolve(keyring)
	require.ErrorContains(t, err, "max-age")

	preview := valid(t)
	preview.MaxAge = 10 * time.Minute
	preview.Signer.Remote.Address = remoteAddr
	preview.Signer.Remote.KeyID = keyName
	_, err = config.Resolve(preview)
	require.NoError(t, err)
}

func TestSlotLeadDerived(t *testing.T) {
	c := valid(t)
	c.Solana.MaxSlotLead = 0
	c.MaxAge = 60 * time.Second

	r, err := config.Resolve(c)
	require.NoError(t, err)
	require.Equal(t, uint64(600), r.SlotLead)

	c.Solana.MaxSlotLead = 1234
	r, err = config.Resolve(c)
	require.NoError(t, err)
	require.Equal(t, uint64(1234), r.SlotLead)
}

func TestRenderRoundTrip(t *testing.T) {
	roundTrip := func(t *testing.T, c config.Config) {
		t.Helper()

		out, err := config.Render(c)
		require.NoError(t, err)

		home := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(home, "config"), 0o750))
		require.NoError(t, os.WriteFile(config.FilePath(home), out, 0o600))

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		config.AddFlags(fs)
		require.NoError(t, fs.Parse([]string{flagHome, home}))

		got, warnings, err := config.Load(fs, nil)
		require.NoError(t, err)
		require.Empty(t, warnings)

		if c.KeyringDir == "" {
			c.KeyringDir = home // Load fills the home directory
		}

		require.Equal(t, c, got)
	}

	t.Run("defaults with required keys", func(t *testing.T) {
		roundTrip(t, valid(t))
	})

	t.Run("every field non-default", func(t *testing.T) {
		c := valid(t)
		// Set every leaf to a value that differs from its zero value and default.
		n := 0
		var set func(v reflect.Value)
		set = func(v reflect.Value) {
			for i := range v.NumField() {
				f := v.Field(i)
				n++

				switch f.Kind() {
				case reflect.Struct:
					set(f)
				case reflect.String:
					if f.String() == "" || f.String() == config.Default().Network {
						f.SetString("value-" + string(rune('a'+n%26)))
					}
				case reflect.Bool:
					f.SetBool(true)
				case reflect.Int:
					f.SetInt(f.Int() + 1)
				case reflect.Uint64:
					f.SetUint(f.Uint() + 1)
				case reflect.Float64:
					f.SetFloat(f.Float() + 1.5)
				case reflect.Int64: // time.Duration
					f.SetInt(f.Int() + int64(time.Second))
				}
			}
		}
		set(reflect.ValueOf(&c).Elem())
		c.Network = "testnet"
		roundTrip(t, c)
	})
}
