package relayer

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/header"
	"pkg.akt.dev/node/v2/internal/relayer/native"
)

type r32Transport func(*http.Request) (*http.Response, error)

func (f r32Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type r32Probe struct {
	mu             sync.Mutex
	planned        int
	prepared       int
	discovered     int
	commitAtPlan   []int
	commitAtProof  []int
	commitAtScan   []int
	advanceOnProof bool
}

func r32Attach(t *testing.T, h *resolutionHarness, probe *r32Probe) {
	t.Helper()
	h.accounts[h.followed.AccessState] = resolutionAccountData(t, h.followed.AccessManager, "AccessManager", bridge.AccessManager{
		Roles: []bridge.Role{
			{
				ID:      1,
				Members: []solana.PublicKey{h.followed.Program},
			},
		},
	})
	client, err := native.NewWithHTTP("http://r32-inert.invalid", h.s.cfg.chainID, time.Minute, &http.Client{
		Transport: r32Transport(func(request *http.Request) (*http.Response, error) {
			raw, err := io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			request.Body = io.NopCloser(bytes.NewReader(raw))
			var call struct {
				Method string
				Params struct {
					Path   string
					Height string
				}
			}
			if err := json.Unmarshal(raw, &call); err != nil {
				return nil, err
			}
			probe.mu.Lock()
			switch {
			case call.Method == "commit":
				probe.commitAtPlan = append(probe.commitAtPlan, probe.planned)
				probe.commitAtProof = append(probe.commitAtProof, probe.prepared)
				probe.commitAtScan = append(probe.commitAtScan, probe.discovered)
				probe.mu.Unlock()
				return nil, fmt.Errorf("R32 inert stop before header encoding or signing")
			case call.Params.Path == "/akash.sink.v1.Query/Transfers" || call.Params.Path == "/akash.sink.v1.Query/Disputes":
				probe.discovered++
			case call.Params.Path == "/akash.sink.v1.Query/Transfer" && call.Params.Height == "28":
				probe.planned++
			case call.Params.Path == "/akash.sink.v1.Query/Transfer" && call.Params.Height == "19" && probe.advanceOnProof:
				probe.prepared++
				probe.mu.Unlock()
				h.mu.Lock()
				h.now = h.now.Add(400 * time.Second)
				clock := make([]byte, 40)
				binary.LittleEndian.PutUint64(clock[32:], uint64(h.now.Unix()))
				h.accounts[solana.SysVarClockPubkey].Data = rpc.DataBytesOrJSONFromBytes(clock)
				h.mu.Unlock()
				return nil, fmt.Errorf("R32 inert proof stop before any paid work")
			}
			probe.mu.Unlock()
			response := httptest.NewRecorder()
			h.nativeRPC(response, request)
			return response.Result(), nil
		}),
	})
	require.NoError(t, err)
	h.s.native = client
}

func r32OldConsensus(t *testing.T, h *resolutionHarness, age time.Duration) {
	t.Helper()
	address, err := h.followed.ConsensusAt(20)
	require.NoError(t, err)
	h.accounts[address] = resolutionAccountData(t, h.followed.Program, "ConsensusStateStore", struct {
		Height    uint64
		Timestamp uint64
		Root      [32]byte
		Next      [32]byte
	}{
		Height:    20,
		Timestamp: uint64(h.now.Add(-age).Unix()) * 1_000_000_000,
	})
}

func TestR32HeaderDueBeforeBacklogPlanning(t *testing.T) {
	h := newResolutionHarness(t)
	probe := &r32Probe{}
	r32Attach(t, h, probe)
	r32OldConsensus(t, h, 2000*time.Second)
	for sequence := uint64(1); sequence <= 500; sequence++ {
		transfer := planningTransfer(sequence)
		transfer.ID.Height = 25
		transfer.Status = sink.StatusRefundPending
		transfer.Settlement.VoidAuthorization = &sink.VoidAuthorization{
			RequestedHeight: 26,
			RequestedRound:  1,
		}
		h.transfers[transfer.ID] = transfer
		h.proofs[transfer.ID] = native.Proof{Transfer: transfer}
		h.visible = append(h.visible, transfer.ID)
	}
	require.NoError(t, h.s.tick(context.Background()))
	require.Zero(t, h.calls["sendTransaction"])
	require.Zero(t, h.calls["simulateTransaction"])
	require.Equal(t, 500, probe.planned)
	require.Len(t, probe.commitAtPlan, 1)
	t.Logf("real tick: first header source request after %d discovery requests and %d/500 planning reads; zero sends", probe.commitAtScan[0], probe.commitAtPlan[0])
	require.Zero(t, probe.commitAtPlan[0], "an already age-due header needs an opportunity before discovery and planning")
	require.Zero(t, probe.commitAtScan[0], "an already age-due header needs an opportunity before discovery")
}

func TestR32HeaderOpportunityDuringCoveredQueue(t *testing.T) {
	h := newResolutionHarness(t)
	probe := &r32Probe{
		advanceOnProof: true,
	}
	r32Attach(t, h, probe)
	for sequence := uint64(1); sequence <= 8; sequence++ {
		transfer := planningTransfer(sequence)
		h.proof(transfer, 0)
		h.visible = append(h.visible, transfer.ID)
	}
	require.NoError(t, h.s.tick(context.Background()))
	require.Zero(t, h.calls["sendTransaction"])
	require.Zero(t, h.calls["simulateTransaction"])
	require.Equal(t, 8, probe.prepared)
	t.Logf("real tick: %d covered entries attempted; fake Clock advanced 3200 seconds; header source requests=%d; zero sends", probe.prepared, len(probe.commitAtProof))
	require.NotEmpty(t, probe.commitAtProof, "a newly age-due header needs another opportunity before the tick returns")
	require.Less(t, probe.commitAtProof[0], 8, "upkeep must get an opportunity before draining the complete ready queue")
}

func TestR32RecoveryFailureStopsBeforeHeaderAdmission(t *testing.T) {
	h := newResolutionHarness(t)
	probe := &r32Probe{}
	r32Attach(t, h, probe)
	r32OldConsensus(t, h, 2000*time.Second)
	id := planningTransfer(1).ID
	_, err := h.s.store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
		return true, ledger.attempt(id)
	})
	require.NoError(t, err)
	h.failAccount, err = h.s.deployment.Status(id)
	require.NoError(t, err)
	require.Error(t, h.s.tick(context.Background()))
	require.Empty(t, probe.commitAtPlan)
	require.Zero(t, h.calls["/akash.sink.v1.Query/Disputes"])
	require.Zero(t, h.calls["sendTransaction"])
	require.Zero(t, h.calls["simulateTransaction"])
}

func TestR32UnsupportedRetryRemainsFiniteAcrossCheckpoints(t *testing.T) {
	h := newResolutionHarness(t)
	calls := 0
	action := func() error {
		calls++
		return header.ErrUnsupported
	}
	updated, err := h.s.headerStep(action, 29)
	require.NoError(t, err)
	require.False(t, updated)
	for i := 0; i < 500; i++ {
		h.now = h.now.Add(time.Second)
		updated, err = h.s.headerStep(action, 29)
		require.NoError(t, err)
		require.False(t, updated)
	}
	require.Equal(t, 1, calls)
	h.now = h.now.Add(400 * time.Second)
	updated, err = h.s.headerStep(action, 29)
	require.NoError(t, err)
	require.False(t, updated)
	require.Equal(t, 2, calls)
	require.Zero(t, h.calls["sendTransaction"])
}
