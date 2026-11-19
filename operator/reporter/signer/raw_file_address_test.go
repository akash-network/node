package signer_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdksecp "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/signer"
)

func TestRawFileAddressScalarAndFormatBoundaries(t *testing.T) {
	order := secp256k1.S256().N
	cases := []struct {
		name  string
		raw   string
		valid bool
	}{
		{
			name:  "one",
			raw:   fmt.Sprintf("%064x", big.NewInt(1)),
			valid: true,
		},
		{
			name:  "order minus one",
			raw:   fmt.Sprintf("%064x", new(big.Int).Sub(order, big.NewInt(1))),
			valid: true,
		},
		{
			name: "zero",
			raw:  strings.Repeat("0", 64),
		},
		{
			name: "order",
			raw:  fmt.Sprintf("%064x", order),
		},
		{
			name: "order plus one",
			raw:  fmt.Sprintf("%064x", new(big.Int).Add(order, big.NewInt(1))),
		},
		{
			name: "uppercase",
			raw:  strings.Repeat("A", 64),
		},
		{
			name: "short",
			raw:  strings.Repeat("1", 63),
		},
		{
			name: "long",
			raw:  strings.Repeat("1", 65),
		},
		{
			name: "mnemonic",
			raw:  "synthetic phrase must not be accepted or printed",
		},
		{
			name: "NUL",
			raw:  strings.Repeat("1", 64) + "\x00",
		},
		{
			name:  "size at limit",
			raw:   strings.Repeat(" ", 4096-64) + strings.Repeat("1", 64),
			valid: true,
		},
		{
			name: "size above limit",
			raw:  strings.Repeat(" ", 4097-64) + strings.Repeat("1", 64),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture.key")
			require.NoError(t, os.WriteFile(path, []byte(tc.raw), 0o600))
			address, err := signer.RawFileAddress(path)
			if !tc.valid {
				require.Error(t, err)
				require.Nil(t, address)
				require.False(t, strings.Contains(err.Error(), tc.raw), "private material leaked")
				return
			}

			require.NoError(t, err)
			decoded, err := hex.DecodeString(strings.TrimSpace(tc.raw))
			require.NoError(t, err)
			defer clear(decoded)
			key := &sdksecp.PrivKey{Key: decoded}
			require.True(t, bytes.Equal(address, key.PubKey().Address()))
			loaded, err := signer.NewRawFile(path, address)
			require.NoError(t, err)
			require.True(t, address.Equals(loaded.Address()))
		})
	}
}

func TestRawFileAddressECMAScriptTrim(t *testing.T) {
	key := sdksecp.GenPrivKeyFromSecret([]byte("R199 trim fixture"))
	defer clear(key.Key)
	raw := hex.EncodeToString(key.Key)
	address := sdk.AccAddress(key.PubKey().Address())
	accepted := "\t\n\v\f\r \u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"
	rejected := "\u0085\u180e\u200b\u2060\ufffd\x00"
	for _, cp := range accepted + rejected {
		t.Run(fmt.Sprintf("U+%04X", cp), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture.key")
			contents := string(cp) + raw + string(cp)
			require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
			got, err := signer.RawFileAddress(path)
			if strings.ContainsRune(accepted, cp) {
				require.NoError(t, err)
				require.True(t, got.Equals(address))
			} else {
				require.Error(t, err)
				require.Nil(t, got)
				require.False(t, strings.Contains(err.Error(), raw), "private material leaked")
			}
		})
	}
}
