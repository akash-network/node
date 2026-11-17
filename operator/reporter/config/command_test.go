package config_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func commandJSON(t *testing.T) map[string]any {
	t.Helper()
	input := legacyJSON(t)
	delete(input, "keyFile")
	key := secp256k1.GenPrivKeyFromSecret([]byte("R196 config fixture"))
	input["reporter"] = sdk.AccAddress(key.PubKey().Address()).String()
	input["externalSigner"] = map[string]any{
		"command":         "/nonexistent/r196-command",
		"args":            []string{"first", "literal;$(not-a-shell)"},
		"publicKeyBase64": base64.StdEncoding.EncodeToString(key.PubKey().Bytes()),
	}
	return input
}

func TestLegacyExternalSigner(t *testing.T) {
	for _, configured := range []bool{false, true} {
		for _, broadcast := range []bool{false, true} {
			input := commandJSON(t)
			if !configured {
				delete(input["externalSigner"].(map[string]any), "publicKeyBase64")
			}
			got, err := readLegacy(t, input, broadcast)
			require.NoError(t, err)
			require.NotNil(t, got.Legacy.ExternalSigner)
			require.Equal(t, "/nonexistent/r196-command", got.Legacy.ExternalSigner.Options.Command)
			require.Equal(t, []string{"first", "literal;$(not-a-shell)"}, got.Legacy.ExternalSigner.Options.Args)
			require.Equal(t, 10*time.Second, got.Legacy.ExternalSigner.Options.Timeout)
			require.Equal(t, configured, got.Legacy.ExternalSigner.PublicKey != nil)
			require.Equal(t, broadcast, got.Broadcast)
			require.Equal(t, "5000uakt", got.Fees.String())
			require.Equal(t, uint64(200000), got.GasPerMessage)
			require.Equal(t, 1, got.Tx.MaxMsgs)
		}
	}
}

func TestLegacyExternalSignerFields(t *testing.T) {
	cases := []struct {
		name  string
		field string
		value any
	}{
		{
			name:  "relative",
			field: "command",
			value: "relative",
		},
		{
			name:  "missing command",
			field: "command",
			value: nil,
		},
		{
			name:  "unknown",
			field: "unknown",
			value: true,
		},
		{
			name:  "cased command",
			field: "Command",
			value: "/fake",
		},
		{
			name:  "args object",
			field: "args",
			value: map[string]string{},
		},
		{
			name:  "args count",
			field: "args",
			value: make([]string, 33),
		},
		{
			name:  "null arg",
			field: "args",
			value: []any{nil},
		},
		{
			name:  "number arg",
			field: "args",
			value: []any{1},
		},
		{
			name:  "zero timeout",
			field: "timeoutSeconds",
			value: 0,
		},
		{
			name:  "negative timeout",
			field: "timeoutSeconds",
			value: -1,
		},
		{
			name:  "fraction timeout",
			field: "timeoutSeconds",
			value: 1.5,
		},
		{
			name:  "string timeout",
			field: "timeoutSeconds",
			value: "10",
		},
		{
			name:  "long timeout",
			field: "timeoutSeconds",
			value: 121,
		},
		{
			name:  "null pubkey",
			field: "publicKeyBase64",
			value: nil,
		},
		{
			name:  "bad pubkey",
			field: "publicKeyBase64",
			value: "!",
		},
		{
			name:  "empty pubkey",
			field: "publicKeyBase64",
			value: "",
		},
		{
			name:  "invalid point",
			field: "publicKeyBase64",
			value: base64.StdEncoding.EncodeToString(make([]byte, 33)),
		},
		{
			name:  "wrong reporter",
			field: "publicKeyBase64",
			value: base64.StdEncoding.EncodeToString(secp256k1.GenPrivKey().PubKey().Bytes()),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := commandJSON(t)
			input["externalSigner"].(map[string]any)[tc.field] = tc.value
			_, err := readLegacy(t, input, false)
			require.Error(t, err)
		})
	}
	for _, value := range []any{nil, "command", []string{}} {
		input := commandJSON(t)
		input["externalSigner"] = value
		_, err := readLegacy(t, input, false)
		require.Error(t, err)
	}
	input := commandJSON(t)
	ext := input["externalSigner"].(map[string]any)
	ext["args"], ext["timeoutSeconds"] = nil, nil
	got, err := readLegacy(t, input, false)
	require.NoError(t, err)
	require.Empty(t, got.Legacy.ExternalSigner.Options.Args)
	require.Equal(t, 10*time.Second, got.Legacy.ExternalSigner.Options.Timeout)
	data, err := json.Marshal(input)
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), `"command":`, `"command":"/first","command":`, 1))
	_, err = readLegacyBytes(t, data, false)
	require.ErrorContains(t, err, "duplicate")
}

func TestLegacyCommandSafetyBudget(t *testing.T) {
	for _, timeout := range []int{1, 10, 79, 80, 120} {
		for _, age := range []int{1, 60, 70, 71, 79, 80, 600} {
			for _, broadcast := range []bool{false, true} {
				input := commandJSON(t)
				input["maxAgeSeconds"] = age
				input["externalSigner"].(map[string]any)["timeoutSeconds"] = timeout
				_, err := readLegacy(t, input, broadcast)
				if broadcast && age+timeout > 80 {
					require.ErrorContains(t, err, "plus externalSigner.timeoutSeconds")
				} else {
					require.NoError(t, err)
				}
			}
		}
	}
}
