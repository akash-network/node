// Package config is the reporter's akash-style configuration: a TOML file at <home>/config/reporter.toml,
// AKT_REPORTER_* environment variables, flags and validation.
package config

import (
	"fmt"
	"reflect"
	"time"

	"github.com/spf13/pflag"
)

const (
	// MainnetChainID is the Akash mainnet chain ID that the mainnet network guard requires.
	MainnetChainID = "akashnet-2"
	// MainnetGenesis is the Solana mainnet genesis hash that the mainnet network guard requires.
	MainnetGenesis = "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d"
	// DevProgramID is the development program ID that the mainnet network guard refuses.
	DevProgramID = "FfFAPUKqjf68ggj1HvpyzMHFDRnT2cFAdZYYQb7mK9i5"

	// EnvPrefix is the prefix of every environment variable the reporter reads.
	EnvPrefix = "AKT_REPORTER"
	// FlagHome is the name of the home directory flag. It is not a configuration key.
	FlagHome = "home"
)

// Config mirrors the keys of spec section 8.3. The mapstructure tags are the key names, so a key is the dotted path of its tags.
type Config struct {
	Legacy *Legacy `mapstructure:"-"`

	Network        string        `mapstructure:"network"`
	Broadcast      bool          `mapstructure:"broadcast"`
	Reporter       string        `mapstructure:"reporter"`
	ChainID        string        `mapstructure:"chain-id"`
	Node           string        `mapstructure:"node"`
	GRPCAddr       string        `mapstructure:"grpc-addr"`
	GRPCInsecure   bool          `mapstructure:"grpc-insecure"`
	MaxAge         time.Duration `mapstructure:"max-age"`
	RequestTimeout time.Duration `mapstructure:"request-timeout"`
	From           string        `mapstructure:"from"`
	KeyringBackend string        `mapstructure:"keyring-backend"`
	KeyringDir     string        `mapstructure:"keyring-dir"`
	Gas            string        `mapstructure:"gas"`
	GasAdjustment  float64       `mapstructure:"gas-adjustment"`
	GasPrices      string        `mapstructure:"gas-prices"`
	Fees           string        `mapstructure:"fees"`

	Solana  Solana  `mapstructure:"solana"`
	Scan    Scan    `mapstructure:"scan"`
	Tx      Tx      `mapstructure:"tx"`
	Signer  Signer  `mapstructure:"signer"`
	Metrics Metrics `mapstructure:"metrics"`
}

// Solana holds the solana.* keys.
type Solana struct {
	RPC                string `mapstructure:"rpc"`
	GenesisHash        string `mapstructure:"genesis-hash"`
	ProgramID          string `mapstructure:"program-id"`
	Mint               string `mapstructure:"mint"`
	LightClientProgram string `mapstructure:"light-client-program"`
	MaxSlotLead        uint64 `mapstructure:"max-slot-lead"`
}

// Scan holds the scan.* keys.
type Scan struct {
	Interval          time.Duration `mapstructure:"interval"`
	AuditPerScan      int           `mapstructure:"audit-per-scan"`
	PageSize          int           `mapstructure:"page-size"`
	Workers           int           `mapstructure:"workers"`
	AkashConcurrency  int           `mapstructure:"akash-concurrency"`
	SolanaConcurrency int           `mapstructure:"solana-concurrency"`
	BlockConcurrency  int           `mapstructure:"block-concurrency"`
}

// Tx holds the tx.* keys.
type Tx struct {
	MaxGas      uint64 `mapstructure:"max-gas"`
	MaxFee      string `mapstructure:"max-fee"`
	MaxMsgs     int    `mapstructure:"max-msgs"`
	MaxInFlight int    `mapstructure:"max-in-flight"`
}

// Signer holds the signer.* keys.
type Signer struct {
	Remote Remote `mapstructure:"remote"`
}

// Remote holds the signer.remote.* keys.
type Remote struct {
	Address    string        `mapstructure:"address"`
	KeyID      string        `mapstructure:"key-id"`
	CAFile     string        `mapstructure:"ca-file"`
	ServerName string        `mapstructure:"server-name"`
	CertFile   string        `mapstructure:"cert-file"`
	KeyFile    string        `mapstructure:"key-file"`
	Timeout    time.Duration `mapstructure:"timeout"`
}

// Metrics holds the metrics.* keys.
type Metrics struct {
	Listen string `mapstructure:"listen"`
}

// Default returns the defaults of spec section 8.3. Required keys are empty. KeyringDir is empty here and
// Load fills it with the home directory.
func Default() Config {
	return Config{
		Network:        networkMainnet,
		Node:           "tcp://localhost:26657",
		GRPCAddr:       "localhost:9090",
		MaxAge:         60 * time.Second,
		RequestTimeout: 15 * time.Second,
		KeyringBackend: "test",
		Gas:            "auto",
		GasAdjustment:  2.0,
		GasPrices:      "0.025uakt",
		Solana: Solana{
			RPC: "http://localhost:8899",
		},
		Scan: Scan{
			Interval:          15 * time.Second,
			AuditPerScan:      100,
			PageSize:          500,
			Workers:           16,
			AkashConcurrency:  32,
			SolanaConcurrency: 8,
			BlockConcurrency:  2,
		},
		Tx: Tx{
			MaxGas:      20_000_000,
			MaxFee:      "1000000uakt",
			MaxMsgs:     25,
			MaxInFlight: 8,
		},
		Signer: Signer{
			Remote: Remote{
				Timeout: 10 * time.Second,
			},
		},
	}
}

// usages is the one-line description of every key. It is the flag usage and the comment Render writes.
var usages = map[string]string{
	"network":                     "mainnet or testnet; each refuses the other's identities",
	"broadcast":                   "send transactions; false previews only",
	"reporter":                    "reporter akash1... address (required)",
	"chain-id":                    "Akash chain ID (required)",
	"node":                        "Akash RPC URL: tcp, http or https, no credentials",
	"grpc-addr":                   "Akash gRPC host:port",
	"grpc-insecure":               "use plaintext gRPC for a non-loopback address; loopback always uses plaintext",
	"max-age":                     "oldest accepted snapshot, above 0 and at most 10m; with broadcast, max-age plus the signer timeout is at most 80s",
	"request-timeout":             "per-request timeout, 1s to 60s",
	"from":                        "keyring key name or address that signs in-process",
	"keyring-backend":             "keyring backend: os, file or test",
	"keyring-dir":                 "keyring directory; empty means the home directory",
	"gas":                         "auto, or gas per message; the transaction limit is that times the message count, at most tx.max-gas",
	"gas-adjustment":              "simulation gas multiplier, 1.0 to 10.0",
	"gas-prices":                  "gas price in uakt, for example 0.025uakt",
	"fees":                        "fixed fee in uakt, at most tx.max-fee; overrides gas-prices",
	"solana.rpc":                  "Solana JSON-RPC URL: http or https, no credentials",
	"solana.genesis-hash":         "Solana genesis hash, canonical base58 (required)",
	"solana.program-id":           "sink program ID, canonical base58 (required)",
	"solana.mint":                 "mint address; must equal the program's \"mint\" PDA (required)",
	"solana.light-client-program": "light client program ID; empty follows the bridge account",
	"solana.max-slot-lead":        "slots the cursor may lead, at most 100000; 0 derives (max-age + 60s, rounded up to whole seconds) x 5",
	"scan.interval":               "wait between passes, 1s to 60s",
	"scan.audit-per-scan":         "transfer IDs the terminal audit rechecks per pass, 1 to 10000",
	"scan.page-size":              "page size for list requests, 1 to 5000",
	"scan.workers":                "pass workers, 1 to 256",
	"scan.akash-concurrency":      "concurrent Akash requests, 1 to 256",
	"scan.solana-concurrency":     "concurrent Solana requests, 1 to 64",
	"scan.block-concurrency":      "concurrent Solana block fetches, 1 to 16",
	"tx.max-gas":                  "gas limit cap for a transaction, 1 to 20000000",
	"tx.max-fee":                  "fee cap for a transaction in uakt, above 0, at most 1000000uakt",
	"tx.max-msgs":                 "messages per transaction, 1 to 100",
	"tx.max-in-flight":            "transactions in flight, 1 to 64",
	"signer.remote.address":       "remote signer host:port; set it or from when broadcasting, not both",
	"signer.remote.key-id":        "remote signer key ID; required with signer.remote.address",
	"signer.remote.ca-file":       "CA file for the remote signer; empty uses system roots",
	"signer.remote.server-name":   "TLS server name for the remote signer",
	"signer.remote.cert-file":     "client certificate for the remote signer; set with signer.remote.key-file",
	"signer.remote.key-file":      "client key for the remote signer; set with signer.remote.cert-file",
	"signer.remote.timeout":       "remote signer request timeout, 1s to 120s",
	"metrics.listen":              "metrics host:port; empty disables the listener",
}

// walk calls fn for every leaf field of the struct v with its dotted key.
func walk(v reflect.Value, prefix string, fn func(key string, field reflect.Value)) {
	t := v.Type()
	for i := range t.NumField() {
		key := t.Field(i).Tag.Get("mapstructure")
		if key == "-" {
			continue
		}
		if prefix != "" {
			key = prefix + "." + key
		}

		if f := v.Field(i); f.Kind() == reflect.Struct {
			walk(f, key, fn)
		} else {
			fn(key, f)
		}
	}
}

// Known returns every configuration key in declaration order.
func Known() []string {
	var keys []string

	walk(reflect.ValueOf(Default()), "", func(key string, _ reflect.Value) {
		keys = append(keys, key)
	})

	return keys
}

// AddFlags registers one flag per key, named by the dotted key, and the home flag unless it already exists.
func AddFlags(fs *pflag.FlagSet) {
	if fs.Lookup(FlagHome) == nil {
		fs.String(FlagHome, "", "home directory (default $HOME/.akt-reporter; env AKT_REPORTER_HOME)")
	}

	walk(reflect.ValueOf(Default()), "", func(key string, f reflect.Value) {
		usage := usages[key]

		switch def := f.Interface().(type) {
		case string:
			fs.String(key, def, usage)
		case bool:
			fs.Bool(key, def, usage)
		case time.Duration:
			fs.Duration(key, def, usage)
		case int:
			fs.Int(key, def, usage)
		case uint64:
			fs.Uint64(key, def, usage)
		case float64:
			fs.Float64(key, def, usage)
		default:
			panic(fmt.Sprintf("config: unsupported type %T for key %s", def, key))
		}
	})
}
