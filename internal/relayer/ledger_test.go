package relayer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/native"
)

func TestDurableLedgerLegacySignedSpendRetryAndDedupe(t *testing.T) {
	store := durableStore(t)
	id, _ := native.ParseID(durableIdentity(t).ID)
	writeFixture(t, store.ledgerPath, durableFixture(t, "ts-ledger.json"))
	ledger, err := store.readLedger()
	if err != nil {
		t.Fatal(err)
	}
	if ledger.spent(id) != -7000 || ledger.entries[id].Attempts != 1 || ledger.ready(id, 1700000005999) || !ledger.ready(id, 1700000006000) {
		t.Fatal("legacy signed spend or millisecond retry changed")
	}
	if err := ledger.charge(id, 100000, "retained-charge", 900000); err != nil || ledger.spent(id) != -7000 {
		t.Fatal("signature charged twice")
	}
	if err := ledger.charge(id, 17000, "new-charge", 20000); err != nil {
		t.Fatal(err)
	}
	if ledger.exhausted(id, 10000) || !ledger.exhausted(id, 9999) {
		t.Fatal("budget threshold must be strict greater-than")
	}
	failures, delay, err := ledger.failed(id, 1700000006000, 9000)
	if err != nil || failures != 2 || delay != 9000 {
		t.Fatalf("retry %d %d %v", failures, delay, err)
	}
	ledger.complete(id)
	doc, err := json.Marshal(ledger.document(store.identity.Genesis, store.identity.ProgramID))
	if err != nil || bytes.Contains(doc, []byte(`"retry"`)) {
		t.Fatalf("retry must be omitted: %s %v", doc, err)
	}
	if err := ledger.attempt(id); err != nil || ledger.entries[id].Completed != nil {
		t.Fatal("attempt did not clear completion")
	}
	entry := ledger.entries[id]
	if int64Value(entry.BufferFundedLamports) != 20000 || int64Value(entry.BufferReclaimedLamports) != 9000 {
		t.Fatalf("wrong liquidity %+v", entry)
	}
	if changed, err := ledger.reset(id.String()); err != nil || !changed {
		t.Fatal("reset ID")
	}
	if changed, err := ledger.reset("all"); err != nil || changed {
		t.Fatal("reset empty ledger")
	}
}

func TestDurableLedgerOverflowAndInvalidSchema(t *testing.T) {
	store := durableStore(t)
	id, _ := native.ParseID(durableIdentity(t).ID)
	ledger, _ := store.readLedger()
	if err := ledger.charge(id, maxSafeInteger, "max", maxSafeInteger); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(ledger.document("g", "p"))
	for _, attempt := range []func() error{
		func() error { return ledger.charge(id, 1, "overflow", 0) },
		func() error { return ledger.charge(id, 0, "liquidity-overflow", 1) },
		func() error {
			_, _, err := ledger.failed(id, maxSafeInteger, 1000)
			return err
		},
	} {
		if err := attempt(); err == nil {
			t.Fatal("overflow accepted")
		}
		after, _ := json.Marshal(ledger.document("g", "p"))
		if !bytes.Equal(before, after) {
			t.Fatal("overflow partially mutated ledger")
		}
	}
	for _, raw := range []string{
		`{"spentLamports":0,"attempts":0,"retry":null}`,
		`{"spentLamports":0,"attempts":0,"retry":{"failures":1}}`,
		`{"spentLamports":0,"attempts":0,"completed":null}`,
		`{"spentLamports":0,"attempts":0,"signatures":[null]}`,
		`{"spentLamports":9007199254740992,"attempts":0}`,
		`{"spentLamports":0,"attempts":-1}`,
		`{"spentLamports":0}`,
	} {
		data, _ := json.Marshal(map[string]any{
			"version":   2,
			"genesis":   store.identity.Genesis,
			"programId": store.identity.ProgramID,
			"transfers": map[string]json.RawMessage{id.String(): json.RawMessage(raw)},
		})
		if _, err := parseSpendLedger(data, store.identity.Genesis, store.identity.ProgramID); err == nil {
			t.Fatalf("invalid ledger accepted: %s", raw)
		}
	}
}

func TestDurableLedgerLockedUpdatesAndChargeAccounting(t *testing.T) {
	store := durableStore(t)
	id, _ := native.ParseID(durableIdentity(t).ID)
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < cap(errs); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
				return true, ledger.attempt(id)
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	ledger, err := store.readLedger()
	if err != nil || ledger.entries[id].Attempts != 12 {
		t.Fatalf("lost locked update %v %v", ledger, err)
	}
	var paymentLog bytes.Buffer
	svc := &service{
		store: store,
		deps: dependencies{
			streams: Streams{
				Out: &paymentLog,
			},
		},
	}
	acquired, err := svc.withPayer(context.Background(), func(payer *payerSession) error {
		for _, charge := range []struct {
			stage string
			delta int64
		}{
			{"proof-create", 1005000},
			{"proof-grow", 205000},
			{"proof-close", -1195000},
			{"resolve", 1005000},
		} {
			result := txResult{
				stage:         charge.stage,
				signature:     charge.stage,
				fee:           5000,
				payerLamports: &charge.delta,
			}
			if err := payer.chargeResult(context.Background(), id, result); err != nil {
				return err
			}
			if err := payer.chargeResult(context.Background(), id, result); err != nil {
				return err
			}
		}
		return nil
	})
	if !acquired || err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(paymentLog.Bytes(), []byte(`"bufferChangeLamports":-1200000`)) || !bytes.Contains(paymentLog.Bytes(), []byte(`"event":"relay-payment"`)) {
		t.Fatalf("missing payment liquidity event: %s", paymentLog.Bytes())
	}
	ledger, _ = store.readLedger()
	entry := ledger.entries[id]
	if entry.SpentLamports != 1020000 || int64Value(entry.BufferFundedLamports) != 1200000 || int64Value(entry.BufferReclaimedLamports) != 1200000 || len(entry.Signatures) != 4 {
		t.Fatalf("wrong accounting %+v", entry)
	}
	before, _ := os.ReadFile(store.ledgerPath)
	failure := errors.New("test callback failure")
	if _, err := store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
		_, _ = ledger.reset("all")
		return true, failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(store.ledgerPath)
	if !bytes.Equal(before, after) {
		t.Fatal("failed mutation persisted")
	}
}

func TestDurableLedgerLargeConfiguredRetryCeiling(t *testing.T) {
	store := durableStore(t)
	id, _ := native.ParseID(durableIdentity(t).ID)
	ledger, err := store.readLedger()
	if err != nil {
		t.Fatal(err)
	}
	for i := int64(1); i <= 33; i++ {
		failures, delay, err := ledger.failed(id, 0, maxSafeInteger*1000)
		if err != nil || failures != i || delay != int64(6000)<<min(i-1, 30) {
			t.Fatalf("large local retry ceiling: %d %d %v", failures, delay, err)
		}
	}
}

func TestDurableLedgerInsertionOrderAndSortedPersistence(t *testing.T) {
	store := durableStore(t)
	first, _ := native.ParseID(durableIdentity(t).ID)
	first.Sequence = 9
	second := first
	second.Sequence = 2
	third := first
	third.Sequence = 1
	entry := `{"spentLamports":0,"attempts":0}`
	data := []byte(fmt.Sprintf(`{"version":2,"genesis":%q,"programId":%q,"transfers":{%q:{"spentLamports":"discarded invalid duplicate"},%q:%s,%q:{"spentLamports":0,"attempts":7}}}`, store.identity.Genesis, store.identity.ProgramID, first.String(), second.String(), entry, first.String()))
	ledger, err := parseSpendLedger(data, store.identity.Genesis, store.identity.ProgramID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ledger.ids(), []sink.ID{first, second}) || ledger.entries[first].Attempts != 7 {
		t.Fatalf("duplicate property semantics changed: %v", ledger.ids())
	}
	if err := ledger.attempt(first); err != nil {
		t.Fatal(err)
	}
	if err := ledger.charge(third, 5, "first-charge", 0); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ledger.ids(), []sink.ID{first, second, third}) {
		t.Fatal("in-memory insertion order lost")
	}
	if _, err := ledger.reset(first.String()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ledger.failed(first, 0, 10000); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ledger.ids(), []sink.ID{second, third, first}) {
		t.Fatal("delete/reinsert position lost")
	}
	copyIDs := ledger.ids()
	copyIDs[0] = first
	if ledger.ids()[0] != second {
		t.Fatal("caller can mutate insertion order")
	}
	encoded, err := json.Marshal(ledger.document(store.identity.Genesis, store.identity.ProgramID))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := parseSpendLedger(encoded, store.identity.Genesis, store.identity.ProgramID)
	if err != nil || !reflect.DeepEqual(loaded.ids(), []sink.ID{third, second, first}) {
		t.Fatalf("baseline lexical save order changed: %v %v", loaded, err)
	}
	if _, err := ledger.reset("all"); err != nil || len(ledger.ids()) != 0 {
		t.Fatal("reset-all retained IDs")
	}
}

func TestDurableLegacyTSReload(t *testing.T) {
	modules := os.Getenv("RELAYER_TEST_TS_MODULES")
	if modules == "" {
		t.Skip("set RELAYER_TEST_TS_MODULES to installed baseline TypeScript dependencies for cross-language reload")
	}
	store := durableStore(t)
	identity := durableIdentity(t)
	id, _ := native.ParseID(identity.ID)
	ledger, err := parseSpendLedger(durableFixture(t, "ts-ledger.json"), identity.Genesis, identity.ProgramID)
	if err != nil {
		t.Fatal(err)
	}
	other := id
	other.Sequence = 2
	if err := ledger.attempt(other); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(ledger.document(identity.Genesis, identity.ProgramID))
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := parseSpendLedger(encoded, identity.Genesis, identity.ProgramID)
	if err != nil {
		t.Fatal(err)
	}
	proofs := []*proofJob{}
	for _, name := range []string{"ts-proof-transfer.json", "ts-proof-settlement.json"} {
		job, err := parseProofJob(durableFixture(t, name), store.identity, id)
		if err != nil {
			t.Fatal(err)
		}
		proofs = append(proofs, job)
	}
	input, err := json.Marshal(struct {
		Identity durableFixtureIdentity `json:"identity"`
		Ledger   json.RawMessage        `json:"ledger"`
		Proofs   []*proofJob            `json:"proofs"`
	}{
		Identity: identity,
		Ledger:   encoded,
		Proofs:   proofs,
	})
	if err != nil {
		t.Fatal(err)
	}
	root := testRepositoryRoot(t)
	script := `const fs=require('node:fs'); const {SpendLedger}=require(process.argv[1]); const {parseProofJob}=require(process.argv[2]); const body=JSON.parse(fs.readFileSync(0,'utf8')); const ledger=SpendLedger.parse(body.ledger,body.identity); for(const job of body.proofs) parseProofJob(job,body.identity,job.id); process.stdout.write(JSON.stringify({ids:[...ledger.entries.keys()],ledger:ledger.toJSON(body.identity)}));`
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "--require", filepath.Join(modules, "tsx/dist/cjs/index.cjs"), "-e", script, filepath.Join(root, "contracts/solana/local/relay-policy.ts"), filepath.Join(root, "contracts/solana/local/proof-job.ts"))
	cmd.Env = append(os.Environ(), "NODE_PATH="+modules)
	cmd.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("actual TS reload failed: %v %s", err, stderr.String())
	}
	var result struct {
		IDs    []string        `json:"ids"`
		Ledger json.RawMessage `json:"ledger"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("TS reload output: %s %v", output, err)
	}
	want := []string{}
	for _, id := range reloaded.ids() {
		want = append(want, id.String())
	}
	if !reflect.DeepEqual(result.IDs, want) {
		t.Fatalf("TS reload order %v != Go %v", result.IDs, want)
	}
	fromTS, err := parseSpendLedger(result.Ledger, identity.Genesis, identity.ProgramID)
	if err != nil || !reflect.DeepEqual(fromTS.ids(), reloaded.ids()) || fromTS.spent(id) != -7000 {
		t.Fatalf("TS second round-trip failed: %v", err)
	}
}
