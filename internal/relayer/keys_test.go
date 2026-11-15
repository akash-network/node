package relayer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestKeyCreationAndProtectedLoading(t *testing.T) {
	dir := durableTestDirectory(t)
	path := filepath.Join(dir, "test-key.json")
	address, err := createKey(path)
	if err != nil {
		t.Fatal(err)
	}
	key, err := loadPayer(path)
	if err != nil || key.PublicKey() != address {
		t.Fatalf("created key did not load: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("created key permissions changed")
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := createKey(path); err == nil {
		t.Fatal("existing key was overwritten")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("existing key contents changed")
	}
	link := filepath.Join(dir, "key-link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPayer(link); err == nil {
		t.Fatal("key symlink followed")
	}
	if _, err := createKey(link); err == nil {
		t.Fatal("existing symlink replaced")
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPayer(path); err == nil {
		t.Fatal("group-readable key accepted")
	}
	if _, err := loadPayer(dir); err == nil {
		t.Fatal("directory accepted as key")
	}
	pipe := filepath.Join(dir, "key-pipe")
	if err := unix.Mkfifo(pipe, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPayer(pipe); err == nil {
		t.Fatal("FIFO accepted as key")
	}
}

func TestKeyRequiresByteArrayAndMatchingPublicHalf(t *testing.T) {
	dir := durableTestDirectory(t)
	path := filepath.Join(dir, "test-key.json")
	if _, err := createKey(path); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var values []any
	if err := json.Unmarshal(contents, &values); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []any{nil, "0", true, -1, 256, 1.5} {
		clone := append([]any(nil), values...)
		clone[0] = invalid
		data, err := json.Marshal(clone)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadPayer(path); err == nil {
			t.Fatal("invalid array element accepted")
		}
	}
	values[63] = float64((int(values[63].(float64)) + 1) % 256)
	data, _ := json.Marshal(values)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPayer(path); err == nil {
		t.Fatal("mismatched public half accepted")
	}
	for _, value := range []any{"base64 is not a Solana keypair array", nil, []int{1, 2, 3}} {
		data, _ := json.Marshal(value)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadPayer(path); err == nil {
			t.Fatal("invalid key document accepted")
		}
	}
}
