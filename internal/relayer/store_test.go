package relayer

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pkg.akt.dev/node/v2/internal/relayer/native"
)

//go:embed testdata/recovery/*.json
var durableFixtures embed.FS

type durableFixtureIdentity struct {
	ChainID   string `json:"chainId"`
	Genesis   string `json:"genesis"`
	ProgramID string `json:"programId"`
	Payer     string `json:"payer"`
	ID        string `json:"id"`
	Blockhash string `json:"blockhash"`
}

func durableFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := durableFixtures.ReadFile("testdata/recovery/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func durableIdentity(t *testing.T) durableFixtureIdentity {
	t.Helper()
	var identity durableFixtureIdentity
	if err := json.Unmarshal(durableFixture(t, "identity.json"), &identity); err != nil {
		t.Fatal(err)
	}
	return identity
}

func durableStore(t *testing.T) *stateStore {
	t.Helper()
	identity := durableIdentity(t)
	store, err := newStateStore(durableTestDirectory(t), identity.ChainID, identity.Genesis, identity.ProgramID, identity.Payer, nil)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func writeFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := atomicJSON(path, data); err != nil {
		t.Fatal(err)
	}
}

func TestDurableStorePathsAndAtomicReplacement(t *testing.T) {
	store := durableStore(t)
	identity := durableIdentity(t)
	id, err := native.ParseID(identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.paymentPath(id)
	if err != nil {
		t.Fatal(err)
	}
	var proof proofJob
	if err := json.Unmarshal(durableFixture(t, "ts-proof-transfer.json"), &proof); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != proof.Nonce+".json" || filepath.Dir(path) != store.jobDirectory {
		t.Fatalf("wrong payment path %s", path)
	}
	root := filepath.Dir(store.ledgerPath)
	if store.payerLock != filepath.Join(root, "header-uploads-v2", identity.Genesis, identity.ProgramID, "payer.lock") || store.ledgerLock != filepath.Join(root, "relay-budget.lock") {
		t.Fatal("legacy lock paths changed")
	}
	writeFixture(t, path, []byte("{\"before\":true}\n"))
	writeFixture(t, path, []byte("{\"after\":true}\n"))
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "{\"after\":true}\n" {
		t.Fatalf("replacement %s %v", data, err)
	}
	for target, mode := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
		info, err := os.Stat(target)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("mode %s: %v %v", target, info, err)
		}
	}
	if err := atomicJSON(filepath.Dir(path), []byte("{}\n")); err == nil {
		t.Fatal("rename onto directory accepted")
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".tmp") {
			t.Fatal("disposable atomic temporary retained")
		}
	}
	if data, err := optionalJSON(path + ".absent"); data != nil || err != nil {
		t.Fatal("absent JSON not optional")
	}
	writeFixture(t, path, []byte("{"))
	if _, err := optionalJSON(path); err == nil {
		t.Fatal("malformed JSON treated as absent")
	}
	if _, err := optionalJSON(filepath.Dir(path)); err == nil {
		t.Fatal("directory read treated as absent")
	}
}

func TestDurableSessionAndActivePreserveExtensions(t *testing.T) {
	store := durableStore(t)
	sessionID := strings.Repeat("ab", 32)
	lookup := store.identity.ProgramID
	slot := int64(123)
	doc := &sessionDoc{
		Version:    1,
		Genesis:    store.identity.Genesis,
		Payer:      store.identity.Payer,
		Client:     store.identity.ProgramID,
		Target:     20,
		Trusted:    10,
		Session:    sessionID,
		Lookup:     &lookup,
		LookupSlot: &slot,
		Assembly: &transactionDoc{
			Stage:     "assembly",
			Signature: "",
			Error:     json.RawMessage(`{"InstructionError":[1,"InvalidArgument"]}`),
			Extra:     map[string]json.RawMessage{"futureResult": json.RawMessage(`false`)},
		},
		Extra: map[string]json.RawMessage{"futureCleanup": json.RawMessage(`{"evidence":[1,null,"kept"]}`)},
	}
	if err := store.saveSession(sessionID, doc); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.loadSession(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Lookup = nil
	loaded.LookupSlot = nil
	closed := true
	loaded.LookupClosed = &closed
	if err := store.saveSession(sessionID, loaded); err != nil {
		t.Fatal(err)
	}
	path, _ := store.sessionPath(sessionID, ".json")
	data, _ := os.ReadFile(path)
	if bytes.Contains(data, []byte(`"lookup":`)) || !bytes.Contains(data, []byte(`"futureCleanup":`)) || !bytes.Contains(data, []byte(`"futureResult":false`)) {
		t.Fatalf("overlay lost extension or failed omission: %s", data)
	}
	ids, err := store.sessionIDs()
	if err != nil || len(ids) != 1 || ids[0] != sessionID {
		t.Fatalf("sessions %v %v", ids, err)
	}
	loaded.Genesis = "wrong"
	if err := store.saveSession(sessionID, loaded); err == nil {
		t.Fatal("wrong deployment accepted")
	}
	if _, err := store.sessionPendingPath("../escape"); err == nil {
		t.Fatal("unsafe session ID accepted")
	}
	packet, err := json.Marshal(headerPacketDoc{
		ChainID: store.identity.ChainID,
		Header:  "abcd",
		Consensus: headerConsensusDoc{
			Timestamp:          "18446744073709551615",
			Root:               strings.Repeat("01", 32),
			NextValidatorsHash: strings.Repeat("02", 32),
			Extra:              map[string]json.RawMessage{"futureConsensus": json.RawMessage(`42`)},
		},
		Signatures: []headerSignatureDoc{},
		Extra:      map[string]json.RawMessage{"futurePacket": json.RawMessage(`[]`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	active := &activeHeaderDoc{
		ChainID: store.identity.ChainID,
		Genesis: store.identity.Genesis,
		Client:  store.identity.ProgramID,
		Target:  20,
		Trusted: 10,
		Packet:  packet,
		Extra:   map[string]json.RawMessage{"futureJob": json.RawMessage(`"retained"`)},
	}
	if err := store.saveActive(active); err != nil {
		t.Fatal(err)
	}
	active, err = store.loadActive()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeActivePacket(active.Packet, store.identity.ChainID)
	if err != nil || string(decoded.Consensus.Extra["futureConsensus"]) != "42" || string(decoded.Extra["futurePacket"]) != "[]" || string(active.Extra["futureJob"]) != `"retained"` {
		t.Fatalf("active extension: %v", err)
	}
	abandoned, err := store.abandonActive(headerHeight(active.Target), time.UnixMilli(1700000000000))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(abandoned) != "abandoned-20-1700000000000.json" {
		t.Fatal(abandoned)
	}
	if loaded, err := store.loadActive(); loaded != nil || err != nil {
		t.Fatal("abandoned job still active")
	}
	if err := store.appendHeaderEvent(map[string]string{"event": "test"}); err != nil {
		t.Fatal(err)
	}
}

func TestDurableDiscoveryLegacyTailAndStrictCursor(t *testing.T) {
	store := durableStore(t)
	writeFixture(t, store.discoveryPath, durableFixture(t, "ts-discovery.json"))
	cursor, err := store.loadDiscovery()
	if err != nil || cursor == nil || cursor.Kind != "tail" || !bytes.Equal(cursor.Key, []byte{0, 1, 255}) {
		t.Fatalf("legacy cursor %v %v", cursor, err)
	}
	if err := store.saveDiscovery(cursor); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(store.discoveryPath)
	if bytes.Contains(data, []byte(`"kind"`)) {
		t.Fatal("new cursor key breaks TS strict schema")
	}
	cursor.Kind = "next"
	if err := store.saveDiscovery(cursor); err == nil {
		t.Fatal("persisted an incomplete discovery traversal")
	}
	for _, bad := range []string{
		strings.Replace(string(data), `"cursor":{`, `"cursor":{"kind":"tail",`, 1),
		strings.Replace(string(data), `"AAH/"`, `"?bad"`, 1),
		strings.Replace(string(data), `"version":3`, `"version":2`, 1),
	} {
		writeFixture(t, store.discoveryPath, []byte(bad))
		if _, err := store.loadDiscovery(); err == nil {
			t.Fatalf("bad discovery accepted: %s", bad)
		}
	}
	if err := store.saveDiscovery(nil); err != nil {
		t.Fatal(err)
	}
	if cursor, err := store.loadDiscovery(); cursor != nil || err != nil {
		t.Fatalf("null cursor %v %v", cursor, err)
	}
}

func TestDurableInjectedWriterFailureRetainsPrevious(t *testing.T) {
	store := durableStore(t)
	writeFixture(t, store.discoveryPath, durableFixture(t, "ts-discovery.json"))
	before, _ := os.ReadFile(store.discoveryPath)
	failure := errors.New("test durable storage failure")
	store.writeJSON = func(string, []byte) error { return failure }
	if err := store.saveDiscovery(nil); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(store.discoveryPath)
	if !bytes.Equal(before, after) {
		t.Fatal("writer failure changed committed state")
	}
}

func TestResolutionCheckpointLegacyShape(t *testing.T) {
	h := newResolutionHarness(t)
	if err := h.s.store.saveCheckpoint(42, "block", "root"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(h.s.store.ledgerPath), "checkpoint.json"))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if len(value) != 4 || value["chainId"] != h.s.cfg.chainID || value["height"] != float64(42) || value["blockHash"] != "block" || value["root"] != "root" {
		t.Fatalf("legacy checkpoint shape changed: %s", data)
	}
}

func TestDurableActivePacketDeferredUntilSameClient(t *testing.T) {
	for _, packet := range []json.RawMessage{
		nil,
		json.RawMessage(`null`),
		json.RawMessage(`"wrong type"`),
		json.RawMessage(`[1,2]`),
		json.RawMessage(`{"chainId":"wrong","header":"no"}`),
	} {
		t.Run(string(packet), func(t *testing.T) {
			store := durableStore(t)
			fields := map[string]any{
				"chainId": store.identity.ChainID,
				"genesis": store.identity.Genesis,
				"client":  "",
				"target":  20,
				"trusted": 10,
				"future":  true,
			}
			if packet != nil {
				fields["packet"] = packet
			}
			raw, err := json.MarshalIndent(fields, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			writeFixture(t, store.activePath, raw)
			active, err := store.loadActive()
			if err != nil {
				t.Fatal(err)
			}
			var original map[string]json.RawMessage
			if err := json.Unmarshal(raw, &original); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(active.Packet, original["packet"]) || string(active.Extra["future"]) != "true" {
				t.Fatal("opaque packet or extension changed")
			}
			if err := store.saveActive(active); err == nil {
				t.Fatal("new malformed active packet accepted")
			}
			unchanged, err := os.ReadFile(store.activePath)
			if err != nil || !bytes.Equal(unchanged, raw) {
				t.Fatalf("invalid save changed durable bytes: %v", err)
			}
		})
	}
}
