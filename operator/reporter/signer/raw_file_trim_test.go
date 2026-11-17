package signer_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/signer"
)

// Expectations were observed through the actual TS loadSigner on 300 files:
// all 25 ECMAScript trim scalars, their neighbors, controls and format characters.
// TS key.ts SHA256: c6d7a5ec06e656cbcbd867fcf0960ce8408b6cde7e442a4bf0bc5aa94900c09f
// Oracle evidence SHA256: c6c30a7c17f0066bb98b0d06f76f9736dd9844b34cb743420ac6f1867d930cc4
func TestRawFileECMAScriptTrimOracle(t *testing.T) {
	_, address, err := bech32.DecodeAndConvert("akash1w508d6qejxtdg4y5r3zarvary0c5xw7khx6akz")
	require.NoError(t, err)
	const scalar1 = "0000000000000000000000000000000000000000000000000000000000000001"
	for _, tc := range []struct {
		codePoint    rune
		edgeAccepted bool
	}{
		{
			codePoint:    0x0,
			edgeAccepted: false,
		},
		{
			codePoint:    0x1,
			edgeAccepted: false,
		},
		{
			codePoint:    0x2,
			edgeAccepted: false,
		},
		{
			codePoint:    0x3,
			edgeAccepted: false,
		},
		{
			codePoint:    0x4,
			edgeAccepted: false,
		},
		{
			codePoint:    0x5,
			edgeAccepted: false,
		},
		{
			codePoint:    0x6,
			edgeAccepted: false,
		},
		{
			codePoint:    0x7,
			edgeAccepted: false,
		},
		{
			codePoint:    0x8,
			edgeAccepted: false,
		},
		{
			codePoint:    0x9,
			edgeAccepted: true,
		},
		{
			codePoint:    0xa,
			edgeAccepted: true,
		},
		{
			codePoint:    0xb,
			edgeAccepted: true,
		},
		{
			codePoint:    0xc,
			edgeAccepted: true,
		},
		{
			codePoint:    0xd,
			edgeAccepted: true,
		},
		{
			codePoint:    0xe,
			edgeAccepted: false,
		},
		{
			codePoint:    0xf,
			edgeAccepted: false,
		},
		{
			codePoint:    0x10,
			edgeAccepted: false,
		},
		{
			codePoint:    0x11,
			edgeAccepted: false,
		},
		{
			codePoint:    0x12,
			edgeAccepted: false,
		},
		{
			codePoint:    0x13,
			edgeAccepted: false,
		},
		{
			codePoint:    0x14,
			edgeAccepted: false,
		},
		{
			codePoint:    0x15,
			edgeAccepted: false,
		},
		{
			codePoint:    0x16,
			edgeAccepted: false,
		},
		{
			codePoint:    0x17,
			edgeAccepted: false,
		},
		{
			codePoint:    0x18,
			edgeAccepted: false,
		},
		{
			codePoint:    0x19,
			edgeAccepted: false,
		},
		{
			codePoint:    0x1a,
			edgeAccepted: false,
		},
		{
			codePoint:    0x1b,
			edgeAccepted: false,
		},
		{
			codePoint:    0x1c,
			edgeAccepted: false,
		},
		{
			codePoint:    0x1d,
			edgeAccepted: false,
		},
		{
			codePoint:    0x1e,
			edgeAccepted: false,
		},
		{
			codePoint:    0x1f,
			edgeAccepted: false,
		},
		{
			codePoint:    0x20,
			edgeAccepted: true,
		},
		{
			codePoint:    0x21,
			edgeAccepted: false,
		},
		{
			codePoint:    0x7f,
			edgeAccepted: false,
		},
		{
			codePoint:    0x85,
			edgeAccepted: false,
		},
		{
			codePoint:    0x9f,
			edgeAccepted: false,
		},
		{
			codePoint:    0xa0,
			edgeAccepted: true,
		},
		{
			codePoint:    0xa1,
			edgeAccepted: false,
		},
		{
			codePoint:    0x167f,
			edgeAccepted: false,
		},
		{
			codePoint:    0x1680,
			edgeAccepted: true,
		},
		{
			codePoint:    0x1681,
			edgeAccepted: false,
		},
		{
			codePoint:    0x180e,
			edgeAccepted: false,
		},
		{
			codePoint:    0x1fff,
			edgeAccepted: false,
		},
		{
			codePoint:    0x2000,
			edgeAccepted: true,
		},
		{
			codePoint:    0x2001,
			edgeAccepted: true,
		},
		{
			codePoint:    0x2002,
			edgeAccepted: true,
		},
		{
			codePoint:    0x2003,
			edgeAccepted: true,
		},
		{
			codePoint:    0x2004,
			edgeAccepted: true,
		},
		{
			codePoint:    0x2005,
			edgeAccepted: true,
		},
		{
			codePoint:    0x2006,
			edgeAccepted: true,
		},
		{
			codePoint:    0x2007,
			edgeAccepted: true,
		},
		{
			codePoint:    0x2008,
			edgeAccepted: true,
		},
		{
			codePoint:    0x2009,
			edgeAccepted: true,
		},
		{
			codePoint:    0x200a,
			edgeAccepted: true,
		},
		{
			codePoint:    0x200b,
			edgeAccepted: false,
		},
		{
			codePoint:    0x2027,
			edgeAccepted: false,
		},
		{
			codePoint:    0x2028,
			edgeAccepted: true,
		},
		{
			codePoint:    0x2029,
			edgeAccepted: true,
		},
		{
			codePoint:    0x202a,
			edgeAccepted: false,
		},
		{
			codePoint:    0x202e,
			edgeAccepted: false,
		},
		{
			codePoint:    0x202f,
			edgeAccepted: true,
		},
		{
			codePoint:    0x2030,
			edgeAccepted: false,
		},
		{
			codePoint:    0x205e,
			edgeAccepted: false,
		},
		{
			codePoint:    0x205f,
			edgeAccepted: true,
		},
		{
			codePoint:    0x2060,
			edgeAccepted: false,
		},
		{
			codePoint:    0x2fff,
			edgeAccepted: false,
		},
		{
			codePoint:    0x3000,
			edgeAccepted: true,
		},
		{
			codePoint:    0x3001,
			edgeAccepted: false,
		},
		{
			codePoint:    0xfefe,
			edgeAccepted: false,
		},
		{
			codePoint:    0xfeff,
			edgeAccepted: true,
		},
		{
			codePoint:    0xff00,
			edgeAccepted: false,
		},
		{
			codePoint:    0xfffd,
			edgeAccepted: false,
		},
		{
			codePoint:    0x1f600,
			edgeAccepted: false,
		},
		{
			codePoint:    0x10ffff,
			edgeAccepted: false,
		},
	} {
		for _, position := range []string{"leading", "trailing", "both", "interior"} {
			t.Run(fmt.Sprintf("U+%04X/%s", tc.codePoint, position), func(t *testing.T) {
				pad := string(tc.codePoint)
				var raw string
				switch position {
				case "leading":
					raw = pad + scalar1
				case "trailing":
					raw = scalar1 + pad
				case "both":
					raw = pad + scalar1 + pad
				case "interior":
					raw = scalar1[:32] + pad + scalar1[32:]
				}
				path := filepath.Join(t.TempDir(), "synthetic.key")
				require.NoError(t, os.WriteFile(path, []byte(raw), 0600))
				s, err := signer.NewRawFile(path, sdk.AccAddress(address))
				if tc.edgeAccepted && position != "interior" {
					require.NoError(t, err)
					require.Equal(t, sdk.AccAddress(address), s.Address())
				} else {
					require.Nil(t, s)
					require.EqualError(t, err, "key file must contain a 32-byte lowercase hex secp256k1 key")
				}
				after, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, raw, string(after))
			})
		}
	}
}

func TestRawFileTrimKeepsScalarAndFileBoundaries(t *testing.T) {
	for _, tc := range []struct {
		raw       string
		wantError string
	}{
		{
			raw:       strings.Repeat("0", 64),
			wantError: "invalid secp256k1 private scalar",
		},
		{
			raw:       "fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141",
			wantError: "invalid secp256k1 private scalar",
		},
		{
			raw:       strings.Repeat("f", 64),
			wantError: "invalid secp256k1 private scalar",
		},
		{
			raw:       strings.Repeat("A", 64),
			wantError: "key file must contain a 32-byte lowercase hex secp256k1 key",
		},
		{
			raw:       strings.Repeat("1", 63),
			wantError: "key file must contain a 32-byte lowercase hex secp256k1 key",
		},
		{
			raw:       strings.Repeat("1", 65),
			wantError: "key file must contain a 32-byte lowercase hex secp256k1 key",
		},
	} {
		path := filepath.Join(t.TempDir(), "synthetic.key")
		require.NoError(t, os.WriteFile(path, []byte("\ufeff"+tc.raw+"\ufeff"), 0600))
		_, err := signer.NewRawFile(path, sdk.AccAddress(make([]byte, 20)))
		require.EqualError(t, err, tc.wantError)
		require.NotContains(t, err.Error(), tc.raw)
	}
}
