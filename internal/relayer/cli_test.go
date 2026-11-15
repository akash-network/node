package relayer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineCommandDoesNotReadKeyOrContactRPC(t *testing.T) {
	root := durableTestDirectory(t)
	config := configExample()
	config["akashRpc"] = "http://127.0.0.1:1"
	config["solanaRpc"] = "http://127.0.0.1:1"
	config["payerKeyFile"] = filepath.Join(root, "does-not-exist")
	config["headerEncoder"] = filepath.Join(root, "also-does-not-exist")
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd, err := parseCommand([]string{"check", "--config", path})
	if err != nil {
		t.Fatal(err)
	}
	var output, warnings bytes.Buffer
	handled, err := runOffline(cmd, BuildInfo{}, Streams{
		Out: &output,
		Err: &warnings,
	}, "/opt/akt")
	if err != nil || !handled || output.String() != "Configuration valid. No RPC request or transaction sent.\n" {
		t.Fatalf("offline check: handled=%v err=%v output=%q", handled, err, output.String())
	}
	if !strings.Contains(warnings.String(), encoderNotice) {
		t.Fatal("missing legacy encoder notice")
	}
}

func TestHeaderCommandMatchesActualRustPacket(t *testing.T) {
	input, err := os.ReadFile("header/testdata/normal-mixed.input.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("header/testdata/normal-mixed.output.json")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = encodeHeaderCommand(nil, Streams{
		In:  bytes.NewReader(input),
		Out: &output,
	})
	if err != nil {
		t.Fatal(err)
	}
	var gotJSON, wantJSON any
	if err := json.Unmarshal(output.Bytes(), &gotJSON); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &wantJSON); err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(gotJSON)
	expected, _ := json.Marshal(wantJSON)
	if !bytes.Equal(got, expected) {
		t.Fatal("CLI packet differs from the Rust oracle")
	}
}

func TestHeaderCommandUsesRustInputBound(t *testing.T) {
	input, err := os.ReadFile("header/testdata/normal-mixed.input.json")
	if err != nil {
		t.Fatal(err)
	}
	const bound = 4 * 1024 * 1024
	for _, prefix := range []int{bound - len(input), bound} {
		reader := strings.NewReader(strings.Repeat(" ", prefix) + string(input))
		var output bytes.Buffer
		err := encodeHeaderCommand(nil, Streams{
			In:  reader,
			Out: &output,
		})
		if prefix == bound {
			if err == nil || output.Len() != 0 || reader.Len() != len(input) {
				t.Fatalf("oversized input was consumed or accepted: err=%v, unread=%d", err, reader.Len())
			}
		} else if err != nil || output.Len() == 0 || reader.Len() != 0 {
			t.Fatalf("exact-bound input rejected: err=%v, unread=%d", err, reader.Len())
		}
	}
}

func TestSourceConfigCommandSurface(t *testing.T) {
	for _, args := range [][]string{
		{"local", "--config", "relative.json"},
		{"local", "run", "--config", "relative.json", "--watch"},
		{"local", "--config", "relative.json", "--reset-budget", "all"},
	} {
		cmd, err := parseCommand(args)
		if err != nil || cmd.name != "local" || cmd.configPath != "relative.json" {
			t.Fatalf("source config rejected: %v, %v", args, err)
		}
	}
	for _, args := range [][]string{
		{"local", "--config", ""},
		{"local", "--config", "relative.json", "--bootstrap"},
		{"local", "--config", "relative.json", "--check-invalid-header"},
		{"local", "bootstrap", "--config", "relative.json"},
		{"local", "update", "42", "--config", "relative.json", "--tamper"},
		{"local", "--config", "relative.json", "--bootstrap", "--reset-budget", "all"},
	} {
		if _, err := parseCommand(args); err == nil {
			t.Fatalf("configured diagnostic accepted: %v", args)
		}
	}
}

func TestCommandSurface(t *testing.T) {
	cases := []struct {
		args []string
		name string
	}{
		{
			args: nil,
			name: "help",
		},
		{
			args: []string{"--help"},
			name: "help",
		},
		{
			args: []string{"--version"},
			name: "version",
		},
		{
			args: []string{"keys", "create", "--out", "/test-key"},
			name: "keys-create",
		},
		{
			args: []string{"--key-file", "/test-key", "keys", "address"},
			name: "keys-address",
		},
		{
			args: []string{"check", "--config", "/config.json"},
			name: "check",
		},
		{
			args: []string{"run", "--config", "/config.json", "--watch"},
			name: "run",
		},
		{
			args: []string{"run", "--config", "/config.json", "--reset-budget", "all", "--network", "testnet"},
			name: "run",
		},
		{
			args: []string{"local", "--bootstrap", "--state", "ignored"},
			name: "local",
		},
		{
			args: []string{"header-encode", "--fixture", "100", "40", "weighted"},
			name: "header-encode",
		},
	}
	for _, tc := range cases {
		command, err := parseCommand(tc.args)
		if err != nil || command.name != tc.name {
			t.Fatalf("%v: command %s, error %v", tc.args, command.name, err)
		}
	}
}

func TestCommandRejectsUnsafeCombinations(t *testing.T) {
	for _, args := range [][]string{
		{"--unknown"},
		{"--network", "devnet", "--help"},
		{"run"},
		{"run", "--config", "relative"},
		{"run", "extra", "--config", "/config"},
		{"run", "--config", "/config", "--broadcast"},
		{"run", "--config", "/config", "--key-file", "/key"},
		{"check", "--config", "/config", "--watch"},
		{"check", "--config", "/config", "--reset-budget", "all"},
		{"run", "--config", "/config", "--watch", "--reset-budget", "all"},
		{"keys", "create"},
		{"keys", "create", "--out", "/key", "--watch"},
		{"keys", "address", "--key-file", "/key", "--out", "/other"},
		{"header-encode", "--fixture", "257", "1"},
		{"header-encode", "--fixture", "4", "5"},
	} {
		if _, err := parseCommand(args); err == nil {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
}
