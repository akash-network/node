package relayer

import (
	"context"
	"crypto/sha256"
	"testing"

	solana "github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/header"
)

func (h *headerJobRPC) expiredStates(t *testing.T, count int) {
	t.Helper()
	h.latest = 100
	h.trusting = 10
	h.accounts[h.f.ClientState] = headerJobAccount(h.f.Program, h.clientData())
	for i := 1; i <= count; i++ {
		address, err := h.f.ConsensusAt(uint64(i))
		require.NoError(t, err)
		h.accounts[address] = headerJobAccount(h.f.Program, headerJobConsensus(uint64(i), header.Packet{Timestamp: 990_000_000_000}))
	}
	latest, err := h.f.ConsensusAt(h.latest)
	require.NoError(t, err)
	h.accounts[latest] = headerJobAccount(h.f.Program, headerJobConsensus(h.latest, header.Packet{Timestamp: 990_000_000_000}))
	foreign := solana.PublicKey(sha256.Sum256([]byte("noncanonical consensus")))
	h.accounts[foreign] = headerJobAccount(h.f.Program, headerJobConsensus(55, header.Packet{Timestamp: 0}))
}

func TestHeaderJobPruneCountsOrderBoundsAndCadence(t *testing.T) {
	h := newHeaderJobFixture(t)
	h.expiredStates(t, 22)
	require.NoError(t, h.s.pruneConsensus(context.Background(), h.f))
	require.Len(t, h.pruned, 20)
	for i, height := range h.pruned {
		require.Equal(t, uint64(i+1), height)
	}
	require.Equal(t, 3, countHeaderStage(h.stages, "consensus-prune"))
	require.Contains(t, h.out.String(), `"scanned":24`)
	require.Contains(t, h.out.String(), `"expired":22`)
	require.Contains(t, h.out.String(), `"remaining":2`)
	require.Contains(t, h.out.String(), `"reclaimedLamports":2460`)
	calls := len(h.calls)
	require.NoError(t, h.s.pruneConsensus(context.Background(), h.f))
	require.Len(t, h.calls, calls)
}

func TestHeaderJobPrunePermissionAndFailures(t *testing.T) {
	for _, allowed := range []bridge.Access{bridge.AccessNone, bridge.AccessOpen, bridge.AccessMember} {
		t.Run(string(allowed), func(t *testing.T) {
			h := newHeaderJobFixture(t)
			h.expiredStates(t, 22)
			h.access = allowed
			h.accounts[h.f.AccessState] = headerJobAccount(h.f.AccessManager, h.accessData())
			h.failStage = "consensus-prune"
			h.failure = map[string]any{
				"InstructionError": []any{0, "InvalidArgument"},
			}
			var result consensusPruneResult
			_, err := h.s.withPayer(context.Background(), func(p *payerSession) error {
				var err error
				result, err = p.pruneConsensus(context.Background(), h.f, 20, 8)
				return err
			})
			require.NoError(t, err)
			if allowed == bridge.AccessMember {
				require.Equal(t, 8, result.Failed)
				require.Equal(t, 22, result.Remaining)
				require.Equal(t, uint64(5000), result.FeeLamports)
				require.Len(t, h.stages, 1)
			} else {
				require.Equal(t, "payer lacks RELAYER_ROLE", result.Skipped)
				require.Empty(t, h.stages)
			}
			require.Empty(t, h.pruned)
		})
	}
	h := newHeaderJobFixture(t)
	h.failMethod = "getProgramAccounts"
	require.NoError(t, h.s.pruneConsensus(context.Background(), h.f))
	require.Contains(t, h.errOut.String(), `"event":"consensus-prune-failed"`)
	require.Empty(t, h.stages)
}
