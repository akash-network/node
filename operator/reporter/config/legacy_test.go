package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/config"
)

func legacyJSON(t *testing.T) map[string]any {
	t.Helper()
	c := valid(t)
	return map[string]any{
		"akashRpc":          "http://127.0.0.1:26657/native%2Frpc?project=r176&n=1",
		"solanaRpc":         "http://127.0.0.1:8899/solana?project=r176",
		"akashChainId":      c.ChainID,
		"reporter":          c.Reporter,
		"solanaGenesisHash": c.Solana.GenesisHash,
		"programId":         c.Solana.ProgramID,
		"mint":              c.Solana.Mint,
		"maxAgeSeconds":     60,
		"pollSeconds":       3,
		"gasLimit":          "200000",
		"feeAmountUakt":     "5000",
		"keyFile":           "/nonexistent/r176/raw.key",
	}
}

func readLegacy(t *testing.T, input any, broadcast bool) (config.Resolved, error) {
	t.Helper()
	data, err := json.Marshal(input)
	require.NoError(t, err)
	return readLegacyBytes(t, data, broadcast)
}

func readLegacyBytes(t *testing.T, data []byte, broadcast bool) (config.Resolved, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, data, 0600))
	cfg, err := config.LoadLegacy(path, "testnet", broadcast)
	if err != nil {
		return config.Resolved{}, err
	}

	return config.Resolve(cfg)
}

func TestLegacyMapsExistingSettings(t *testing.T) {
	input := legacyJSON(t)
	c, err := readLegacy(t, input, true)
	require.NoError(t, err)
	require.Equal(t, input["akashRpc"], c.Node)
	require.Equal(t, input["solanaRpc"], c.Solana.RPC)
	require.Equal(t, input["akashChainId"], c.ChainID)
	require.Equal(t, input["reporter"], c.Config.Reporter)
	require.Equal(t, input["solanaGenesisHash"], c.Solana.GenesisHash)
	require.Equal(t, input["programId"], c.ProgramID.String())
	require.Equal(t, input["mint"], c.Mint.String())
	require.Equal(t, input["keyFile"], c.Legacy.KeyFile)
	require.Empty(t, c.GRPCAddr)
	require.Empty(t, c.From)
	require.Empty(t, c.GasPrices)
	require.Equal(t, "5000uakt", c.Fees.String())
	require.Equal(t, uint64(200000), c.GasPerMessage)
	require.Equal(t, 1, c.Tx.MaxMsgs)
	require.Equal(t, 500, c.Scan.PageSize)
	require.Equal(t, 100, c.Scan.AuditPerScan)
	require.Equal(t, 3*time.Second, c.Scan.Interval)
	require.Equal(t, 60*time.Second, c.MaxAge)
	require.Equal(t, uint64(600), c.SlotLead)
	require.Nil(t, c.LightClientPin)
	input["lightClientProgram"] = input["programId"]
	input["maxSlotLead"] = 17
	input["auditPerScan"] = 42
	c, err = readLegacy(t, input, false)
	require.NoError(t, err)
	require.Equal(t, input["programId"], c.LightClientPin.String())
	require.Equal(t, uint64(17), c.SlotLead)
	require.Equal(t, 42, c.Scan.AuditPerScan)
}

func TestLegacyRejectsMalformedOrUnsupported(t *testing.T) {
	cases := []struct {
		name      string
		change    func(map[string]any)
		broadcast bool
		text      string
	}{
		{
			name: "unknown",
			change: func(m map[string]any) {
				m["rpc"] = "http://127.0.0.1"
			},
			text: "unknown",
		},
		{
			name: "case folded",
			change: func(m map[string]any) {
				m["AkashRpc"] = m["akashRpc"]
				delete(m, "akashRpc")
			},
			text: "unknown",
		},
		{
			name: "missing",
			change: func(m map[string]any) {
				delete(m, "maxAgeSeconds")
			},
			text: "missing",
		},
		{
			name: "null",
			change: func(m map[string]any) {
				m["maxSlotLead"] = nil
			},
			text: "null",
		},
		{
			name: "fraction",
			change: func(m map[string]any) {
				m["pollSeconds"] = 1.2
			},
			text: "pollSeconds",
		},
		{
			name: "quoted age",
			change: func(m map[string]any) {
				m["maxAgeSeconds"] = "60"
			},
			text: "maxAgeSeconds",
		},
		{
			name: "overflow",
			change: func(m map[string]any) {
				m["maxAgeSeconds"] = 1e30
			},
			text: "maxAgeSeconds",
		},
		{
			name: "zero lead",
			change: func(m map[string]any) {
				m["maxSlotLead"] = 0
			},
			text: "maxSlotLead",
		},
		{
			name: "fee cap",
			change: func(m map[string]any) {
				m["feeAmountUakt"] = "1000001"
			},
			text: "tx.max-fee",
		},
		{
			name: "number fee",
			change: func(m map[string]any) {
				m["feeAmountUakt"] = 5000
			},
			text: "strings",
		},
		{
			name: "gas cap",
			change: func(m map[string]any) {
				m["gasLimit"] = "20000001"
			},
			text: "tx.max-gas",
		},
		{
			name: "priced gas",
			change: func(m map[string]any) {
				m["gasLimit"] = "auto"
			},
			text: "decimal",
		},
		{
			name: "leading zero fee",
			change: func(m map[string]any) {
				m["feeAmountUakt"] = "05000"
			},
			text: "decimal",
		},
		{
			name: "long broadcast age",
			change: func(m map[string]any) {
				m["maxAgeSeconds"] = 81
			},
			broadcast: true,
			text:      "80",
		},
		{
			name: "too old preview",
			change: func(m map[string]any) {
				m["maxAgeSeconds"] = 601
			},
			text: "maxAgeSeconds",
		},
		{
			name: "missing custody",
			change: func(m map[string]any) {
				delete(m, "keyFile")
			},
			broadcast: true,
			text:      "keyFile",
		},
		{
			name: "relative key",
			change: func(m map[string]any) {
				m["keyFile"] = "raw.key"
			},
			text: "absolute",
		},
		{
			name: "conflicting custody",
			change: func(m map[string]any) {
				m["externalSigner"] = map[string]any{"command": "/r176/signer"}
			},
			text: "not both",
		},
		{
			name: "credentials",
			change: func(m map[string]any) {
				m["akashRpc"] = "http://user:SECRET@127.0.0.1"
			},
			text: "without credentials",
		},
		{
			name: "broken URL",
			change: func(m map[string]any) {
				m["akashRpc"] = "http://SECRET:%Q/"
			},
			text: "without credentials",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := legacyJSON(t)
			tc.change(input)
			_, err := readLegacy(t, input, tc.broadcast)
			require.ErrorContains(t, err, tc.text)
			require.NotContains(t, err.Error(), "SECRET")
		})
	}
}

func TestLegacyStrictJSONAndModes(t *testing.T) {
	input := legacyJSON(t)
	data, err := json.Marshal(input)
	require.NoError(t, err)
	for _, bad := range []string{"null", "[]", "{", string(data) + " {}", strings.Replace(string(data), "{", "{\"akashRpc\":\"http://127.0.0.1\",", 1)} {
		_, err := readLegacyBytes(t, []byte(bad), false)
		require.Error(t, err)
	}

	input["maxAgeSeconds"] = 600
	delete(input, "keyFile")
	c, err := readLegacy(t, input, false)
	require.NoError(t, err)
	require.Equal(t, 10*time.Minute, c.MaxAge)
	input["maxAgeSeconds"] = 80
	input["keyFile"] = "/nonexistent/r176/raw.key"
	c, err = readLegacy(t, input, true)
	require.NoError(t, err)
	require.Equal(t, 80*time.Second, c.MaxAge)
	c.From = "another"
	_, err = config.Resolve(c.Config)
	require.ErrorContains(t, err, "custody")
	_, err = config.LoadLegacy("relative.json", "testnet", false)
	require.ErrorContains(t, err, "absolute")
}
