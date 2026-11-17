package tx

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/config"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

// This is offline signing with a disposable test signer, not native simulation or admission.
func TestLegacyFixedFeeTransactionShape(t *testing.T) {
	s := testSigner(t)
	input := map[string]any{
		"akashRpc":          "http://127.0.0.1:1",
		"solanaRpc":         "http://127.0.0.1:1",
		"akashChainId":      testChainID,
		"reporter":          s.Address().String(),
		"solanaGenesisHash": solana.TokenProgramID.String(),
		"programId":         solana.TokenProgramID.String(),
		"mint":              aktsolana.PDA(solana.TokenProgramID, []byte("mint")).String(),
		"maxAgeSeconds":     80,
		"pollSeconds":       3,
		"gasLimit":          "200000",
		"feeAmountUakt":     "0",
		"keyFile":           filepath.Join(t.TempDir(), "unused.key"),
	}
	path := filepath.Join(t.TempDir(), "legacy.json")
	cases := []struct {
		name      string
		fee       string
		canonical bool
		auto      bool
		simFee    string
		finalFee  string
	}{
		{
			name:     "legacy zero",
			fee:      "0",
			simFee:   "0",
			finalFee: "0",
		},
		{
			name:     "legacy one",
			fee:      "1",
			simFee:   "1",
			finalFee: "1",
		},
		{
			name:     "legacy max",
			fee:      "1000000",
			simFee:   "1000000",
			finalFee: "1000000",
		},
		{
			name:      "canonical manual priced",
			canonical: true,
			simFee:    "5000",
			finalFee:  "5000",
		},
		{
			name:      "canonical auto priced ceil",
			canonical: true,
			auto:      true,
			simFee:    "1",
			finalFee:  "3751",
		},
		{
			name:      "canonical fixed wins over prices",
			fee:       "1",
			canonical: true,
			simFee:    "1",
			finalFee:  "1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input["feeAmountUakt"] = "0"
			if tc.fee != "" {
				input["feeAmountUakt"] = tc.fee
			}
			data, err := json.Marshal(input)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, data, 0o600))
			cfg, err := config.LoadLegacy(path, "testnet", false)
			require.NoError(t, err)
			if tc.canonical {
				cfg.Legacy = nil
				cfg.GRPCAddr = "127.0.0.1:1"
				cfg.GasPrices = "0.025uakt"
				cfg.Fees = ""
				if tc.fee != "" {
					cfg.Fees = tc.fee + "uakt"
				}
				if tc.auto {
					cfg.Gas = "auto"
					cfg.GasAdjustment = 1.5
				}
			}
			resolved, err := config.Resolve(cfg)
			require.NoError(t, err)
			rec := &recordingSigner{Signer: s}
			builder := NewBuilder(testTxConfig(), testChainID, testAccountNumber, rec)
			b := New(Options{
				MaxMsgs:       resolved.Tx.MaxMsgs,
				GasPerMessage: resolved.GasPerMessage,
				GasAdjustment: resolved.GasAdjustment,
				GasPrices:     resolved.GasPrices,
				Fees:          resolved.Fees,
				MaxGas:        resolved.Tx.MaxGas,
				MaxFee:        resolved.MaxFee,
			}, nil, builder, nil)
			timeout := SlotTimeout(15_000_000, 3)
			msgs := []sdk.Msg{voteMsg(s)}
			sim, err := b.SimBytes(msgs, timeout)
			require.NoError(t, err)
			require.Empty(t, rec.got, "preparing simulation must never sign")
			simRaw, simBody, simAuth := decodeRaw(t, sim)
			requireUnorderedBody(t, simBody, simAuth, timeout)
			require.Equal(t, tc.simFee, simAuth.Fee.Amount.AmountOf("uakt").String())
			require.Equal(t, resolved.GasPerMessage, simAuth.Fee.GasLimit)
			require.Len(t, simRaw.Signatures, 1)
			require.Equal(t, make([]byte, 64), simRaw.Signatures[0])

			// The used-gas value is synthetic; this test does not call a simulation service.
			gas := b.gasLimit(100001, len(msgs))
			expectedGas := uint64(200000)
			if tc.auto {
				expectedGas = 150002
			}
			require.Equal(t, expectedGas, gas)
			fee := FeeFor(gas, resolved.GasPrices, resolved.Fees)
			require.Equal(t, tc.finalFee, fee.AmountOf("uakt").String())
			require.Empty(t, b.overCap(gas, fee))
			encoded, hash, err := builder.Build(t.Context(), msgs, gas, fee, timeout)
			require.NoError(t, err)
			raw, body, auth := decodeRaw(t, encoded)
			requireUnorderedBody(t, body, auth, timeout)
			require.Equal(t, simRaw.BodyBytes, raw.BodyBytes)
			require.Equal(t, gas, auth.Fee.GasLimit)
			require.Equal(t, tc.finalFee, auth.Fee.Amount.AmountOf("uakt").String())
			if tc.finalFee == "0" {
				require.Empty(t, simAuth.Fee.Amount)
				require.Empty(t, auth.Fee.Amount)
			}
			if !tc.auto {
				require.Equal(t, simRaw.AuthInfoBytes, raw.AuthInfoBytes)
			}
			require.Len(t, rec.got, 1)
			require.Len(t, raw.Signatures, 1)
			require.Len(t, raw.Signatures[0], 64)
			signDoc, err := (&txtypes.SignDoc{
				BodyBytes:     raw.BodyBytes,
				AuthInfoBytes: raw.AuthInfoBytes,
				ChainId:       testChainID,
				AccountNumber: testAccountNumber,
			}).Marshal()
			require.NoError(t, err)
			require.Equal(t, signDoc, rec.got[0])
			require.True(t, s.PubKey().VerifySignature(signDoc, raw.Signatures[0]))
			sum := sha256.Sum256(encoded)
			require.Equal(t, sum[:], hash)
		})
	}
}
