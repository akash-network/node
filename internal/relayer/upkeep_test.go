package relayer

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	cmtjson "github.com/cometbft/cometbft/libs/json"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"github.com/stretchr/testify/require"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/header"
	"pkg.akt.dev/node/v2/internal/relayer/native"
)

type upkeepProbe struct {
	mu         sync.Mutex
	h          *resolutionHarness
	phase      string
	counts     map[string]int
	admissions []map[string]int
	heights    []string
	hidden     bool
	failProof  bool
	onUnit     func(string, int)
}

func advanceUpkeepClock(h *resolutionHarness, delta time.Duration) {
	h.now = h.now.Add(delta)
	clock := make([]byte, 40)
	binary.LittleEndian.PutUint64(clock[32:], uint64(h.now.Unix()))
	h.accounts[solana.SysVarClockPubkey].Data = rpc.DataBytesOrJSONFromBytes(clock)
}

func (p *upkeepProbe) unit(phase string) {
	p.mu.Lock()
	p.counts[phase]++
	n := p.counts[phase]
	p.mu.Unlock()
	if phase == p.phase && n == 1 {
		p.h.mu.Lock()
		advanceUpkeepClock(p.h, 3*time.Second)
		p.h.mu.Unlock()
	}
	if p.onUnit != nil {
		p.onUnit(phase, n)
	}
}

func newUpkeepProbe(t *testing.T, h *resolutionHarness) *upkeepProbe {
	t.Helper()
	p := &upkeepProbe{
		h:      h,
		counts: make(map[string]int),
	}
	// Open update access avoids introducing unrelated member-only prune work.
	h.accounts[h.followed.AccessState] = resolutionAccountData(t, h.followed.AccessManager, "AccessManager", bridge.AccessManager{
		Roles: []bridge.Role{
			{
				ID:      1,
				Members: []solana.PublicKey{h.followed.Program},
			},
		},
	})
	client, err := native.NewWithHTTP("http://upkeep-inert.invalid", h.s.cfg.chainID, time.Minute, &http.Client{
		Transport: r32Transport(p.native),
	})
	require.NoError(t, err)
	h.s.native = client
	sol := rpc.NewWithCustomRPCClient(jsonrpc.NewClientWithOpts("http://upkeep-solana-inert.invalid", &jsonrpc.RPCClientOpts{
		HTTPClient: &http.Client{
			Transport: r32Transport(p.solana),
		},
	}))
	t.Cleanup(func() { require.NoError(t, sol.Close()) })
	h.s.solana = sol
	h.s.reader = bridge.NewReader(sol, h.s.cfg.program, h.s.cfg.chainID, nil)
	return p
}

func (p *upkeepProbe) native(request *http.Request) (*http.Response, error) {
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	request.Body = io.NopCloser(bytes.NewReader(raw))
	var call struct {
		ID     json.RawMessage
		Method string
		Params struct{ Path, Height, Data string }
	}
	if err := json.Unmarshal(raw, &call); err != nil {
		return nil, err
	}
	if call.Method == "commit" {
		p.mu.Lock()
		counts := make(map[string]int)
		for k, v := range p.counts {
			counts[k] = v
		}
		p.admissions = append(p.admissions, counts)
		p.mu.Unlock()
		return nil, errors.New("inert header source stop")
	}
	path := call.Params.Path
	if path == "/akash.sink.v1.Query/Transfers" || path == "/akash.sink.v1.Query/Disputes" {
		data, err := hex.DecodeString(strings.TrimPrefix(call.Params.Data, "0x"))
		if err != nil {
			return nil, err
		}
		var q sink.QueryTransfersRequest
		if path == "/akash.sink.v1.Query/Transfers" {
			if err := q.Unmarshal(data); err != nil {
				return nil, err
			}
		} else {
			var d sink.QueryDisputesRequest
			if err := d.Unmarshal(data); err != nil {
				return nil, err
			}
			q.Pagination = d.Pagination
		}
		phase := "open"
		if path == "/akash.sink.v1.Query/Transfers" && q.Status == sink.StatusInvalid {
			phase = "history"
		}
		p.mu.Lock()
		p.heights = append(p.heights, call.Params.Height)
		p.mu.Unlock()
		var values []sink.TransferState
		p.h.mu.Lock()
		for _, id := range p.h.visible {
			transfer := p.h.transfers[id]
			if p.hidden && phase == "open" {
				continue
			}
			if path == "/akash.sink.v1.Query/Disputes" {
				if transfer.Settlement.Hold != sink.HoldNone {
					values = append(values, transfer)
				}
			} else if q.Status == sink.StatusInvalid || q.Status == transfer.Status {
				values = append(values, transfer)
			}
		}
		p.h.mu.Unlock()
		start := 0
		if len(q.Pagination.Key) > 0 {
			start, err = strconv.Atoi(string(q.Pagination.Key))
			if err != nil {
				return nil, err
			}
		}
		end := min(start+500, len(values))
		page := &query.PageResponse{}
		if end < len(values) {
			page.NextKey = []byte(strconv.Itoa(end))
		}
		height, err := strconv.ParseInt(call.Params.Height, 10, 64)
		if err != nil {
			return nil, err
		}
		reply := planningReply(p.h.t, &sink.QueryTransfersResponse{
			Transfers:  values[start:end],
			Pagination: page,
		}, height)
		if path == "/akash.sink.v1.Query/Disputes" {
			reply = planningReply(p.h.t, &sink.QueryDisputesResponse{
				Transfers:  values[start:end],
				Pagination: page,
			}, height)
		}
		if len(values[start:end]) > 0 {
			p.unit(phase)
		}
		encoded, err := cmtjson.Marshal(&ctypes.ResultABCIQuery{Response: reply})
		if err != nil {
			return nil, err
		}
		response := httptest.NewRecorder()
		err = json.NewEncoder(response).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      call.ID,
			"result":  json.RawMessage(encoded),
		})
		return response.Result(), err
	}
	phase := ""
	if path == "/akash.sink.v1.Query/Transfer" {
		switch call.Params.Height {
		case "30":
			phase = "saved"
		case "28":
			phase = "planning"
		case "19":
			phase = "preparations"
		}
	}
	response := httptest.NewRecorder()
	p.h.nativeRPC(response, request)
	if phase != "" {
		p.unit(phase)
	}
	if phase == "preparations" && p.failProof {
		return nil, errors.New("inert prepared proof failure")
	}
	return response.Result(), nil
}

func (p *upkeepProbe) solana(request *http.Request) (*http.Response, error) {
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	request.Body = io.NopCloser(bytes.NewReader(raw))
	var call struct {
		Method string
		Params []json.RawMessage
	}
	if err := json.Unmarshal(raw, &call); err != nil {
		return nil, err
	}
	phase := ""
	if call.Method == "getMultipleAccounts" {
		var keys []solana.PublicKey
		if err := json.Unmarshal(call.Params[0], &keys); err != nil {
			return nil, err
		}
		if len(keys) > 0 && keys[0] != p.h.s.deployment.Config {
			phase = "status"
			if keys[0] == solana.SystemProgramID || keys[0] == p.h.s.payer.PublicKey() {
				phase = "recipients"
			}
		}
	}
	response := httptest.NewRecorder()
	p.h.solanaRPC(response, request)
	if phase != "" {
		p.unit(phase)
	}
	return response.Result(), nil
}

func upkeepRefund(sequence uint64) sink.TransferState {
	transfer := planningTransfer(sequence)
	transfer.ID.Height = 25
	transfer.Status = sink.StatusRefundPending
	transfer.Settlement.VoidAuthorization = &sink.VoidAuthorization{
		RequestedHeight: 26,
		RequestedRound:  1,
	}
	return transfer
}

func startUpkeep(t *testing.T, s *service) *headerUpkeep {
	t.Helper()
	ctx := context.Background()
	found, err := s.follow(ctx)
	require.NoError(t, err)
	require.NoError(t, s.recoverPayments(ctx))
	u := &headerUpkeep{
		service:  s,
		expected: found,
	}
	require.NoError(t, u.refresh(ctx, nil))
	return u
}

func TestR35EveryQueuePhaseAdmitsRealHeaderWork(t *testing.T) {
	for _, phase := range []string{"open", "saved", "history", "status", "planning", "recipients", "preparations", "serial"} {
		t.Run(phase, func(t *testing.T) {
			h := newResolutionHarness(t)
			p := newUpkeepProbe(t, h)
			p.phase, p.failProof = phase, true
			r32OldConsensus(t, h, 1799*time.Second)
			count := 205
			if phase == "open" || phase == "history" {
				count = 501
			}
			if phase == "saved" || phase == "planning" || phase == "preparations" || phase == "serial" {
				count = 8
			}
			for i := 1; i <= count; i++ {
				transfer := upkeepRefund(uint64(i))
				if phase == "recipients" || phase == "preparations" || phase == "serial" {
					transfer = planningTransfer(uint64(i))
					h.proof(transfer, 0)
				} else {
					h.transfers[transfer.ID] = transfer
					h.proofs[transfer.ID] = native.Proof{Transfer: transfer}
				}
				if phase == "saved" {
					_, err := h.s.store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
						return true, ledger.attempt(transfer.ID)
					})
					require.NoError(t, err)
				} else {
					h.visible = append(h.visible, transfer.ID)
				}
			}
			p.hidden = phase == "history"
			if phase == "serial" {
				p.failProof = false
				h.afterSend = func(stage string) {
					if stage == "resolve" {
						p.mu.Lock()
						p.counts["serial"]++
						if p.counts["serial"] == 1 {
							advanceUpkeepClock(h, 3*time.Second)
						}
						p.mu.Unlock()
					}
				}
			}
			require.NoError(t, h.s.tick(context.Background()))
			require.NotEmpty(t, p.admissions)
			first := p.admissions[0][phase]
			require.Positive(t, first)
			require.Less(t, first, p.counts[phase], "header source must run before this phase drains")
			for _, height := range p.heights {
				require.Equal(t, "30", height, "pagination must keep the original snapshot")
			}
			if phase != "serial" {
				require.Zero(t, h.calls["sendTransaction"])
			} else {
				require.Equal(t, count, p.counts["serial"])
			}
			t.Logf("actual tick header-source admission after %d/%d %s units", first, p.counts[phase], phase)
		})
	}
}

func TestR35Fast500AndCadenceEquality(t *testing.T) {
	h := newResolutionHarness(t)
	p := newUpkeepProbe(t, h)
	for i := uint64(1); i <= 500; i++ {
		transfer := upkeepRefund(i)
		h.transfers[transfer.ID] = transfer
		h.proofs[transfer.ID] = native.Proof{Transfer: transfer}
		h.visible = append(h.visible, transfer.ID)
	}
	require.NoError(t, h.s.tick(context.Background()))
	require.Len(t, p.admissions, 1, "complete proof demand forces one attempt")
	require.Equal(t, 500, p.admissions[0]["planning"])
	require.Equal(t, 2, h.calls["/akash.sink.v1.Query/State"], "two forced inspections, no RPC per entry")
	h.s.cfg.poll = 600 * time.Second
	u := startUpkeep(t, h.s)
	r32OldConsensus(t, h, 2000*time.Second)
	before := h.calls["/akash.sink.v1.Query/State"]
	h.now = h.now.Add(599 * time.Second)
	for range 500 {
		require.NoError(t, u.check(context.Background()))
	}
	require.Equal(t, before, h.calls["/akash.sink.v1.Query/State"])
	h.now = h.now.Add(time.Second)
	require.NoError(t, u.check(context.Background()))
	require.Equal(t, before+1, h.calls["/akash.sink.v1.Query/State"])
	require.Equal(t, h.now.Add(600*time.Second), u.nextCheck)
}

func TestR35DirtyRecoveryOnlyBeforeAdmittedHeaderAndNotSwallowed(t *testing.T) {
	for _, mode := range []string{"backoff", "no-authority", "not-due", "due"} {
		t.Run(mode, func(t *testing.T) {
			h := newResolutionHarness(t)
			p := newUpkeepProbe(t, h)
			u := startUpkeep(t, h.s)
			proof := h.proof(planningTransfer(1), 0)
			_, err := h.s.store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
				return true, ledger.attempt(proof.Transfer.ID)
			})
			require.NoError(t, err)
			h.failAccount, err = h.s.deployment.Status(proof.Transfer.ID)
			require.NoError(t, err)
			u.paymentsDirty = true
			if mode != "not-due" {
				r32OldConsensus(t, h, 2000*time.Second)
			}
			if mode == "backoff" {
				h.s.headerRetry.RetryAt = h.now.Add(time.Minute).UnixMilli()
			}
			if mode == "no-authority" {
				h.accounts[h.followed.AccessState] = resolutionAccountData(t, h.followed.AccessManager, "AccessManager", bridge.AccessManager{})
			}
			err = u.refresh(context.Background(), nil)
			if mode == "due" {
				require.Error(t, err)
				require.Empty(t, h.errOut.String(), "recovery error cannot become header retry")
			} else {
				require.NoError(t, err)
			}
			require.True(t, u.paymentsDirty)
			require.Empty(t, p.admissions)
			require.Zero(t, h.calls["sendTransaction"])
		})
	}
}

func TestR35MidTickAmbiguousPaymentBlocksHeaderAndRetainsBytes(t *testing.T) {
	for _, mode := range []string{"metadata", "unknown-history"} {
		t.Run(mode, func(t *testing.T) {
			h := newResolutionHarness(t)
			p := newUpkeepProbe(t, h)
			r32OldConsensus(t, h, 1799*time.Second)
			for i := uint64(1); i <= 4; i++ {
				transfer := planningTransfer(i)
				h.proof(transfer, 0)
				h.visible = append(h.visible, transfer.ID)
			}
			h.noMetadata = mode == "metadata"
			h.lostStage = "proof-create"
			var retainedPath string
			var before []byte
			h.afterSend = func(stage string) {
				if stage != "proof-create" {
					return
				}
				advanceUpkeepClock(h, 3*time.Second)
				for _, id := range h.visible {
					path, err := h.s.store.paymentPath(id)
					require.NoError(t, err)
					data, err := os.ReadFile(path)
					if err == nil {
						retainedPath, before = path, data
					}
				}
				if mode == "unknown-history" {
					h.failMethod = "getSignatureStatuses"
				}
			}
			err := h.s.tick(context.Background())
			require.Error(t, err)
			require.NotEmpty(t, retainedPath)
			after, err := os.ReadFile(retainedPath)
			require.NoError(t, err)
			require.True(t, bytes.Equal(before, after), "ambiguous signed intent must remain byte-identical")
			require.Equal(t, []string{"proof-create"}, h.stages)
			require.Equal(t, 1, h.calls["sendTransaction"])
			require.Empty(t, p.admissions)
			require.NotContains(t, h.errOut.String(), "header-retry")
		})
	}
}

func TestR35HistoryCheckpointFailureNeverCommitsOrDowngrades(t *testing.T) {
	h := newResolutionHarness(t)
	p := newUpkeepProbe(t, h)
	p.hidden = true
	for i := uint64(1); i <= 501; i++ {
		transfer := upkeepRefund(i)
		h.transfers[transfer.ID] = transfer
		h.visible = append(h.visible, transfer.ID)
	}
	u := startUpkeep(t, h.s)
	p.onUnit = func(phase string, n int) {
		if phase == "history" && n == 1 {
			h.mu.Lock()
			advanceUpkeepClock(h, 3*time.Second)
			h.failAccount = h.followed.AccessState
			h.mu.Unlock()
		}
	}
	_, err := h.s.unresolved(context.Background(), u.state, u.check)
	require.Error(t, err)
	cursor, err := h.s.store.loadDiscovery()
	require.NoError(t, err)
	require.Nil(t, cursor)
	require.Equal(t, 1, p.counts["history"])
	require.NotContains(t, h.errOut.String(), "relay-discovery-failed")
}

func TestR35PreparedAndRetainedRootsSurviveLatestAdvance(t *testing.T) {
	h := newResolutionHarness(t)
	p := newUpkeepProbe(t, h)
	u := startUpkeep(t, h.s)
	var plans []relayPlan
	proofs := make(map[solana.PublicKey][]byte)
	for i := uint64(1); i <= 4; i++ {
		transfer := planningTransfer(i)
		proof := h.proof(transfer, 0)
		plan := relayPlan{
			id:       transfer.ID,
			transfer: transfer,
			height:   11,
		}
		if i == 4 {
			plan.resumeHeight = 20
			require.NoError(t, h.s.store.saveProof(h.job(proof)))
		}
		plans = append(plans, plan)
		buffer, err := h.s.deployment.ProofBuffer(h.s.payer.PublicKey(), proof.Key)
		require.NoError(t, err)
		proofs[buffer] = bytes.Clone(proof.Merkle)
	}
	p.onUnit = func(phase string, n int) {
		if phase != "preparations" || n != 2 {
			return
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		advanceUpkeepClock(h, 3*time.Second)
		client, err := bridge.DecodeClient(h.accounts[h.followed.ClientState].Data.GetBinary())
		require.NoError(t, err)
		client.LatestHeight.RevisionHeight = 29
		h.accounts[h.followed.ClientState] = resolutionAccountData(t, h.followed.Program, "ClientState", client)
		address, err := h.followed.ConsensusAt(29)
		require.NoError(t, err)
		h.accounts[address] = headerJobAccount(h.followed.Program, headerJobConsensus(29, header.Packet{
			Timestamp: uint64(h.now.Unix()) * 1_000_000_000,
		}))
	}
	require.NoError(t, h.s.relayCovered(context.Background(), plans, u.header, func() { u.paymentsDirty = true }))
	latest, err := h.followed.ConsensusAt(29)
	require.NoError(t, err)
	old, err := h.followed.ConsensusAt(20)
	require.NoError(t, err)
	resolved := make(map[solana.PublicKey]resolutionSent)
	appended := make(map[solana.PublicKey][]byte)
	for _, sent := range h.sent {
		if sent.stage == "resolve" {
			resolved[sent.accounts[4]] = sent
		}
		if sent.stage == "proof-append" {
			require.Len(t, sent.data, 1)
			appended[sent.accounts[1]] = append(appended[sent.accounts[1]], sent.data[0][16:]...)
		}
	}
	for i, plan := range plans {
		status, err := h.s.deployment.Status(plan.id)
		require.NoError(t, err)
		sent, exists := resolved[status]
		require.True(t, exists)
		require.Equal(t, latest, sent.accounts[7])
		proofConsensus := old
		wantHeight := uint64(20)
		if i == 2 {
			proofConsensus, wantHeight = latest, 29
		}
		require.Equal(t, proofConsensus, sent.accounts[8])
		data := sent.data[0]
		require.Equal(t, wantHeight, binary.LittleEndian.Uint64(data[len(data)-8:]))
		require.Equal(t, proofs[sent.accounts[3]], appended[sent.accounts[3]])
	}
	require.Empty(t, p.admissions, "target equals latest: no needless new header")
}

func TestR35RepinAfterPreparationAbortsBeforePayment(t *testing.T) {
	h := newResolutionHarness(t)
	p := newUpkeepProbe(t, h)
	u := startUpkeep(t, h.s)
	var plans []relayPlan
	for i := uint64(1); i <= 2; i++ {
		transfer := planningTransfer(i)
		h.proof(transfer, 0)
		plans = append(plans, relayPlan{
			id:       transfer.ID,
			transfer: transfer,
			height:   11,
		})
	}
	p.onUnit = func(phase string, n int) {
		if phase == "preparations" && n == 2 {
			h.mu.Lock()
			defer h.mu.Unlock()
			advanceUpkeepClock(h, 3*time.Second)
			// The same light-client program can change access-manager identity.
			h.accounts[h.followed.AppState] = resolutionAccountData(t, h.followed.Program, "AppState", struct {
				Manager solana.PublicKey
				Pending bool
				Padding [256]byte
			}{
				Manager: solana.PublicKey{99},
			})
		}
	}
	err := h.s.relayCovered(context.Background(), plans, u.header, func() { u.paymentsDirty = true })
	require.ErrorContains(t, err, "identity changed")
	require.Zero(t, h.calls["sendTransaction"])
}

func TestR35GlobalReservationSurvivesRecipientCheckpoints(t *testing.T) {
	h := newResolutionHarness(t)
	p := newUpkeepProbe(t, h)
	u := startUpkeep(t, h.s)
	state, err := bridge.DecodeBridge(h.accounts[h.s.deployment.Bridge].Data.GetBinary())
	require.NoError(t, err)
	state.Policy.DailyMintCap = 75 * 123
	h.accounts[h.s.deployment.Bridge] = resolutionAccountData(t, h.s.cfg.program, "BridgeConfig", state)
	var plans []relayPlan
	for i := uint64(1); i <= 105; i++ {
		transfer := planningTransfer(i)
		transfer.Data.Dest = h.s.payer.PublicKey().String()
		plans = append(plans, relayPlan{
			id:       transfer.ID,
			transfer: transfer,
		})
	}
	p.phase = "recipients"
	r32OldConsensus(t, h, 1799*time.Second)
	ready, err := h.s.payable(context.Background(), plans, u.check)
	require.NoError(t, err)
	require.Equal(t, plans[:75], ready)
	require.Len(t, h.s.waiting, 30)
	require.Equal(t, 1, p.admissions[0]["recipients"])
}

func TestR35MixedPriorityAndFullOneShot(t *testing.T) {
	h := newResolutionHarness(t)
	p := newUpkeepProbe(t, h)
	var plans []relayPlan
	for i := uint64(1); i <= 9; i++ {
		transfer := planningTransfer(i)
		switch i {
		case 1:
			transfer.Settlement.Hold = sink.Hold(1)
		case 2, 3:
			transfer.Status = sink.StatusRefundPending
			if i == 3 {
				transfer.Status = sink.StatusVoided
			}
			transfer.Settlement.VoidAuthorization = &sink.VoidAuthorization{
				RequestedHeight: 12,
				RequestedRound:  1,
			}
		}
		proof := h.proof(transfer, 0)
		plan := relayPlan{
			id:       transfer.ID,
			transfer: transfer,
			height:   20,
		}
		if i == 4 {
			require.NoError(t, h.s.store.saveProof(h.job(proof)))
			plan.retained = true
		}
		plans = append(plans, plan)
		h.visible = append(h.visible, transfer.ID)
	}
	// Query priority places the dispute first, then refund/void, then fresh slots.
	want := relayOrder(plans, h.s.payer.PublicKey())
	runDeps := h.s.deps
	runDeps.sleep = func(context.Context, time.Duration) error {
		t.Fatal("one shot must not sleep between chunks")
		return nil
	}
	require.NoError(t, runTicks(context.Background(), false, h.s.cfg.poll, h.s.tick, runDeps))
	var got []string
	decoder := json.NewDecoder(&h.out)
	for decoder.More() {
		var event map[string]any
		require.NoError(t, decoder.Decode(&event))
		if event["event"] == "transfer-resolved" {
			got = append(got, event["id"].(string))
		}
	}
	var ids []string
	for _, plan := range want {
		ids = append(ids, plan.id.String())
	}
	require.Equal(t, ids, got)
	require.Empty(t, p.admissions)
}

// This composes the actual native reader and header job with inert RPCs. The
// Solana harness interprets instructions; it does not execute the SBF program.
func upkeepHeaderNative(t *testing.T, h *headerJobRPC) {
	t.Helper()
	n := newResolutionHarness(t)
	n.s.cfg.chainID = h.s.cfg.chainID
	raw, err := os.ReadFile("header/testdata/template.json")
	require.NoError(t, err)
	source, err := header.ParseSource(raw)
	require.NoError(t, err)
	source.Signed.Header.Height = 29
	source.Signed.Header.Time = time.Unix(h.clock, 0)
	source.TrustedHeight = h.latest
	source.Revision = 1
	source, err = header.Fixture(source, 4, 4, false)
	require.NoError(t, err)
	encoded, err := header.Encode(source)
	require.NoError(t, err)
	h.packet, err = header.RuntimePacket(encoded)
	require.NoError(t, err)
	h.s.native, err = native.NewWithHTTP("http://header-upkeep-inert.invalid", h.s.cfg.chainID, time.Minute, &http.Client{
		Transport: r32Transport(func(request *http.Request) (*http.Response, error) {
			raw, err := io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			request.Body = io.NopCloser(bytes.NewReader(raw))
			var call struct {
				ID     json.RawMessage
				Method string
				Params struct{ Height string }
			}
			if err := json.Unmarshal(raw, &call); err != nil {
				return nil, err
			}
			var result any
			switch call.Method {
			case "commit":
				require.Equal(t, "29", call.Params.Height)
				result = &ctypes.ResultCommit{
					SignedHeader:    *source.Signed,
					CanonicalCommit: true,
				}
			case "validators":
				height, err := strconv.ParseInt(call.Params.Height, 10, 64)
				if err != nil {
					return nil, err
				}
				result = &ctypes.ResultValidators{
					BlockHeight: height,
					Validators:  source.Validators,
					Count:       len(source.Validators),
					Total:       len(source.Validators),
				}
			default:
				response := httptest.NewRecorder()
				n.nativeRPC(response, request)
				return response.Result(), nil
			}
			encoded, err := cmtjson.Marshal(result)
			if err != nil {
				return nil, err
			}
			response := httptest.NewRecorder()
			err = json.NewEncoder(response).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      call.ID,
				"result":  json.RawMessage(encoded),
			})
			return response.Result(), err
		}),
	})
	require.NoError(t, err)
	h.s.cfg.poll = 3 * time.Second
	h.s.cfg.refreshFraction = 0.5
}

func TestR35ActualHeaderUpdateAndCompletionCadence(t *testing.T) {
	h := newHeaderJobFixture(t)
	h.clock = 50_000
	h.setClock()
	upkeepHeaderNative(t, h)
	u := startUpkeep(t, h.s)
	require.Equal(t, headerHeight(29), u.current.height)
	require.Equal(t, 1, countHeaderStage(h.stages, "assembly"))
	require.Contains(t, h.stages, "signature")
	require.Contains(t, h.stages, "chunk")
	require.Contains(t, h.stages, "cleanup")
	require.Contains(t, h.stages, "lookup-deactivate")
	require.Contains(t, h.out.String(), `"event":"authenticated-header"`)
	require.Equal(t, h.now.Add(h.s.cfg.poll), u.nextCheck, "cadence starts after transaction/lookup completion")
	active, err := h.s.store.loadActive()
	require.NoError(t, err)
	require.Nil(t, active)
	before := len(h.stages)
	h.now = u.nextCheck
	require.NoError(t, u.check(context.Background()))
	require.Len(t, h.stages, before, "native has no newer target")
	t.Logf("actual Go header-job trace (inert Solana instruction interpreter): %v", h.stages)
}

func TestR35ActiveHeaderAuthorityRetryAndExactIntent(t *testing.T) {
	h := newHeaderJobFixture(t)
	h.active(t)
	h.lostStage = "assembly"
	require.Error(t, h.s.resumeHeader(context.Background(), nil))
	ids, err := h.s.store.sessionIDs()
	require.NoError(t, err)
	require.Len(t, ids, 1)
	path, err := h.s.store.sessionPendingPath(ids[0])
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	intent, err := h.s.store.loadIntent(path)
	require.NoError(t, err)
	stages := append([]string(nil), h.stages...)
	h.lostStage = ""
	// Native state is needed after an authorized resume. The saved packet is
	// still h.packet from the original active session, not a newly encoded one.
	original := h.packet
	upkeepHeaderNative(t, h)
	h.packet = original
	h.access = bridge.AccessNone
	h.accounts[h.f.AccessState] = headerJobAccount(h.f.AccessManager, h.accessData())
	u := startUpkeep(t, h.s)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.Equal(before, after))
	require.Equal(t, stages, h.stages)
	h.access = bridge.AccessOpen
	h.accounts[h.f.AccessState] = headerJobAccount(h.f.AccessManager, h.accessData())
	h.s.headerRetry.RetryAt = h.now.Add(6 * time.Second).UnixMilli()
	require.NoError(t, u.refresh(context.Background(), nil))
	require.Equal(t, stages, h.stages)
	h.now = h.now.Add(6 * time.Second)
	require.NoError(t, u.check(context.Background()))
	require.Equal(t, 1, countHeaderStage(h.stages, "assembly"))
	require.Equal(t, intent.Raw, h.transactions[intent.Signature].raw)
	require.Contains(t, h.stages, "cleanup")
	_, err = os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestR35BudgetCleanupMarksDirtyBeforePayment(t *testing.T) {
	h := newResolutionHarness(t)
	p := newUpkeepProbe(t, h)
	r32OldConsensus(t, h, 1799*time.Second)
	proof := h.proof(planningTransfer(1), 0)
	h.visible = []sink.ID{proof.Transfer.ID}
	job := h.job(proof)
	require.NoError(t, h.s.store.saveProof(job))
	buffer, err := h.s.deployment.ProofBuffer(h.s.payer.PublicKey(), proof.Key)
	require.NoError(t, err)
	nonce, err := hex.DecodeString(job.Nonce)
	require.NoError(t, err)
	h.buffer(buffer, bridge.ProofBuffer{
		Submitter:   h.s.payer.PublicKey(),
		Nonce:       [32]byte(nonce),
		ExpectedLen: uint32(len(proof.Merkle)),
		Data:        proof.Merkle,
		Allocated:   proofHeaderSize + len(proof.Merkle),
		Lamports:    1000,
	})
	_, err = h.s.store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
		return true, ledger.charge(proof.Transfer.ID, h.s.cfg.maxLamports+1, "prior-spend", 0)
	})
	require.NoError(t, err)
	h.lostStage = "proof-close"
	h.noMetadata = true
	h.afterSend = func(stage string) {
		require.Equal(t, "proof-close", stage)
		advanceUpkeepClock(h, 3*time.Second)
	}
	err = h.s.tick(context.Background())
	require.Error(t, err)
	require.Equal(t, []string{"proof-close"}, h.stages)
	require.Empty(t, p.admissions)
	path, err := h.s.store.paymentPath(proof.Transfer.ID)
	require.NoError(t, err)
	intent, err := h.s.store.loadIntent(path)
	require.NoError(t, err)
	require.NotNil(t, intent)
	require.Equal(t, "proof-close", intent.Stage)
	require.NotContains(t, h.errOut.String(), "header-retry")
}

func TestR35InactiveBetweenPreparationAndPayment(t *testing.T) {
	for _, mode := range []string{"frozen", "expired"} {
		t.Run(mode, func(t *testing.T) {
			h := newResolutionHarness(t)
			p := newUpkeepProbe(t, h)
			u := startUpkeep(t, h.s)
			transfer := planningTransfer(1)
			h.proof(transfer, 0)
			p.onUnit = func(phase string, n int) {
				if phase == "preparations" && n == 1 {
					h.mu.Lock()
					defer h.mu.Unlock()
					advanceUpkeepClock(h, 3*time.Second)
					client, err := bridge.DecodeClient(h.accounts[h.followed.ClientState].Data.GetBinary())
					require.NoError(t, err)
					if mode == "frozen" {
						client.FrozenHeight.RevisionHeight = 1
					} else {
						client.TrustingPeriod = 3
					}
					h.accounts[h.followed.ClientState] = resolutionAccountData(t, h.followed.Program, "ClientState", client)
				}
			}
			err := h.s.relayCovered(context.Background(), []relayPlan{
				{
					id:       transfer.ID,
					transfer: transfer,
					height:   11,
				},
			}, u.header, func() { u.paymentsDirty = true })
			require.Error(t, err)
			require.Empty(t, p.admissions)
			require.Zero(t, h.calls["sendTransaction"])
		})
	}
}
