package relayer

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExpiryTypeScriptRollback(t *testing.T) {
	modules := os.Getenv("RELAYER_TEST_TS_MODULES")
	if modules == "" {
		t.Skip("set RELAYER_TEST_TS_MODULES to installed baseline TypeScript dependencies for cross-language rollback")
	}
	modules, err := filepath.Abs(modules)
	if err != nil {
		t.Fatal(err)
	}
	f := newExpiryFixture(t, "proof-append")
	_, err = f.service.store.updateLedger(context.Background(), func(l *spendLedger) (bool, error) {
		if err := l.attempt(f.id); err != nil {
			return false, err
		}
		return true, l.charge(f.id, 15000, "earlier-observed", 100000)
	})
	if err != nil {
		t.Fatal(err)
	}
	root := testRepositoryRoot(t)
	script := filepath.Join(durableTestDirectory(t), "rollback.cjs")
	code := `const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const {createRequire} = require('node:module');
const [root, modules, pending, ledgerPath, mode] = process.argv.slice(2);
const local = createRequire(path.join(root, 'contracts/solana/package.json'));
require(path.join(modules, 'tsx/dist/cjs/index.cjs'));
const {web3} = require(path.join(modules, '@coral-xyz/anchor'));
const {resumeTransaction} = local('./local/transactions.ts');
const {parseProofJob} = local('./local/proof-job.ts');
const {SpendLedger} = local('./local/relay-policy.ts');
const {parseSinkId} = local('./shared/id.ts');
(async () => {
  const proofBytes = await fs.readFile(pending+'.proof');
  const ledgerBytes = await fs.readFile(ledgerPath);
  const proof = JSON.parse(proofBytes);
  const identity = {chainId:proof.chainId, genesis:proof.genesis, programId:proof.programId, payer:proof.payer};
  const id = parseSinkId(proof.id);
  assert.deepEqual(parseProofJob(proof, identity, id), proof);
  const ledger = SpendLedger.parse(JSON.parse(ledgerBytes), {genesis:proof.genesis, programId:proof.programId});
  assert.equal(ledger.spent(id), 15000);
  assert.equal(ledger.entries.get(id).attempts, 1);
  assert.deepEqual(ledger.entries.get(id).signatures, ['earlier-observed']);
  let reads = 0, observed = 0;
  const wallet = {
    publicKey:new web3.PublicKey(proof.payer),
    connection:{
      getGenesisHash:async () => { reads++; return proof.genesis; },
      getSignatureStatuses:async () => { reads++; return {value:[null]}; },
      getBlockHeight:async () => { reads++; return 1001; },
    },
  };
  const resume = () => resumeTransaction(wallet, pending, async () => { observed++; });
  if (mode === 'retained') {
    await assert.rejects(resume, /expired transaction history unresolved/);
    assert.equal(reads, 3);
  } else {
    await resume();
    assert.equal(reads, 0);
    assert.equal((await fs.readdir(path.dirname(pending))).filter(name => name.endsWith('.json')).length, 0);
  }
  assert.equal(observed, 0);
  assert.deepEqual(await fs.readFile(pending+'.proof'), proofBytes);
  assert.deepEqual(await fs.readFile(ledgerPath), ledgerBytes);
  process.stdout.write('real TS rollback '+mode+' passed\n');
})().catch(error => { process.stderr.write(String(error)+'\n'); process.exitCode=1; });
`
	if err := os.WriteFile(script, []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("configured TypeScript rollback requires installed Node: %v", err)
	}
	run := func(mode string) {
		t.Helper()
		command := exec.Command(node, script, root, modules, f.path, f.service.store.ledgerPath, mode)
		command.Dir = root
		command.Env = []string{
			"PATH=" + os.Getenv("PATH"),
			"TMPDIR=" + os.Getenv("TMPDIR"),
			"NODE_PATH=" + modules,
			"TSX_DISABLE_CACHE=1",
			"AKT_LOCAL_PROFILE=single",
		}
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("real TS rollback %s: %v: %s", mode, err, output)
		}
		if !bytes.Contains(output, []byte("real TS rollback "+mode+" passed")) {
			t.Fatal("TypeScript reload did not complete")
		}
	}
	run("retained")
	if err := f.resume(context.Background()); !isIntentNotIncluded(err) {
		t.Fatal(err)
	}
	run("retired")
}
