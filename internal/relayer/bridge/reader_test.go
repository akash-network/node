package bridge

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	sink "pkg.akt.dev/go/node/sink/v1"
)

type rpcCall struct {
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
	ID     json.RawMessage   `json:"id"`
}

type rpcFixture struct {
	mu              sync.Mutex
	accounts        map[string]*rpc.Account
	calls           []rpcCall
	slot            uint64
	failure         bool
	batchDelta      int
	programAccounts []*rpc.KeyedAccount
	changeResult    func(rpcCall, any) any
}

func accountFixture(owner solana.PublicKey, data []byte) *rpc.Account {
	return &rpc.Account{
		RentEpoch: new(big.Int),
		Owner:     owner,
		Lamports:  123,
		Data:      rpc.DataBytesOrJSONFromBytes(data),
	}
}

func newRPCFixture(t *testing.T) (*rpcFixture, *Reader, Deployment, Followed) {
	t.Helper()
	m := &rpcFixture{
		accounts: make(map[string]*rpc.Account),
		slot:     100,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		var call rpcCall
		if err := json.NewDecoder(req.Body).Decode(&call); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(400)
			return
		}
		m.calls = append(m.calls, call)
		w.Header().Set("Content-Type", "application/json")
		if m.failure {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      call.ID,
				"error": map[string]any{
					"code":    -32005,
					"message": "injected unavailable",
				},
			})
			return
		}
		var value any
		switch call.Method {
		case "getAccountInfo":
			var address string
			_ = json.Unmarshal(call.Params[0], &address)
			value = map[string]any{
				"context": map[string]any{"slot": m.slot},
				"value":   m.accounts[address],
			}
		case "getMultipleAccounts":
			var addresses []string
			_ = json.Unmarshal(call.Params[0], &addresses)
			infos := make([]*rpc.Account, 0, len(addresses))
			for _, address := range addresses {
				infos = append(infos, m.accounts[address])
			}
			if m.batchDelta < 0 {
				infos = infos[:len(infos)-1]
			}
			if m.batchDelta > 0 {
				infos = append(infos, nil)
			}
			value = map[string]any{
				"context": map[string]any{"slot": m.slot},
				"value":   infos,
			}
		case "getProgramAccounts":
			value = m.programAccounts
		default:
			t.Errorf("unexpected RPC method (reads only): %s", call.Method)
		}
		if m.changeResult != nil {
			value = m.changeResult(call, value)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      call.ID,
			"result":  value,
		})
	}))
	t.Cleanup(server.Close)
	client := rpc.New(server.URL)
	t.Cleanup(func() { _ = client.Close() })
	d, err := Derive(keyByte(1))
	require.NoError(t, err)

	f, err := DeriveClient(keyByte(2), keyByte(3))
	require.NoError(t, err)

	r := NewReader(client, d.Program, "akash-local-1", nil)
	return m, r, d, f
}

func (m *rpcFixture) change(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn()
}

func (m *rpcFixture) observed() []rpcCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]rpcCall(nil), m.calls...)
}

func (m *rpcFixture) set(address solana.PublicKey, info *rpc.Account) {
	m.change(func() { m.accounts[address.String()] = info })
}

func loadFollow(t *testing.T, m *rpcFixture, d Deployment, f Followed) {
	t.Helper()
	g := fixtures(t)
	m.set(d.Bridge, accountFixture(d.Program, unhex(t, g.Accounts["BridgeConfig"])))
	m.set(f.AppState, accountFixture(f.Program, unhex(t, g.Accounts["AppState"])))
}

func TestFollowReadOrderingAndMinimumSlot(t *testing.T) {
	m, r, d, f := newRPCFixture(t)
	loadFollow(t, m, d, f)
	found, err := r.Follow(context.Background())
	require.NoError(t, err, "client deliberately absent: discovery must not block payment recovery")
	require.Equal(t, f.Program, found.Program)
	require.Equal(t, f.AccessManager, found.AccessManager)
	require.Equal(t, f.ClientState, found.ClientState)
	require.Equal(t, f.AppState, found.AppState)
	require.Equal(t, f.AccessState, found.AccessState)
	require.Equal(t, uint64(100), found.Slot)
	require.Equal(t, "akash-local-1", found.ChainID)
	require.Zero(t, found.Latest)

	calls := m.observed()
	require.Len(t, calls, 2)
	var options map[string]any
	require.NoError(t, json.Unmarshal(calls[1].Params[1], &options))
	require.Equal(t, float64(100), options["minContextSlot"])
	require.Equal(t, "confirmed", options["commitment"])

	m.change(func() {
		m.changeResult = func(call rpcCall, value any) any {
			if bytes.Contains(call.Params[0], []byte(f.AppState.String())) {
				value.(map[string]any)["context"] = map[string]any{"slot": 99}
			}
			return value
		}
	})
	_, err = r.Follow(context.Background())
	require.ErrorContains(t, err, "context slot")
}

func TestFollowMismatchAndMalformedReads(t *testing.T) {
	for _, branch := range []string{"chain", "pin", "owner", "discriminator", "app-owner", "absent", "outage"} {
		t.Run(branch, func(t *testing.T) {
			m, r, d, f := newRPCFixture(t)
			loadFollow(t, m, d, f)
			switch branch {
			case "chain":
				r.chainID = "another-chain"
			case "pin":
				pin := keyByte(20)
				r.pin = &pin
			case "owner":
				m.set(d.Bridge, accountFixture(keyByte(55), unhex(t, fixtures(t).Accounts["BridgeConfig"])))
			case "discriminator":
				m.set(d.Bridge, accountFixture(d.Program, make([]byte, 500)))
			case "app-owner":
				m.set(f.AppState, accountFixture(d.Program, unhex(t, fixtures(t).Accounts["AppState"])))
			case "absent":
				m.set(d.Bridge, nil)
			case "outage":
				m.change(func() { m.failure = true })
			}
			_, err := r.Follow(context.Background())
			require.Error(t, err)
			if branch == "chain" || branch == "pin" {
				var mismatch *FollowMismatch
				require.ErrorAs(t, err, &mismatch)
				require.Equal(t, f.Program, mismatch.Found.Program)
				require.Equal(t, "akash-local-1", mismatch.Found.ChainID)
				require.Equal(t, uint64(100), mismatch.Found.Slot)
				event := "bridge-chain-mismatch"
				if branch == "pin" {
					event = "light-client-mismatch"
				}
				require.Equal(t, event, mismatch.Event)
				require.Len(t, m.observed(), 1)
			}
		})
	}
}

func TestOptionalReadAbsenceAndRPCFailure(t *testing.T) {
	for _, name := range []string{"status", "buffer", "buffer-confirmed", "consensus", "consensus-confirmed", "lookup", "lookup-confirmed", "client-nullable", "chunk", "batch", "launch", "bridge", "access"} {
		for _, outage := range []bool{false, true} {
			t.Run(name+map[bool]string{true: "-outage", false: "-absent"}[outage], func(t *testing.T) {
				m, r, _, f := newRPCFixture(t)
				m.change(func() { m.failure = outage })
				var result any
				var err error
				switch name {
				case "status":
					result, err = r.Status(context.Background(), fixtureID(t))
				case "buffer":
					result, err = r.Buffer(context.Background(), keyByte(4))
				case "buffer-confirmed":
					result, err = r.BufferConfirmed(context.Background(), keyByte(4))
				case "consensus":
					result, err = r.Consensus(context.Background(), f, 20)
				case "consensus-confirmed":
					result, err = r.ConsensusConfirmed(context.Background(), f, 20)
				case "lookup":
					result, err = r.Lookup(context.Background(), keyByte(4))
				case "lookup-confirmed":
					result, err = r.LookupConfirmed(context.Background(), keyByte(4))
				case "client-nullable":
					result, err = r.ClientNullable(context.Background(), f)
				case "chunk":
					result, err = r.Chunk(context.Background(), f, keyByte(4), 20, 0)
				case "batch":
					result, err = r.Batch(context.Background(), f, keyByte(4), keyByte(6))
				case "launch":
					result, err = r.Launch(context.Background())
				case "bridge":
					result, err = r.Bridge(context.Background())
				case "access":
					result, err = r.AccessState(context.Background(), f)
				}
				require.Nil(t, result)
				if outage {
					require.ErrorContains(t, err, "injected unavailable")
					require.False(t, errors.Is(err, rpc.ErrNotFound))
				} else {
					require.NoError(t, err)
				}
				calls := m.observed()
				require.Len(t, calls, 1)
				require.Equal(t, "getAccountInfo", calls[0].Method)
			})
		}
	}
}

func TestStatusIdentityAndBatchBoundary(t *testing.T) {
	m, r, d, _ := newRPCFixture(t)
	id := fixtureID(t)
	address, err := d.Status(id)
	require.NoError(t, err)

	good := unhex(t, fixtures(t).Accounts["TransferStatus"])
	m.set(address, accountFixture(d.Program, good))
	status, err := r.Status(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, id, status.ID)

	ids := make([]sink.ID, 101)
	for i := range ids {
		ids[i] = id
	}
	resolved, err := r.Resolved(context.Background(), ids, nil)
	require.NoError(t, err)
	require.Len(t, resolved, 101)

	for _, result := range resolved {
		require.True(t, result)
	}
	calls := m.observed()
	require.Len(t, calls, 3)
	var first, second []string
	require.NoError(t, json.Unmarshal(calls[1].Params[0], &first))
	require.NoError(t, json.Unmarshal(calls[2].Params[0], &second))
	require.Len(t, first, 100)
	require.Len(t, second, 1)

	for _, delta := range []int{-1, 1} {
		m.change(func() { m.batchDelta = delta })
		_, err = r.Resolved(context.Background(), []sink.ID{id}, nil)
		require.ErrorContains(t, err, "batch length")
	}
	m.change(func() { m.batchDelta = 0 })
	bad := bytes.Clone(good)
	bad[8+4] ^= 1
	m.set(address, accountFixture(d.Program, bad))
	_, err = r.Status(context.Background(), id)
	require.ErrorContains(t, err, "identity")
	_, err = r.Resolved(context.Background(), []sink.ID{id}, nil)
	require.ErrorContains(t, err, "identity")

	m.set(address, accountFixture(keyByte(7), good))
	_, err = r.Status(context.Background(), id)
	require.ErrorContains(t, err, "owner")
	resolved, err = r.Resolved(context.Background(), []sink.ID{id}, nil)
	require.NoError(t, err)
	require.Equal(t, []bool{false}, resolved, "baseline foreign status scan")

	m.change(func() { m.failure = true })
	_, err = r.Resolved(context.Background(), []sink.ID{id}, nil)
	require.ErrorContains(t, err, "injected unavailable")
}

func TestCheckedBuffersConsensusChunksAndBatches(t *testing.T) {
	g := fixtures(t)
	for _, kind := range []string{"buffer", "consensus", "chunk", "batch"} {
		t.Run(kind, func(t *testing.T) {
			m, r, d, f := newRPCFixture(t)
			var address solana.PublicKey
			var err error
			var name string
			owner := f.Program
			var readValue func() error
			switch kind {
			case "buffer":
				name, owner = "ProofBuffer", d.Program
				address, err = d.ProofBuffer(keyByte(4), unhex(t, g.NativeKey))
				readValue = func() error {
					v, err := r.Buffer(context.Background(), address)
					if err == nil {
						require.Equal(t, uint64(123), v.Lamports)
					}
					return err
				}
			case "consensus":
				name = "ConsensusStateStore"
				address, err = f.ConsensusAt(20)
				readValue = func() error {
					_, err := r.Consensus(context.Background(), f, 20)
					return err
				}
			case "chunk":
				name = "HeaderChunk"
				address, err = f.ChunkAt(keyByte(4), 20, 0)
				readValue = func() error {
					_, err := r.Chunk(context.Background(), f, keyByte(4), 20, 0)
					return err
				}
			case "batch":
				name = "SignatureBatch"
				address, err = f.BatchAt(keyByte(4), keyByte(6))
				readValue = func() error {
					_, err := r.Batch(context.Background(), f, keyByte(4), keyByte(6))
					return err
				}
			}
			require.NoError(t, err)
			good := unhex(t, g.Accounts[name])
			m.set(address, accountFixture(owner, good))
			require.NoError(t, readValue())
			var options map[string]any
			require.NoError(t, json.Unmarshal(m.observed()[0].Params[1], &options))
			commitment := "confirmed"
			if kind == "buffer" || kind == "consensus" {
				commitment = "finalized"
			}
			require.Equal(t, commitment, options["commitment"])
			m.set(address, accountFixture(keyByte(99), good))
			require.ErrorContains(t, readValue(), "owner")
			bad := bytes.Clone(good)
			bad[8] ^= 1
			m.set(address, accountFixture(owner, bad))
			require.Error(t, readValue(), "stored identity/version mismatch")
			m.set(address, accountFixture(owner, good[:7]))
			require.Error(t, readValue(), "malformed account")
		})
	}
}

func clockData(seconds int64) []byte {
	data := make([]byte, 32)
	return binary.LittleEndian.AppendUint64(data, uint64(seconds))
}

func TestAccessAndActiveUseCurrentClient(t *testing.T) {
	m, r, _, f := newRPCFixture(t)
	g := fixtures(t)
	m.set(f.AccessState, accountFixture(f.AccessManager, unhex(t, g.Accounts["AccessManager"])))
	access, err := r.Access(context.Background(), f, keyByte(4))
	require.NoError(t, err)
	require.Equal(t, AccessMember, access)
	access, err = r.Access(context.Background(), f, keyByte(55))
	require.NoError(t, err)
	require.Equal(t, AccessOpen, access)

	frozen := fixtureClient()
	frozen.TrustingPeriod = 1
	e := encoder(discriminator("account", "ClientState"))
	e.client(frozen)
	m.set(f.ClientState, accountFixture(f.Program, e))
	consensus, err := f.ConsensusAt(20)
	require.NoError(t, err)

	data := unhex(t, g.Accounts["ConsensusStateStore"])
	binary.LittleEndian.PutUint64(data[16:], 1_000_000_000)
	m.set(consensus, accountFixture(f.Program, data))
	m.set(solana.SysVarClockPubkey, accountFixture(solana.SystemProgramID, clockData(1)))
	require.NoError(t, r.Active(context.Background(), f))

	m.set(solana.SysVarClockPubkey, accountFixture(solana.SystemProgramID, clockData(2)))
	require.ErrorContains(t, r.Active(context.Background(), f), "expired")

	m.set(solana.SysVarClockPubkey, accountFixture(solana.SystemProgramID, clockData(math.MinInt64)))
	require.NoError(t, r.Active(context.Background(), f))
	frozen.FrozenHeight.RevisionHeight = 3
	e = encoder(discriminator("account", "ClientState"))
	e.client(frozen)
	m.set(f.ClientState, accountFixture(f.Program, e))
	require.ErrorContains(t, r.Active(context.Background(), f), "frozen")

	m.change(func() { m.failure = true })
	require.ErrorContains(t, r.Active(context.Background(), f), "injected unavailable")
	_, err = r.Access(context.Background(), f, keyByte(4))
	require.ErrorContains(t, err, "injected unavailable")
}

func TestLookupAndSlotHashes(t *testing.T) {
	m, r, _, _ := newRPCFixture(t)
	address := keyByte(20)
	data := encoder{}
	data.u32(1)
	data.u64(math.MaxUint64)
	data.u64(99)
	data.u8(0)
	data.u8(1)
	data.key(keyByte(4))
	data.raw([]byte{0, 0})
	data.key(keyByte(5))
	m.set(address, accountFixture(solana.AddressLookupTableProgramID, data))
	table, err := r.Lookup(context.Background(), address)
	require.NoError(t, err)
	require.Equal(t, []solana.PublicKey{keyByte(5)}, []solana.PublicKey(table.State.Addresses))
	require.Equal(t, keyByte(4), *table.State.Authority)
	require.Equal(t, uint64(123), table.Lamports)
	var options map[string]any
	require.NoError(t, json.Unmarshal(m.observed()[0].Params[1], &options))
	require.Equal(t, "finalized", options["commitment"])

	for _, bad := range [][]byte{data[:55], append(bytes.Clone(data), 1), make([]byte, 56)} {
		m.set(address, accountFixture(solana.AddressLookupTableProgramID, bad))
		_, err := r.Lookup(context.Background(), address)
		require.Error(t, err)
	}
	m.set(address, accountFixture(keyByte(99), data))
	_, err = r.Lookup(context.Background(), address)
	require.ErrorContains(t, err, "owner")

	slots := encoder{}
	slots.u64(2)
	for _, slot := range []uint64{100, 90} {
		slots.u64(slot)
		slots.hash([32]byte{})
	}
	m.set(solana.SysVarSlotHashesPubkey, accountFixture(solana.SystemProgramID, slots))
	got, err := r.SlotHashes(context.Background())
	require.NoError(t, err)
	require.Equal(t, []uint64{100, 90}, got)

	for _, bad := range [][]byte{nil, make([]byte, 8), slots[:len(slots)-1], binary.LittleEndian.AppendUint64(nil, 513)} {
		m.set(solana.SysVarSlotHashesPubkey, accountFixture(solana.SystemProgramID, bad))
		_, err := r.SlotHashes(context.Background())
		require.Error(t, err)
	}
}

func TestConsensusScanChecksFilterOwnerAndPDA(t *testing.T) {
	m, r, _, f := newRPCFixture(t)
	g := fixtures(t)
	address, err := f.ConsensusAt(20)
	require.NoError(t, err)

	m.change(func() {
		m.programAccounts = []*rpc.KeyedAccount{
			{
				Pubkey:  address,
				Account: accountFixture(f.Program, unhex(t, g.Accounts["ConsensusStateStore"])),
			},
			{
				Pubkey:  keyByte(66),
				Account: accountFixture(f.Program, unhex(t, g.Accounts["ConsensusStateStore"])),
			},
		}
	})
	states, err := r.ConsensusStates(context.Background(), f)
	require.NoError(t, err)
	require.Len(t, states, 2)
	require.Equal(t, address, states[0].Address)
	require.Equal(t, keyByte(66), states[1].Address)
	var options map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(m.observed()[0].Params[1], &options))
	var filters []rpc.RPCFilter
	require.NoError(t, json.Unmarshal(options["filters"], &filters))
	require.Equal(t, uint64(88), filters[0].DataSize)
	require.Equal(t, solana.Base58(discriminator("account", "ConsensusStateStore")), filters[1].Memcmp.Bytes)

	m.change(func() { m.programAccounts[0].Account.Owner = keyByte(99) })
	_, err = r.ConsensusStates(context.Background(), f)
	require.ErrorContains(t, err, "owner")

	m.change(func() { m.failure = true })
	_, err = r.ConsensusStates(context.Background(), f)
	require.ErrorContains(t, err, "injected unavailable")
}

func TestClientCommitmentsNullableAndRequired(t *testing.T) {
	m, r, _, f := newRPCFixture(t)
	ctx := context.Background()
	_, err := r.Client(ctx, f)
	require.ErrorContains(t, err, "unavailable")

	_, err = r.ClientFinalized(ctx, f)
	require.ErrorContains(t, err, "unavailable")

	state, err := r.ClientNullable(ctx, f)
	require.NoError(t, err)
	require.Nil(t, state)

	m.set(f.ClientState, accountFixture(f.Program, unhex(t, fixtures(t).Accounts["ClientState"])))
	confirmed, err := r.Client(ctx, f)
	require.NoError(t, err)
	finalized, err := r.ClientFinalized(ctx, f)
	require.NoError(t, err)
	state, err = r.ClientNullable(ctx, f)
	require.NoError(t, err)
	require.Equal(t, confirmed, finalized)
	require.Equal(t, confirmed, *state)

	calls := m.observed()
	for i, commitment := range []string{"confirmed", "finalized", "confirmed", "confirmed", "finalized", "confirmed"} {
		var options map[string]any
		require.NoError(t, json.Unmarshal(calls[i].Params[1], &options))
		require.Equal(t, commitment, options["commitment"])
		_, hasMinimum := options["minContextSlot"]
		require.False(t, hasMinimum, "only app discovery inherits bridge slot")
	}

	m.set(f.ClientState, accountFixture(keyByte(77), unhex(t, fixtures(t).Accounts["ClientState"])))
	_, err = r.ClientNullable(ctx, f)
	require.ErrorContains(t, err, "owner")
}

func TestMalformedRPCResultsAreNotAbsence(t *testing.T) {
	m, r, _, f := newRPCFixture(t)
	m.change(func() {
		m.changeResult = func(_ rpcCall, _ any) any { return nil }
	})
	_, err := r.Status(context.Background(), fixtureID(t))
	require.Error(t, err)
	_, err = r.Resolved(context.Background(), []sink.ID{fixtureID(t)}, nil)
	require.Error(t, err)
	_, err = r.ConsensusStates(context.Background(), f)
	require.Error(t, err)
}

func TestOptionalAccountRequiresCompleteRPCResponse(t *testing.T) {
	cases := map[string]any{
		"null-result":     nil,
		"missing-value":   map[string]any{"context": map[string]any{"slot": 100}},
		"missing-context": map[string]any{"value": nil},
		"missing-slot":    map[string]any{"context": map[string]any{}, "value": nil},
		"null-slot":       map[string]any{"context": map[string]any{"slot": nil}, "value": nil},
	}
	for name, response := range cases {
		t.Run(name, func(t *testing.T) {
			m, r, _, _ := newRPCFixture(t)
			m.change(func() {
				m.changeResult = func(rpcCall, any) any { return response }
			})
			lookup, err := r.Lookup(context.Background(), keyByte(4))
			require.Error(t, err, "malformed response must not mark a lookup absent")
			require.Nil(t, lookup)
		})
	}
}

func TestAbsentAccountStillRequiresRequestedContext(t *testing.T) {
	m, r, _, _ := newRPCFixture(t)
	minimum := uint64(101)
	account, _, err := r.Account(context.Background(), keyByte(4), rpc.CommitmentConfirmed, &minimum)
	require.ErrorContains(t, err, "precedes")
	require.Nil(t, account)
	m.change(func() { m.slot = minimum })
	account, slot, err := r.Account(context.Background(), keyByte(4), rpc.CommitmentConfirmed, &minimum)
	require.NoError(t, err)
	require.Nil(t, account)
	require.Equal(t, minimum, slot)
	m.change(func() { m.slot = 0 })
	account, slot, err = r.Account(context.Background(), keyByte(4), rpc.CommitmentFinalized, nil)
	require.NoError(t, err)
	require.Nil(t, account)
	require.Zero(t, slot, "explicit genesis slot is valid")
}

func TestAccountBatchRequiresCompleteRPCResponse(t *testing.T) {
	cases := map[string]any{
		"null-result":     nil,
		"missing-context": map[string]any{"value": []any{nil}},
		"missing-slot":    map[string]any{"context": map[string]any{}, "value": []any{nil}},
		"missing-value":   map[string]any{"context": map[string]any{"slot": 100}},
		"null-value":      map[string]any{"context": map[string]any{"slot": 100}, "value": nil},
	}
	for name, response := range cases {
		t.Run(name, func(t *testing.T) {
			m, r, _, _ := newRPCFixture(t)
			m.change(func() {
				m.changeResult = func(rpcCall, any) any { return response }
			})
			resolved, err := r.Resolved(context.Background(), []sink.ID{fixtureID(t)}, nil)
			require.Error(t, err, "malformed batch must not imply all statuses unresolved")
			require.Nil(t, resolved)
		})
	}
	_, r, _, _ := newRPCFixture(t)
	resolved, err := r.Resolved(context.Background(), []sink.ID{fixtureID(t)}, nil)
	require.NoError(t, err)
	require.Equal(t, []bool{false}, resolved)
}
