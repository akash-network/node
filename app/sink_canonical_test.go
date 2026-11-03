package app_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	commitment "github.com/cosmos/ibc-go/v10/modules/core/23-commitment/types"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
)

func TestSinkCanonicalSettlementMembershipProofs(t *testing.T) {
	h := reportingHarness(t)
	result := h.deliver(t, "", h.deposit(10))
	require.Zero(t, result.Code, result.Log)
	h.commitBlock(t, h.now.Add(time.Second))
	var previous []byte
	var receipt *types.VoidAuthorization
	var fixtures []map[string]any
	check := func(status types.Status) {
		t.Helper()
		entry := h.transfer(t, 1)
		require.Equal(t, status, entry.Status)

		if receipt != nil {
			require.Equal(t, receipt, entry.Settlement.VoidAuthorization)
		}

		key, err := types.SettlementKey(types.NewSettlementID(status, entry.ID))
		require.NoError(t, err)
		value, err := entry.Settlement.Marshal()
		require.NoError(t, err)
		root := bytes.Clone(h.a.CommitMultiStore().LastCommitID().Hash)
		query, err := h.a.Query(context.Background(), &abci.RequestQuery{Path: "/store/sink/key", Data: key, Height: h.height, Prove: true})
		require.NoError(t, err)
		require.Zero(t, query.Code, query.Log)
		require.Equal(t, value, query.Value)
		proof, err := commitment.ConvertProofs(query.ProofOps)
		require.NoError(t, err)
		require.NoError(t, proof.VerifyMembership(commitment.GetSDKSpecs(), commitment.NewMerkleRoot(root), commitment.NewMerklePath([]byte(types.StoreKey), key), value))
		badKey := bytes.Clone(key)
		badKey[2] = byte(types.StatusRefunded + 1)
		require.Error(t, proof.VerifyMembership(commitment.GetSDKSpecs(), commitment.NewMerkleRoot(root), commitment.NewMerklePath([]byte(types.StoreKey), badKey), value))
		proofBytes, err := proof.Marshal()
		require.NoError(t, err)
		nonce := types.ProofNonce(key)
		fixtures = append(fixtures, map[string]any{"query_height": h.height, "status": status, "root": hex.EncodeToString(root), "key": hex.EncodeToString(key), "value": hex.EncodeToString(value), "nonce": hex.EncodeToString(nonce[:]), "proof": hex.EncodeToString(proofBytes)})
		t.Logf("status %d: native key %d bytes, value %d bytes, IAVL/multistore proof %d bytes", status, len(key), len(value), len(proofBytes))

		if previous != nil {
			absent, err := h.a.Query(context.Background(), &abci.RequestQuery{Path: "/store/sink/key", Data: previous, Height: h.height, Prove: true})
			require.NoError(t, err)
			require.Zero(t, absent.Code, absent.Log)
			require.Empty(t, absent.Value)
			absence, err := commitment.ConvertProofs(absent.ProofOps)
			require.NoError(t, err)
			require.NoError(t, absence.VerifyNonMembership(commitment.GetSDKSpecs(), commitment.NewMerkleRoot(root), commitment.NewMerklePath([]byte(types.StoreKey), previous)))
		}

		previous = key
	}

	check(types.StatusTransferPending)
	require.NoError(t, h.order(h.id(1)))
	h.commitBlock(t, h.now.Add(time.Second))
	receipt = h.transfer(t, 1).Settlement.VoidAuthorization
	check(types.StatusVoided)
	require.NoError(t, h.resolve(t, h.id(1), types.DecisionConfirmMint))
	h.commitBlock(t, h.now.Add(time.Second))
	check(types.StatusMinted)
	result = h.report(t, 0, 1, types.OutcomeVoided)
	require.Zero(t, result.Code, result.Log)
	require.NoError(t, h.resolve(t, h.id(1), types.DecisionConfirmVoid))
	h.commitBlock(t, h.now.Add(time.Second))
	check(types.StatusRefundPending)
	h.now = time.Unix(h.transfer(t, 1).Settlement.ChallengeEnd, 0).UTC()
	result = h.refund(t, 1)
	require.Zero(t, result.Code, result.Log)
	check(types.StatusRefunded)
	h.checkSettlementExport(t)

	if path := os.Getenv("SINK_SETTLEMENT_PROOF_FIXTURE"); path != "" {
		data, err := json.MarshalIndent(fixtures, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o644)) //nolint:gosec // explicitly requested audit fixture
	}
}

func TestSinkRequestVoidRoundIsBoundBySignature(t *testing.T) {
	for _, mode := range []signing.SignMode{signing.SignMode_SIGN_MODE_DIRECT, signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON} {
		t.Run(mode.String(), func(t *testing.T) {
			h := reportingHarness(t)
			result := h.deliver(t, "", h.deposit(10))
			require.Zero(t, result.Code, result.Log)
			msg := &types.MsgRequestVoid{Authority: h.sender.String(), ID: h.id(1), ExpectedRound: 1}
			cfg := h.a.TxConfig()
			signed, err := cfg.TxDecoder()(h.txFromMode(t, h.key, mode, "", msg))
			require.NoError(t, err)
			builder, err := cfg.WrapTxBuilder(signed)
			require.NoError(t, err)
			changed := signed.GetMsgs()[0].(*types.MsgRequestVoid)
			changed.ExpectedRound++
			require.NoError(t, builder.SetMsgs(changed))
			encoded, err := cfg.TxEncoder()(builder.GetTx())
			require.NoError(t, err)
			result = h.deliverBytes(t, encoded)
			require.NotZero(t, result.Code)
			require.Contains(t, result.Log, "signature verification failed")
			require.Nil(t, h.transfer(t, 1).Settlement.VoidAuthorization)
		})
	}
}
