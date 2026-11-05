package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/x/sink/keeper"
)

// Solana's additional boundary cases use the unchanged native codecs and SDK.
// UPDATE_PROOF_VECTORS=1 regenerates the fixture consumed by Rust and TypeScript.
func TestNativeBoundaryVectors(t *testing.T) {
	cdc := codec.NewProtoCodec(sdkutil.NewCodecOptions().NewInterfaceRegistry())
	var records []map[string]string
	for _, size := range []int{1, 20, 32, 255} {
		owner := bytes.Repeat([]byte{42}, size)
		id := types.ID{
			Owner:    sdk.AccAddress(owner).String(),
			Height:   9,
			Sequence: 3,
		}
		require.NoError(t, id.ValidateBasic())
		data := types.RecordData{
			Dest:   "3JF3sEqM796hk5WFqA6EtmEwJQ9quALszsfJyvXNQKy3",
			Amount: sdk.NewInt64Coin(sdkutil.DenomUakt, 123),
		}
		require.NoError(t, data.ValidateBasic())
		key, err := keeper.TransferKey(id)
		require.NoError(t, err)
		value, err := codec.CollValue[types.RecordData](cdc).Encode(data)
		require.NoError(t, err)
		nonce := sha256.Sum256(key)
		records = append(records, map[string]string{
			"name":                 fmt.Sprintf("owner_%d_bytes", size),
			"owner":                id.Owner,
			"owner_hex":            hex.EncodeToString(owner),
			"height":               "9",
			"sequence":             "3",
			"recipient":            data.Dest,
			"recipient_hex":        hex.EncodeToString(bytes.Repeat([]byte{0x22}, 32)),
			"amount":               "123",
			"id_text":              id.String(),
			"transfer_key_hex":     hex.EncodeToString(key),
			"record_value_hex":     hex.EncodeToString(value),
			"mint_proof_nonce_hex": hex.EncodeToString(nonce[:]),
			"height_le_hex":        hex.EncodeToString(binary.LittleEndian.AppendUint64(nil, 9)),
			"sequence_le_hex":      hex.EncodeToString(binary.LittleEndian.AppendUint64(nil, 3)),
		})
	}

	members := make([]string, 7)
	for i := range members {
		members[i] = sdk.AccAddress(bytes.Repeat([]byte{byte(i + 1)}, 32)).String()
	}
	policy := types.ReporterPolicy{
		Version:   1,
		Members:   members,
		Threshold: sdkmath.LegacyOneDec(),
	}
	id := types.ID{
		Owner:    members[0],
		Height:   9,
		Sequence: 3,
	}
	settlement := types.SettlementData{
		Round:        1,
		Policy:       policy,
		ChallengeEnd: 1000,
		VoidAuthorization: &types.VoidAuthorization{
			RequestedHeight: 9,
			RequestedRound:  1,
		},
	}
	for _, member := range members {
		settlement.Votes = append(settlement.Votes, types.Vote{
			Reporter:      member,
			PolicyVersion: 1,
			Observation: types.Observation{
				Outcome:              types.OutcomeVoided,
				Slot:                 100,
				TransactionSignature: bytes.Repeat([]byte{1}, 64),
			},
		})
	}
	require.NoError(t, settlement.Validate(id, types.StatusRefundPending, map[uint64]types.ReporterPolicy{1: policy}))
	key, err := keeper.SettlementKey(*types.NewSettlementID(types.StatusRefundPending, id))
	require.NoError(t, err)
	value, err := codec.CollValue[types.SettlementData](cdc).Encode(settlement)
	require.NoError(t, err)
	encodedSettlement, err := cdc.MarshalJSON(&settlement)
	require.NoError(t, err)
	nonce := sha256.Sum256(key)
	fixture := map[string]any{
		"records": records,
		"settlements": []map[string]any{
			{
				"name":                 "7_reporters_32_byte_addresses",
				"owner":                id.Owner,
				"owner_hex":            hex.EncodeToString(bytes.Repeat([]byte{1}, 32)),
				"height":               "9",
				"sequence":             "3",
				"status":               int32(types.StatusRefundPending),
				"settlement_key_hex":   hex.EncodeToString(key),
				"settlement_value_hex": hex.EncodeToString(value),
				"proof_nonce_hex":      hex.EncodeToString(nonce[:]),
				"settlement":           json.RawMessage(encodedSettlement),
			},
		},
	}
	encoded, err := json.MarshalIndent(fixture, "", "  ")
	require.NoError(t, err)
	encoded = append(encoded, '\n')
	const path = "../../programs/akt/tests/fixtures/native_boundary_vectors.json"
	if os.Getenv("UPDATE_PROOF_VECTORS") == "1" {
		require.NoError(t, os.WriteFile(path, encoded, 0o600))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), string(encoded))
}
