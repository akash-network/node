package relayer

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	cmtcrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	commitment "github.com/cosmos/ibc-go/v10/modules/core/23-commitment/types"
	"github.com/stretchr/testify/require"

	sink "pkg.akt.dev/go/node/sink/v1"
)

func TestPrepareProofSeparateQueriesAndTimings(t *testing.T) {
	for _, failure := range []string{"", "transfer", "proof", "proof-value"} {
		t.Run(failure, func(t *testing.T) {
			s := planningService(t)
			transfer := planningTransfer(1)
			transfer.Status = sink.StatusRefundPending
			transfer.Settlement.VetoReason = "historical veto"
			transfer.Settlement.VoidAuthorization = &sink.VoidAuthorization{
				RequestedHeight: 12,
				RequestedRound:  1,
			}
			job := planningProof(t, s, transfer, 20)
			var merkle commitment.MerkleProof
			require.NoError(t, merkle.Unmarshal(planningHex(t, job.Proof)))
			ops := &cmtcrypto.ProofOps{}
			for i, proof := range merkle.Proofs {
				encoded, err := proof.Marshal()
				require.NoError(t, err)
				ops.Ops = append(ops.Ops, cmtcrypto.ProofOp{
					Type: []string{"ics23:iavl", "ics23:simple"}[i],
					Data: encoded,
				})
			}
			var events bytes.Buffer
			s.deps.streams.Out = &events
			var calls []string
			s.native = planningNative(t, func(path string, height int64, key []byte, prove bool) (abci.ResponseQuery, error) {
				calls = append(calls, path)
				require.Equal(t, int64(19), height)
				if path == "/akash.sink.v1.Query/Transfer" {
					require.False(t, prove)
					if failure == "transfer" {
						return abci.ResponseQuery{}, errors.New("transfer unavailable")
					}
					return planningReply(t, &sink.QueryTransferResponse{Transfer: transfer}, height), nil
				}
				require.True(t, prove)
				require.Equal(t, job.Key, hex.EncodeToString(key))
				if failure == "proof" {
					return abci.ResponseQuery{}, errors.New("proof unavailable")
				}
				value := merkle.Proofs[0].GetExist().Value
				if failure == "proof-value" {
					value = []byte("different historical settlement")
				}
				return abci.ResponseQuery{
					Height:   height,
					Key:      key,
					Value:    value,
					ProofOps: ops,
				}, nil
			})
			got, err := s.prepareProof(context.Background(), transfer.ID, 20)
			if failure != "" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, transfer, got.Transfer)
				require.Equal(t, int64(20), got.Height)
				require.Equal(t, job.Proof, hex.EncodeToString(got.Merkle))
			}
			wantCalls := []string{"/akash.sink.v1.Query/Transfer"}
			if failure != "transfer" {
				wantCalls = append(wantCalls, "/store/sink/key")
			}
			require.Equal(t, wantCalls, calls)
			decoder := json.NewDecoder(&events)
			for i := range calls {
				var event map[string]any
				require.NoError(t, decoder.Decode(&event))
				require.Equal(t, "relay-stage-duration", event["event"])
				require.Equal(t, []string{"transfer-query", "proof-query"}[i], event["stage"])
				require.Equal(t, transfer.ID.String(), event["id"])
				require.Equal(t, failure == "" || i < len(calls)-1, event["completed"])
			}
			var trailing any
			require.ErrorIs(t, decoder.Decode(&trailing), io.EOF)
		})
	}
}
