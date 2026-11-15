package native

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	cmtcrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	ics23 "github.com/cosmos/ics23/go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/x/sink/keeper"
)

func decode(t *testing.T, text string) []byte {
	t.Helper()
	result, err := hex.DecodeString(text)
	require.NoError(t, err)
	return result
}

func fixtureFile(t *testing.T, name string, target any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("../../../contracts/solana/programs/akt/tests/fixtures", name+".json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, target))
}

func TestLegacyNativeFixtures(t *testing.T) {
	var records []map[string]string
	var boundary struct {
		Records     []map[string]string
		Settlements []map[string]json.RawMessage
	}
	fixtureFile(t, "proof_vectors", &records)
	fixtureFile(t, "native_boundary_vectors", &boundary)
	records = append(records, boundary.Records...)
	for _, test := range records {
		t.Run(test["name"], func(t *testing.T) {
			id, err := ParseID(test["id_text"])
			require.NoError(t, err)
			key, err := keeper.TransferKey(id)
			require.NoError(t, err)
			require.Equal(t, test["transfer_key_hex"], hex.EncodeToString(key))
			amount, ok := sdkmath.NewIntFromString(test["amount"])
			require.True(t, ok)
			data := sink.RecordData{
				Dest:   test["recipient"],
				Amount: sdk.NewCoin("uakt", amount),
			}
			raw, err := data.Marshal()
			require.NoError(t, err)
			require.Equal(t, test["record_value_hex"], hex.EncodeToString(raw))
			hash := sha256.Sum256(key)
			require.Equal(t, test["mint_proof_nonce_hex"], hex.EncodeToString(hash[:]))
			value := transfer(1)
			value.ID = id
			value.Data = data
			require.NoError(t, validTransfer(value))
			require.NoError(t, recordShape(raw))
		})
	}
	var settlements []map[string]json.RawMessage
	fixtureFile(t, "settlement_vectors", &settlements)
	settlements = append(settlements, boundary.Settlements...)
	for _, test := range settlements {
		text := func(key string) string {
			var value string
			require.NoError(t, json.Unmarshal(test[key], &value))
			return value
		}
		t.Run(text("name"), func(t *testing.T) {
			id, err := ParseID(fmt.Sprintf("%s/%s/%s", text("owner"), text("height"), text("sequence")))
			require.NoError(t, err)
			var status int32
			require.NoError(t, json.Unmarshal(test["status"], &status))
			key, err := keeper.SettlementKey(*sink.NewSettlementID(sink.Status(status), id))
			require.NoError(t, err)
			require.Equal(t, text("settlement_key_hex"), hex.EncodeToString(key))
			raw := decode(t, text("settlement_value_hex"))
			var settlement sink.SettlementData
			require.NoError(t, settlement.Unmarshal(raw))
			encoded, err := settlement.Marshal()
			require.NoError(t, err)
			require.Equal(t, raw, encoded)
			if settlement.VoidAuthorization != nil {
				value := transfer(1)
				value.ID = id
				value.Status = sink.Status(status)
				value.Settlement = settlement
				gotKey, gotValue, err := ProofValue(value, settlement.VoidAuthorization.RequestedHeight)
				require.NoError(t, err)
				require.Equal(t, key, gotKey)
				require.Equal(t, raw, gotValue)
			}
		})
	}
}

func TestCanonicalIDIngress(t *testing.T) {
	id := transfer(1).ID
	for _, value := range []string{
		strings.ToUpper(id.Owner) + "/9/1", id.Owner + "/09/1", id.Owner + "/+9/1", id.Owner + "/9/01", id.Owner + "/0/1", id.Owner + "/9/0", id.Owner + "/9223372036854775808/1", id.Owner + "/9/18446744073709551616",
	} {
		_, err := ParseID(value)
		require.Error(t, err, value)
	}
	value := id.Owner + "/9223372036854775807/18446744073709551615"
	got, err := ParseID(value)
	require.NoError(t, err)
	require.Equal(t, value, got.String())
}

func proofOps(t *testing.T, key, value []byte) *cmtcrypto.ProofOps {
	t.Helper()
	proof := func(key, value []byte) []byte {
		out := &ics23.CommitmentProof{
			Proof: &ics23.CommitmentProof_Exist{
				Exist: &ics23.ExistenceProof{
					Key:   key,
					Value: value,
					Leaf: &ics23.LeafOp{
						Hash:         ics23.HashOp_SHA256,
						PrehashValue: ics23.HashOp_SHA256,
						Length:       ics23.LengthOp_VAR_PROTO,
						Prefix:       []byte{0},
					},
				},
			},
		}
		raw, err := out.Marshal()
		require.NoError(t, err)
		return raw
	}
	return &cmtcrypto.ProofOps{
		Ops: []cmtcrypto.ProofOp{
			{
				Type: "ics23:iavl",
				Key:  key,
				Data: proof(key, value),
			},
			{
				Type: "ics23:simple",
				Key:  []byte("sink"),
				Data: proof([]byte("sink"), bytes.Repeat([]byte{42}, 32)),
			},
		},
	}
}

func TestProofAndRetained(t *testing.T) {
	for _, authorized := range []bool{false, true} {
		t.Run(strconv.FormatBool(authorized), func(t *testing.T) {
			historical := transfer(1)
			if authorized {
				historical.Status = sink.StatusRefundPending
				historical.Settlement.VoidAuthorization = &sink.VoidAuthorization{
					RequestedHeight: 9,
					RequestedRound:  1,
				}
				historical.Settlement.VetoReason = "permanent evidence must survive"
			}
			key, value, err := ProofValue(historical, 10)
			require.NoError(t, err)
			ops := proofOps(t, key, value)
			client := mockClient(t, func(r request) (any, error) {
				require.Equal(t, "10", stringParam(t, r, "height"))
				require.Equal(t, "/store/sink/key", stringParam(t, r, "path"))
				require.Equal(t, key, dataParam(t, r))
				return &ctypes.ResultABCIQuery{
					Response: abci.ResponseQuery{
						Height:   10,
						Key:      key,
						Value:    value,
						ProofOps: ops,
					},
				}, nil
			})
			got, err := client.Prove(context.Background(), historical, 11)
			require.NoError(t, err)
			require.Equal(t, historical, got.Transfer)
			require.Equal(t, int64(11), got.Height)
			require.Equal(t, key, got.Key)
			require.Equal(t, value, got.Value)
			// Independent protobuf envelope expected by ts-proto: repeated field 1.
			var expected []byte
			for _, op := range ops.Ops {
				expected = protowire.AppendTag(expected, 1, protowire.BytesType)
				expected = protowire.AppendBytes(expected, op.Data)
			}
			require.Equal(t, expected, got.Merkle)
			retained, err := client.Retained(context.Background(), key, 11, sha256.Sum256(value))
			require.NoError(t, err)
			require.True(t, retained)
		})
	}
}

func TestRetainedOnlyExactPruning(t *testing.T) {
	key, value := []byte("key"), []byte("value")
	for _, test := range []struct {
		name    string
		change  func(*abci.ResponseQuery)
		missing bool
	}{
		{
			name:    "exact-pruned",
			missing: true,
		},
		{
			name: "sdk38",
			change: func(r *abci.ResponseQuery) {
				r.Code = 38
				r.Codespace = "sdk"
			},
		},
		{
			name:   "wrong-height",
			change: func(r *abci.ResponseQuery) { r.Height = 9 },
		},
		{
			name:   "wrong-key",
			change: func(r *abci.ResponseQuery) { r.Key = []byte("other") },
		},
		{
			name:   "codespace",
			change: func(r *abci.ResponseQuery) { r.Codespace = "sdk" },
		},
		{
			name:   "value",
			change: func(r *abci.ResponseQuery) { r.Value = value },
		},
		{
			name:   "proof",
			change: func(r *abci.ResponseQuery) { r.ProofOps = &cmtcrypto.ProofOps{} },
		},
		{
			name:   "unknown-log",
			change: func(r *abci.ResponseQuery) { r.Log = "other" },
		},
		{
			name: "wrong-hash",
			change: func(r *abci.ResponseQuery) {
				r.Log = ""
				r.Value = []byte("different")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := mockClient(t, func(r request) (any, error) {
				result := abci.ResponseQuery{
					Height: 10,
					Key:    key,
					Log:    "version does not exist",
				}
				if test.change != nil {
					test.change(&result)
				}
				return &ctypes.ResultABCIQuery{Response: result}, nil
			})
			retained, err := client.Retained(context.Background(), key, 11, sha256.Sum256(value))
			require.False(t, retained)
			if test.missing {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.NotErrorIs(t, err, ErrPruned)
			}
		})
	}
}

func TestProofRejectsMalformedEnvelope(t *testing.T) {
	key, value := []byte("key"), []byte("value")
	for _, mode := range []string{"missing", "reversed", "wrong-key", "wrong-value", "wrong-store", "ambiguous-before", "ambiguous-after", "duplicated-exist", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			ops := proofOps(t, key, value)
			switch mode {
			case "missing":
				ops.Ops = ops.Ops[:1]
			case "reversed":
				ops.Ops[0], ops.Ops[1] = ops.Ops[1], ops.Ops[0]
			case "wrong-key":
				ops = proofOps(t, []byte("other"), value)
			case "wrong-value":
				ops = proofOps(t, key, []byte("other"))
			case "wrong-store":
				ops.Ops[1].Data = ops.Ops[0].Data
			case "ambiguous-before":
				ops.Ops[0].Data = append([]byte{0x12, 0}, ops.Ops[0].Data...)
			case "ambiguous-after":
				ops.Ops[0].Data = append(ops.Ops[0].Data, 0x12, 0)
			case "duplicated-exist":
				ops.Ops[0].Data = append(ops.Ops[0].Data, ops.Ops[0].Data...)
			case "oversized":
				value = bytes.Repeat([]byte{1}, 16384)
				ops = proofOps(t, key, value)
			}
			_, err := membershipProof(key, value, ops)
			require.Error(t, err)
		})
	}
}

func TestPresenceAndCanonicalCoins(t *testing.T) {
	value := transfer(1)
	raw, err := value.Marshal()
	require.NoError(t, err)
	shape, err := fields(raw)
	require.NoError(t, err)
	var missing []byte
	for _, number := range []protowire.Number{1, 2} {
		missing = protowire.AppendTag(missing, number, protowire.BytesType)
		missing = protowire.AppendBytes(missing, shape[number][0])
	}
	require.Error(t, transferShape(missing))
	for _, amount := range []string{"00", "01", "+1", "-1", "1.0", "", "18446744073709551616"} {
		coin := protowire.AppendTag(nil, 1, protowire.BytesType)
		coin = protowire.AppendBytes(coin, []byte("uakt"))
		coin = protowire.AppendTag(coin, 2, protowire.BytesType)
		coin = protowire.AppendBytes(coin, []byte(amount))
		record := protowire.AppendTag(nil, 1, protowire.BytesType)
		record = protowire.AppendBytes(record, []byte(value.Data.Dest))
		record = protowire.AppendTag(record, 2, protowire.BytesType)
		record = protowire.AppendBytes(record, coin)
		err := recordShape(record)
		if err == nil {
			var data sink.RecordData
			require.NoError(t, data.Unmarshal(record))
			err = validCoin(data.Amount, true)
		}
		require.Error(t, err, amount)
	}
}

func TestTSMembershipBytes(t *testing.T) {
	raw, err := os.ReadFile("testdata/membership-ts.json")
	require.NoError(t, err)
	var fixture struct {
		Key, Value, Merkle string
		Ops                []string
	}
	require.NoError(t, json.Unmarshal(raw, &fixture))
	key, value := decode(t, fixture.Key), decode(t, fixture.Value)
	ops := &cmtcrypto.ProofOps{
		Ops: []cmtcrypto.ProofOp{
			{
				Type: "ics23:iavl",
				Data: decode(t, fixture.Ops[0]),
			},
			{
				Type: "ics23:simple",
				Data: decode(t, fixture.Ops[1]),
			},
		},
	}
	got, err := membershipProof(key, value, ops)
	require.NoError(t, err)
	require.Equal(t, fixture.Merkle, hex.EncodeToString(got))
}

func TestProveValidatesPreReadTransferBeforeRPC(t *testing.T) {
	for _, name := range []string{"identity-height", "unsafe-height", "recipient", "authorization-height", "authorization-round", "snapshot-mismatch"} {
		t.Run(name, func(t *testing.T) {
			read := transfer(1)
			height := int64(11)
			switch name {
			case "identity-height":
				height = read.ID.Height
			case "unsafe-height":
				height = maxSafeHeight + 1
			case "recipient":
				read.Data.Dest = "invalid"
			case "authorization-height":
				read.Settlement.VoidAuthorization = &sink.VoidAuthorization{
					RequestedHeight: height,
					RequestedRound:  1,
				}
			case "authorization-round":
				read.Settlement.VoidAuthorization = &sink.VoidAuthorization{
					RequestedHeight: height - 1,
					RequestedRound:  read.Settlement.Round + 1,
				}
			}
			calls := 0
			client := mockClient(t, func(r request) (any, error) {
				calls++
				require.Equal(t, "/store/sink/key", stringParam(t, r, "path"))
				return &ctypes.ResultABCIQuery{
					Response: abci.ResponseQuery{
						Height: height - 1,
						Key:    dataParam(t, r),
						Value:  []byte("different snapshot"),
					},
				}, nil
			})
			_, err := client.Prove(context.Background(), read, height)
			require.Error(t, err)
			if name == "snapshot-mismatch" {
				require.Equal(t, 1, calls)
				require.ErrorContains(t, err, "snapshot")
			} else {
				require.Zero(t, calls)
			}
		})
	}
}
