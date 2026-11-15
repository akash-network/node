package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"
)

func TestR156HeaderReaderBoundary(t *testing.T) {
	for _, kind := range []string{"chunk", "batch"} {
		t.Run(kind, func(t *testing.T) {
			m, reader, _, f := newRPCFixture(t)
			payer, session := keyByte(4), keyByte(6)
			address, err := f.ChunkAt(payer, 20, 0)
			require.NoError(t, err)
			name := "HeaderChunk"
			read := func() (any, error) { return reader.Chunk(context.Background(), f, payer, 20, 0) }
			if kind == "batch" {
				address, err = f.BatchAt(payer, session)
				require.NoError(t, err)
				name = "SignatureBatch"
				read = func() (any, error) { return reader.Batch(context.Background(), f, payer, session) }
			}
			good := unhex(t, fixtures(t).Accounts[name])
			for _, tc := range []struct {
				name       string
				owner      solana.PublicKey
				data       []byte
				lamports   uint64
				executable bool
				absent     bool
				allocated  bool
			}{
				{
					name:   "explicit-null",
					absent: true,
				},
				{
					name:  "system-empty-zero",
					owner: solana.SystemProgramID,
					data:  []byte{},
				},
				{
					name:     "system-empty-below-mock-rent",
					owner:    solana.SystemProgramID,
					data:     []byte{},
					lamports: 122,
				},
				{
					name:     "system-empty-at-mock-rent",
					owner:    solana.SystemProgramID,
					data:     []byte{},
					lamports: 123,
				},
				{
					name:     "system-empty-above-mock-rent",
					owner:    solana.SystemProgramID,
					data:     []byte{},
					lamports: 124,
				},
				{
					name:  "system-populated",
					owner: solana.SystemProgramID,
					data:  []byte{1},
				},
				{
					name:  "foreign-empty",
					owner: keyByte(99),
					data:  []byte{},
				},
				{
					name:  "foreign-populated",
					owner: keyByte(99),
					data:  good,
				},
				{
					name:  "ics07-empty",
					owner: f.Program,
					data:  []byte{},
				},
				{
					name:  "ics07-malformed",
					owner: f.Program,
					data:  good[:7],
				},
				{
					name:       "system-executable",
					owner:      solana.SystemProgramID,
					data:       []byte{},
					executable: true,
				},
				{
					name:       "ics07-executable",
					owner:      f.Program,
					data:       good,
					executable: true,
				},
				{
					name:      "ics07-valid",
					owner:     f.Program,
					data:      good,
					lamports:  123,
					allocated: true,
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var account *rpc.Account
					if !tc.absent {
						account = accountFixture(tc.owner, tc.data)
						account.Lamports = tc.lamports
						account.Executable = tc.executable
					}
					m.set(address, account)
					value, err := read()
					if tc.absent || (tc.owner == solana.SystemProgramID && len(tc.data) == 0 && !tc.executable) {
						require.NoError(t, err)
						require.Nil(t, value)
					} else if tc.allocated {
						require.NoError(t, err)
						require.NotNil(t, value)
					} else {
						require.Error(t, err)
						require.Nil(t, value)
					}
				})
			}
			identityOffsets := []int{0, 8}
			if kind == "batch" {
				identityOffsets = append(identityOffsets, 9, 41)
			}
			for _, offset := range identityOffsets {
				t.Run(fmt.Sprintf("identity-byte-%d", offset), func(t *testing.T) {
					bad := bytes.Clone(good)
					bad[offset] ^= 1
					m.set(address, accountFixture(f.Program, bad))
					value, err := read()
					require.Error(t, err)
					require.Nil(t, value)
				})
			}
			for _, mode := range []string{"missing", "null", "json", "null-payload", "encoding", "invalid-base64", "missing-value", "missing-context", "null-context", "missing-slot", "null-result", "rpc-error"} {
				t.Run(mode, func(t *testing.T) {
					m.change(func() {
						m.failure = mode == "rpc-error"
						m.changeResult = func(_ rpcCall, result any) any {
							out := result.(map[string]any)
							switch mode {
							case "missing-value":
								delete(out, "value")
							case "missing-context":
								delete(out, "context")
							case "null-context":
								out["context"] = nil
							case "missing-slot":
								out["context"] = map[string]any{}
							case "null-result":
								return nil
							default:
								bad := mutateAccountField(t, accountFixture(solana.SystemProgramID, []byte{}), "data", mode)
								if mode == "invalid-base64" {
									bad.(map[string]any)["data"] = []any{"!", "base64"}
								}
								out["value"] = bad
							}
							return out
						}
					})
					value, err := read()
					require.Error(t, err)
					require.Nil(t, value)
				})
			}
			for _, call := range m.observed() {
				var requested solana.PublicKey
				require.NoError(t, json.Unmarshal(call.Params[0], &requested))
				require.Equal(t, address, requested)
				var options map[string]any
				require.NoError(t, json.Unmarshal(call.Params[1], &options))
				require.Equal(t, "confirmed", options["commitment"])
			}
		})
	}
}
