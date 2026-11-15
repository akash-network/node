package relayer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func configExample() map[string]any {
	return map[string]any{
		"akashRpc":          "http://127.0.0.1:26657",
		"akashChainId":      "akashnet-2",
		"solanaRpc":         "http://127.0.0.1:8899",
		"solanaGenesisHash": mainnetGenesis,
		"programId":         "98inUHb5mMaHWQZ9ut9PGrnmkMpncLZWWAyFwxCWagME",
		"payerKeyFile":      "/run/keys/key",
		"stateDir":          "/data",
	}
}

func parseTestConfig(t *testing.T, values map[string]any) (config, error) {
	t.Helper()
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := decodeConfigFields(data)
	if err != nil {
		return config{}, err
	}
	return parseConfig(fields, "/opt/akt")
}

func TestConfigDefaultsAndLegacyEncoder(t *testing.T) {
	values := configExample()
	values["headerEncoder"] = "/does/not/exist/old-encoder"
	cfg, err := parseTestConfig(t, values)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.maxLamports != 5_000_000 || cfg.maxSignatures != 100 || cfg.refreshFraction != 0.2 || cfg.retryMaxSeconds != 900 || cfg.pruneIntervalSeconds != 600 || cfg.maxAge != time.Minute || cfg.poll != 3*time.Second {
		t.Fatalf("configuration defaults changed: %+v", cfg)
	}
	if len(cfg.notices) != 1 || cfg.notices[0] != encoderNotice {
		t.Fatal("legacy encoder must produce a notice")
	}
	delete(values, "headerEncoder")
	cfg, err = parseTestConfig(t, values)
	if err != nil || len(cfg.notices) != 0 {
		t.Fatalf("built-in encoding should need no path: %v", err)
	}
}

func TestConfigRPCPortBounds(t *testing.T) {
	for _, field := range []string{"akashRpc", "solanaRpc"} {
		for _, endpoint := range []string{"http://localhost:65536", "http://localhost:99999", "https://localhost:99999/rpc"} {
			values := configExample()
			values[field] = endpoint
			if _, err := parseTestConfig(t, values); err == nil {
				t.Fatalf("%s accepted impossible port: %s", field, endpoint)
			}
		}
		for _, endpoint := range []string{"http://localhost", "http://localhost:0", "http://localhost:65535", "http://[::1]:8899"} {
			values := configExample()
			values[field] = endpoint
			if _, err := parseTestConfig(t, values); err != nil {
				t.Fatalf("%s rejected valid endpoint %s: %v", field, endpoint, err)
			}
		}
	}
}

func TestSourceConfigKeepsRawPathSemantics(t *testing.T) {
	root := durableTestDirectory(t)
	values := configExample()
	values["akashChainId"] = "custom-development-1"
	values["programId"] = developmentProgram
	values["payerKeyFile"] = "payer.json"
	values["stateDir"] = ".cache/state"
	values["headerEncoder"] = "old-encoder"
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadSourceConfig("config.json", root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.payerPath != filepath.Join(root, "payer.json") || cfg.stateDir != filepath.Join(root, ".cache/state") || cfg.local {
		t.Fatal("source config lost relative paths or enabled local bootstrap")
	}
	if _, err := loadConfig(path, "mainnet", "/opt/akt"); err == nil {
		t.Fatal("source config bypassed public service validation")
	}
}

func TestConfigRejectsMalformedBoundaries(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{
			name:  "unknown",
			value: true,
		},
		{
			name:  "headerEncoder",
			value: nil,
		},
		{
			name:  "headerEncoder",
			value: "",
		},
		{
			name:  "maxLamportsPerTransfer",
			value: "5000000",
		},
		{
			name:  "maxLamportsPerTransfer",
			value: 0,
		},
		{
			name:  "maxHeaderSignatures",
			value: 129,
		},
		{
			name:  "maxHeaderSignatures",
			value: 2.5,
		},
		{
			name:  "refreshFraction",
			value: 1,
		},
		{
			name:  "refreshFraction",
			value: 0,
		},
		{
			name:  "retryMaxSeconds",
			value: 5,
		},
		{
			name:  "pruneIntervalSeconds",
			value: 86_401,
		},
		{
			name:  "pollSeconds",
			value: nil,
		},
		{
			name:  "maxAgeSeconds",
			value: 9,
		},
		{
			name:  "akashChainId",
			value: strings.Repeat("x", 65),
		},
		{
			name:  "solanaGenesisHash",
			value: "invalid",
		},
		{
			name:  "programId",
			value: "invalid",
		},
		{
			name:  "akashRpc",
			value: "file:///etc/passwd",
		},
		{
			name:  "solanaRpc",
			value: "https://example-user:example-password@localhost",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values := configExample()
			values[tc.name] = tc.value
			if _, err := parseTestConfig(t, values); err == nil {
				t.Fatal("accepted invalid config")
			} else if strings.Contains(err.Error(), "example-password") {
				t.Fatal("configuration error leaked a URL credential")
			}
		})
	}
	for _, raw := range []string{"null", "[]", "{} {}", "{", `{"pollSeconds":true}`} {
		fields, err := decodeConfigFields([]byte(raw))
		if err == nil {
			_, err = parseConfig(fields, "/opt/akt")
		}
		if err == nil {
			t.Fatalf("accepted invalid document %s", raw)
		}
	}
}

func TestServiceConfigIsOfflineAndPinsNetwork(t *testing.T) {
	dir := durableTestDirectory(t)
	path := filepath.Join(dir, "config.json")
	values := configExample()
	values["headerEncoder"] = "/nonexistent/encoder"
	write := func() {
		data, err := json.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if _, err := loadConfig(path, "mainnet", "/opt/akt"); err != nil {
		t.Fatalf("offline check of nonexistent keys/encoder and offline RPC failed: %v", err)
	}
	if _, err := loadConfig(path, "testnet", "/opt/akt"); err == nil {
		t.Fatal("mainnet accepted as testnet")
	}
	values["akashChainId"] = "local-test-1"
	values["solanaGenesisHash"] = developmentProgram
	values["programId"] = developmentProgram
	write()
	if _, err := loadConfig(path, "testnet", "/opt/akt"); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path, "mainnet", "/opt/akt"); err == nil {
		t.Fatal("local deployment accepted as mainnet")
	}
	for _, state := range []string{"/opt/akt/state", "/srv/.cache/state", "relative"} {
		values["stateDir"] = state
		write()
		if _, err := loadConfig(path, "testnet", "/opt/akt"); err == nil {
			t.Fatalf("unsafe persistent state location accepted: %s", state)
		}
	}
	values["stateDir"] = "/srv/relayer"
	values["headerEncoder"] = "relative-encoder"
	write()
	if _, err := loadConfig(path, "testnet", "/opt/akt"); err == nil {
		t.Fatal("relative legacy encoder path accepted")
	}
}

func TestLocalConfigurationPreservesUnboundedSafeSeconds(t *testing.T) {
	env := map[string]string{
		"AKT_LOCAL_PROFILE":                "testnetify",
		"AKT_LOCAL_RUN":                    "go-test",
		"AKT_RELAY_RETRY_MAX_SECONDS":      "9007199254740991",
		"AKT_RELAY_PRUNE_INTERVAL_SECONDS": "9007199254740991",
	}
	lookup := func(name string) (string, bool) {
		value, ok := env[name]
		return value, ok
	}
	cfg, err := localConfig(lookup, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.retryMaxSeconds != maxSafeInteger || cfg.pruneIntervalSeconds != maxSafeInteger || cfg.genesis != nil || !cfg.local {
		t.Fatal("local limits or network scope changed")
	}
	if cfg.chainID != "akash-bridge-v2-testnetify-go-test-1" || cfg.akashRPC != "http://127.0.0.1:27657" || cfg.solanaRPC != "http://127.0.0.1:18999" || cfg.stateDir != "/repo/.cache/solana/run/testnetify-stack-v2-go-test" {
		t.Fatal("local profile no longer matches existing stack")
	}
	env["AKT_RELAY_RETRY_MAX_SECONDS"] = ""
	if _, err := localConfig(lookup, "/repo"); err == nil {
		t.Fatal("blank env setting must not silently use default")
	}
	delete(env, "AKT_RELAY_RETRY_MAX_SECONDS")
	env["AKT_LOCAL_PROFILE"] = ""
	if _, err := localConfig(lookup, "/repo"); err == nil {
		t.Fatal("blank local profile accepted")
	}
}

func TestLocalNumberJavaScriptWhitespace(t *testing.T) {
	// Number(string) trims ECMAScript WhiteSpace and LineTerminator, including BOM.
	whitespace := []rune{
		'\t', '\n', '\v', '\f', '\r', ' ', '\u00a0', '\u1680',
		'\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005',
		'\u2006', '\u2007', '\u2008', '\u2009', '\u200a', '\u2028',
		'\u2029', '\u202f', '\u205f', '\u3000', '\ufeff',
	}
	for _, space := range whitespace {
		t.Run(fmt.Sprintf("U+%04X", space), func(t *testing.T) {
			for _, text := range []string{string(space) + "3000", "3000" + string(space), string(space) + "3000" + string(space)} {
				lookup := func(string) (string, bool) { return text, true }
				number, err := localNumber(lookup, "AKT_AKASH_RPC_PORT", 26657)
				if err != nil || number != 3000 {
					t.Fatalf("Number(%q): got %v, %v; want 3000", text, number, err)
				}
			}

			lookup := func(string) (string, bool) { return string(space), true }
			number, err := localNumber(lookup, "AKT_AKASH_RPC_PORT", 26657)
			if err != nil || number != 0 {
				t.Fatalf("Number(whitespace): got %v, %v; want 0", number, err)
			}
		})
	}
}

func TestLocalNumberRejectsNonJavaScriptSyntax(t *testing.T) {
	values := []string{
		"3_000", "0x_1000", "0x1_000", "0b1_0", "0o7_0", "3e_3",
		"+0x1p1", "-0x1p1", "3000x", "3 000", "NaN", "Infinity",
		"\u00853000", "3000\u0085", "\u0085", "\u001c3000",
		"\u180e3000", "\u200b3000", "3\ufeff000",
	}
	for _, text := range values {
		t.Run(fmt.Sprintf("%q", text), func(t *testing.T) {
			lookup := func(string) (string, bool) { return text, true }
			if _, err := localNumber(lookup, "AKT_AKASH_RPC_PORT", 26657); err == nil || err.Error() != "invalid AKT_AKASH_RPC_PORT" {
				t.Fatalf("invalid Number(%q): got %v", text, err)
			}
		})
	}
}

func TestLocalNumberKeepsJavaScriptNumericForms(t *testing.T) {
	for _, text := range []string{"3000", "+3000", "03000", "3e3", "0xbb8", "0o5670", "0b101110111000"} {
		lookup := func(string) (string, bool) { return text, true }
		number, err := localNumber(lookup, "AKT_AKASH_RPC_PORT", 26657)
		if err != nil || number != 3000 {
			t.Fatalf("Number(%q): got %v, %v; want 3000", text, number, err)
		}
	}

	missing := func(string) (string, bool) { return "", false }
	if number, err := localNumber(missing, "AKT_AKASH_RPC_PORT", 26657); err != nil || number != 26657 {
		t.Fatalf("missing value must use fallback: %v, %v", number, err)
	}
	blank := func(string) (string, bool) { return "", true }
	if number, err := localNumber(blank, "AKT_AKASH_RPC_PORT", 26657); err != nil || number != 0 {
		t.Fatalf("present blank must become zero: %v, %v", number, err)
	}
}

func TestLocalConfigurationUsesJavaScriptNumbers(t *testing.T) {
	env := map[string]string{
		"AKT_AKASH_RPC_PORT":                  "\ufeff3000\ufeff",
		"AKT_SOLANA_RPC_PORT":                 "\u20283001\u2029",
		"AKT_RELAY_REFRESH_FRACTION":          "\ufeff0.25\ufeff",
		"AKT_RELAY_RETRY_MAX_SECONDS":         "\ufeff900\ufeff",
		"AKT_RELAY_PRUNE_INTERVAL_SECONDS":    "\ufeff600\ufeff",
		"AKT_RELAY_MAX_LAMPORTS_PER_TRANSFER": "\ufeff5000000\ufeff",
		"AKT_RELAY_MAX_HEADER_SIGNATURES":     "\ufeff100\ufeff",
	}
	lookup := func(name string) (string, bool) {
		value, ok := env[name]
		return value, ok
	}
	cfg, err := localConfig(lookup, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.akashRPC != "http://127.0.0.1:3000" || cfg.solanaRPC != "http://127.0.0.1:3001" || cfg.refreshFraction != 0.25 || cfg.retryMaxSeconds != 900 || cfg.pruneIntervalSeconds != 600 || cfg.maxLamports != 5000000 || cfg.maxSignatures != 100 {
		t.Fatalf("local numeric configuration changed: %+v", cfg)
	}

	for name, valid := range env {
		env[name] = "1_000"
		if _, err := localConfig(lookup, "/repo"); err == nil {
			t.Errorf("accepted numeric separator in %s", name)
		}
		env[name] = valid
	}
}
