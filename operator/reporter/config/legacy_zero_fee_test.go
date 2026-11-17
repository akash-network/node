package config_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/config"
)

func loadLegacyFee(t *testing.T, amount string, broadcast bool) config.Config {
	t.Helper()
	input := legacyJSON(t)
	input["feeAmountUakt"] = amount
	data, err := json.Marshal(input)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "legacy.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	cfg, err := config.LoadLegacy(path, "testnet", broadcast)
	require.NoError(t, err)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, data, after)
	return cfg
}

func TestLegacyFixedFeeCompatibility(t *testing.T) {
	for _, amount := range []string{"0", "1", "1000000"} {
		for _, broadcast := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/broadcast=%t", amount, broadcast), func(t *testing.T) {
				cfg := loadLegacyFee(t, amount, broadcast)
				got, err := config.Resolve(cfg)
				require.NoError(t, err)
				require.Equal(t, cfg, got.Config)
				require.Equal(t, amount+"uakt", got.Config.Fees)
				require.Empty(t, got.Config.GasPrices)
				require.Empty(t, got.GasPrices)
				require.Equal(t, amount, got.Fees.AmountOf("uakt").String())
				require.NoError(t, got.Fees.Validate())
				if amount == "0" {
					require.Empty(t, got.Fees, "SDK normalization removes the zero coin")
				}
				require.Equal(t, "200000", got.Gas)
				require.Equal(t, uint64(200000), got.GasPerMessage)
				require.Equal(t, 1, got.Tx.MaxMsgs)
				require.Equal(t, "1000000uakt", got.MaxFee.String())
			})
		}
	}
}

func TestLegacyFixedFeeRejectsInvalid(t *testing.T) {
	for _, value := range []any{"00", "+0", "-1", "0.0", " 0", "1000001", "999999999999999999999999999999999999999999999999999999999999999999999999999999999999", 0, nil} {
		t.Run(fmt.Sprintf("%T/%v", value, value), func(t *testing.T) {
			input := legacyJSON(t)
			input["feeAmountUakt"] = value
			_, err := readLegacy(t, input, false)
			require.Error(t, err)
		})
	}
	t.Run("missing", func(t *testing.T) {
		input := legacyJSON(t)
		delete(input, "feeAmountUakt")
		_, err := readLegacy(t, input, false)
		require.ErrorContains(t, err, "missing legacy config field feeAmountUakt")
	})
	t.Run("lower cap", func(t *testing.T) {
		cfg := loadLegacyFee(t, "2", false)
		cfg.Tx.MaxFee = "1uakt"
		_, err := config.Resolve(cfg)
		require.ErrorContains(t, err, "fees: must be at most tx.max-fee")
	})
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("zero cap/legacy=%t", legacy), func(t *testing.T) {
			cfg := valid(t)
			if legacy {
				cfg = loadLegacyFee(t, "0", false)
			}
			cfg.Tx.MaxFee = "0uakt"
			_, err := config.Resolve(cfg)
			require.ErrorContains(t, err, "tx.max-fee: must be above 0")
		})
	}
	for _, broadcast := range []bool{false, true} {
		t.Run(fmt.Sprintf("canonical zero/broadcast=%t", broadcast), func(t *testing.T) {
			cfg := valid(t)
			cfg.Broadcast = broadcast
			cfg.From = "unused-offline-key"
			cfg.Fees = "0uakt"
			cfg.GasPrices = ""
			_, err := config.Resolve(cfg)
			require.ErrorContains(t, err, "fees: must be above 0")
		})
	}
}

func TestLegacyConfiguredPriceConflict(t *testing.T) {
	for _, amount := range []string{"", "0", "1", "1000000"} {
		for _, price := range []string{"0.025uakt", "0uakt", "malformed"} {
			t.Run(amount+"/"+price, func(t *testing.T) {
				cfg := loadLegacyFee(t, "0", false)
				cfg.Fees = ""
				if amount != "" {
					cfg.Fees = amount + "uakt"
				}
				cfg.GasPrices = price
				_, err := config.Resolve(cfg)
				require.ErrorContains(t, err, "gas-prices: legacy JSON cannot be combined with gas prices")
				require.Equal(t, price, cfg.GasPrices)
			})
		}
	}
}

func TestLegacyZeroPreservesBroadcastBudget(t *testing.T) {
	cases := []struct {
		name      string
		age       int
		timeout   int
		broadcast bool
		valid     bool
	}{
		{
			name:      "raw80",
			age:       80,
			broadcast: true,
			valid:     true,
		},
		{
			name:      "raw81",
			age:       81,
			broadcast: true,
		},
		{
			name:      "command70+10",
			age:       70,
			timeout:   10,
			broadcast: true,
			valid:     true,
		},
		{
			name:      "command71+10",
			age:       71,
			timeout:   10,
			broadcast: true,
		},
		{
			name:      "command1+79",
			age:       1,
			timeout:   79,
			broadcast: true,
			valid:     true,
		},
		{
			name:      "command1+80",
			age:       1,
			timeout:   80,
			broadcast: true,
		},
		{
			name:  "preview1",
			age:   1,
			valid: true,
		},
		{
			name:  "preview600",
			age:   600,
			valid: true,
		},
		{
			name:    "command preview600+120",
			age:     600,
			timeout: 120,
			valid:   true,
		},
		{
			name:      "broadcast600",
			age:       600,
			broadcast: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := legacyJSON(t)
			input["feeAmountUakt"] = "0"
			input["maxAgeSeconds"] = tc.age
			if tc.timeout > 0 {
				delete(input, "keyFile")
				input["externalSigner"] = map[string]any{
					"command":        "/nonexistent/r202-command",
					"timeoutSeconds": tc.timeout,
				}
			}
			got, err := readLegacy(t, input, tc.broadcast)
			if !tc.valid {
				require.ErrorContains(t, err, "max-age:")
				return
			}
			require.NoError(t, err)
			require.Equal(t, time.Duration(tc.age)*time.Second, got.MaxAge)
			require.Equal(t, uint64((tc.age+60)*5), got.SlotLead)
			require.Equal(t, "0uakt", got.Config.Fees)
			require.Empty(t, got.Fees)
			if tc.timeout > 0 {
				require.Equal(t, time.Duration(tc.timeout)*time.Second, got.Legacy.ExternalSigner.Options.Timeout)
			}
		})
	}
}

func TestLegacyZeroRejectsInvalidAge(t *testing.T) {
	for _, age := range []int{0, 601} {
		input := legacyJSON(t)
		input["feeAmountUakt"] = "0"
		input["maxAgeSeconds"] = age
		_, err := readLegacy(t, input, false)
		require.ErrorContains(t, err, "maxAgeSeconds must be an integer from 1 to 600")
	}
}
