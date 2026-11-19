package signer

import (
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdksecp "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	"pkg.akt.dev/go/sdkutil"
)

const rawKeyLimit = 4096

// NewRawFile loads legacy raw32 custody into an ephemeral SDK keyring. It never
// imports into a persistent backend or changes the original file.
func NewRawFile(path string, reporter sdk.AccAddress) (Signer, error) {
	decoded, err := readRawKey(path)
	if err != nil {
		return nil, err
	}

	defer clear(decoded)
	kr := keyring.NewInMemory(sdkutil.MakeEncodingConfig().Codec)
	if kr.ImportPrivKeyHex("legacy", hex.EncodeToString(decoded), "secp256k1") != nil {
		return nil, errors.New("cannot import raw key into ephemeral keyring")
	}

	return NewKeyring(kr, "legacy", reporter)
}

// RawFileAddress derives an address using the same custody checks as NewRawFile,
// without importing the key into a keyring or retaining it for signing.
func RawFileAddress(path string) (sdk.AccAddress, error) {
	decoded, err := readRawKey(path)
	if err != nil {
		return nil, err
	}

	defer clear(decoded)
	key := &sdksecp.PrivKey{Key: decoded}

	return sdk.AccAddress(key.PubKey().Address()), nil
}

func readRawKey(path string) ([]byte, error) {
	// O_NONBLOCK prevents a FIFO from hanging before fstat can reject it.
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, errors.New("cannot open raw signing key without following symlinks")
	}

	file := os.NewFile(uintptr(fd), "raw signing key")
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, errors.New("cannot inspect raw signing key")
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || stat.Uid != uint32(os.Geteuid()) {
		return nil, errors.New("signing key must be a caller-owned owner-only regular file")
	}

	data, err := io.ReadAll(io.LimitReader(file, rawKeyLimit+1))
	if err != nil || len(data) > rawKeyLimit {
		return nil, errors.New("raw signing key is unreadable or exceeds 4096 bytes")
	}

	defer clear(data)
	// ECMAScript trim includes BOM and excludes U+0085, unlike strings.TrimSpace.
	raw := strings.Trim(string(data), "\t\n\v\f\r \u00a0\u1680"+
		"\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a"+
		"\u2028\u2029\u202f\u205f\u3000\ufeff")
	if len(raw) != 64 || strings.Trim(raw, "0123456789abcdef") != "" {
		return nil, errors.New("key file must contain a 32-byte lowercase hex secp256k1 key")
	}

	decoded, err := hex.DecodeString(raw)
	if err != nil {
		return nil, errors.New("invalid raw signing key")
	}

	var scalar secp256k1.ModNScalar
	defer scalar.Zero()
	if scalar.SetByteSlice(decoded) || scalar.IsZero() {
		clear(decoded)
		return nil, errors.New("invalid secp256k1 private scalar")
	}

	return decoded, nil
}
