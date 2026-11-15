package relayer

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/gagliardetto/solana-go"
	"golang.org/x/sys/unix"
)

func loadPayer(path string) (solana.PrivateKey, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("payer key path must be absolute")
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open payer key: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect payer key: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || uint64(stat.Uid) != uint64(os.Getuid()) {
		return nil, fmt.Errorf("payer key must be an owner-only regular file")
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read payer key: %w", err)
	}
	var raw []json.RawMessage
	if err = json.Unmarshal(contents, &raw); err != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("payer key must be a 64-byte Solana keypair array")
	}
	key := make(solana.PrivateKey, ed25519.PrivateKeySize)
	for i, element := range raw {
		var number float64
		if len(element) == 0 || bytes.Equal(element, []byte("null")) || json.Unmarshal(element, &number) != nil || number < 0 || number >= 256 || number != float64(byte(number)) {
			return nil, fmt.Errorf("payer key must be a 64-byte Solana keypair array")
		}
		key[i] = byte(number)
	}
	derived := ed25519.NewKeyFromSeed(key[:ed25519.SeedSize])
	if !bytes.Equal(derived, key) {
		return nil, fmt.Errorf("payer key public half does not match its seed")
	}
	return key, nil
}

func createKey(path string) (solana.PublicKey, error) {
	if !filepath.IsAbs(path) {
		return solana.PublicKey{}, fmt.Errorf("key path must be absolute")
	}
	key, err := solana.NewRandomPrivateKey()
	if err != nil {
		return solana.PublicKey{}, err
	}
	values := make([]int, len(key))
	for i, value := range key {
		values[i] = int(value)
	}
	contents, err := json.Marshal(values)
	if err != nil {
		return solana.PublicKey{}, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("create payer key: %w", err)
	}
	defer file.Close()
	if _, err = file.Write(append(contents, '\n')); err != nil {
		return solana.PublicKey{}, err
	}
	if err = file.Sync(); err != nil {
		return solana.PublicKey{}, err
	}
	return key.PublicKey(), nil
}
