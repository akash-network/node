import assert from "node:assert/strict";
import test from "node:test";
import { web3 } from "@coral-xyz/anchor";
import {
  Observation,
  Outcome,
  Settlement,
  Sink_Params as Params,
  Sink_Status as Status,
  Totals,
  type TransferState,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import {
  OPEN_PAGE_LIMIT,
  Scanner,
  type ScanAkash,
  type ScanObserver,
} from "./scan";
import {
  LightClientMismatchError,
  type FinalizedObserved,
  type LightClientChange,
  type Observed,
  type Record as SinkRecord,
} from "./solana";

const members = ["reporter", "b", "c", "d"];
const { STATUS_PENDING: P, STATUS_VOIDED: V, STATUS_MINTED: M } = Status;
const OPEN = [P, V];

function transfer(id: bigint, status: Status): TransferState {
  const record = Buffer.alloc(77);
  record[0] = 1;
  record.writeBigUInt64LE(id, 1);
  record.writeBigUInt64LE(60n, 61);
  record.writeBigUInt64LE(1n, 69);
  return {
    record,
    voidOrdered: false,
    settlement: Settlement.fromPartial({
      round: 1n,
      status,
      policy: { epoch: 1n, members, threshold: 3 },
    }),
  };
}

// statuses[i] belongs to transfer i + 1. `open: false` mimics an older node.
function ledger(statuses: Status[], { open = true } = {}) {
  const status = (id: bigint) => statuses[Number(id) - 1] ?? P;
  const openIds = statuses.flatMap((value, i) =>
    OPEN.includes(value) ? [BigInt(i + 1)] : [],
  );
  const state = {
    params: Params.fromPartial({ reporters: members, reporterThreshold: 3 }),
    totals: Totals.fromPartial({
      reporterEpoch: 1n,
      lastTransferId: BigInt(statuses.length),
      pendingCount: BigInt(openIds.length),
    }),
    height: 10,
    time: 86_400n,
  };
  const reads: bigint[] = [];
  const pages: bigint[] = [];
  const akash: ScanAkash & { stateCalls: number; transferCalls: number } = {
    stateCalls: 0,
    transferCalls: 0,
    async state() {
      akash.stateCalls++;
      return state;
    },
    async transfer(id) {
      akash.transferCalls++;
      return transfer(id, status(id));
    },
    async snapshot(id) {
      reads.push(id);
      return { ...state, transfer: transfer(id, status(id)) };
    },
    async openTransfers(startAfter, limit, height) {
      assert.equal(height, 10);
      if (!open) return undefined;
      pages.push(startAfter);
      const rest = openIds.filter((id) => id > startAfter);
      const page = rest.slice(0, limit);
      return {
        transfers: page.map((id) => transfer(id, status(id))),
        next: rest.length > limit ? page[page.length - 1]! : 0n,
      };
    },
  };
  return { akash, reads, pages, state };
}

class FakeObserver implements ScanObserver {
  changes: LightClientChange[] = [];
  program = web3.Keypair.generate().publicKey;
  // Finalized Solana outcomes by transfer ID; others have no status.
  solana = new Map<bigint, Outcome>();
  evidence: bigint[] = [];
  failAt?: bigint;
  mismatch?: LightClientMismatchError;
  async lightClient() {
    if (this.mismatch) throw this.mismatch;
    return this.program;
  }
  takeLightClientChanges() {
    return this.changes.splice(0);
  }
  async observeFinalized(record: SinkRecord): Promise<FinalizedObserved> {
    const outcome = this.solana.get(record.id);
    if (outcome === undefined) return { kind: "absent", contextSlot: 100n };
    return {
      kind: "resolved",
      contextSlot: 100n,
      address: web3.PublicKey.default,
      status: undefined as never,
      outcome,
      slot: 50n,
      proofHeight: 2n,
    };
  }
  async observe(record: SinkRecord): Promise<Observed> {
    if (record.id === this.failAt && this.mismatch) throw this.mismatch;
    this.evidence.push(record.id);
    const outcome = this.solana.get(record.id);
    if (outcome === undefined)
      return { kind: "absent", contextSlot: 100n, slotLimit: 700n };
    return {
      kind: "resolved",
      contextSlot: 100n,
      observation: Observation.fromPartial({
        outcome,
        slot: 50n,
        transactionSignature: new Uint8Array(64).fill(1),
      }),
    };
  }
}

function scanner(
  akash: ScanAkash,
  observer: ScanObserver,
  options: { maxTransfers?: number; auditPerScan?: number } = {},
) {
  const lines: Record<string, unknown>[] = [];
  const instance = new Scanner(akash, observer, {
    reporter: "reporter",
    maxTransfers: options.maxTransfers ?? 100,
    auditPerScan: options.auditPerScan ?? 100,
    output: (value: unknown) => lines.push(value as Record<string, unknown>),
    signal: new AbortController().signal,
  });
  return {
    lines,
    async run() {
      lines.length = 0;
      return { failures: await instance.scan(), lines: [...lines] };
    },
  };
}

test("a pinned light-client mismatch stops the scan with an alert", async () => {
  const { akash, reads } = ledger([P, P, P]);
  const observer = new FakeObserver();
  const pinned = observer.program;
  const current = web3.Keypair.generate().publicKey;
  observer.mismatch = new LightClientMismatchError(pinned, current);
  const scan = scanner(akash, observer);
  const before = await scan.run();
  assert.equal(before.failures, 1);
  assert.equal(akash.stateCalls, 0);
  assert.deepEqual(before.lines[0], {
    alert: "light-client-mismatch",
    pinned: pinned.toBase58(),
    current: current.toBase58(),
  });
  assert.match(String(before.lines[1]?.error), /differs from pinned/);
  assert.deepEqual(reads, []);

  // A re-pin observed mid-scan stops every later observation.
  observer.lightClient = async () => pinned;
  observer.failAt = 2n;
  const during = await scan.run();
  assert.equal(during.failures, 1);
  assert.deepEqual(reads, [1n, 2n]);
  assert.deepEqual(
    during.lines.map((line) => line.alert ?? line.id ?? line.error),
    [1n, "light-client-mismatch", during.lines[2]?.error],
  );
  assert.ok(!during.lines.some((line) => "scanned" in line));
});

test("an unpinned scan alerts on each light-client change and continues", async () => {
  const { akash, reads } = ledger([P, P]);
  const observer = new FakeObserver();
  const previous = observer.program;
  const current = web3.Keypair.generate().publicKey;
  const observe = observer.observe.bind(observer);
  observer.observe = async (record) => {
    if (record.id === 1n)
      observer.changes.push({ previous, current, contextSlot: 101n });
    return observe(record);
  };
  const { failures, lines } = await scanner(akash, observer).run();
  assert.equal(failures, 0);
  assert.deepEqual(reads, [1n, 2n]);
  // The change is reported before any decision made under it.
  assert.deepEqual(lines[0], {
    alert: "light-client-changed",
    previous: previous.toBase58(),
    current: current.toBase58(),
    slot: 101n,
  });
  assert.equal(lines[1]?.id, 1n);
  assert.equal(lines[2]?.id, 2n);
  assert.deepEqual(lines.at(-1), {
    scanned: 2,
    audited: 0,
    index: "open-transfers",
    failures: 0,
    lightClientProgram: previous.toBase58(),
  });
});

test("open transfers replace the full scan and lift maxTransfers", async () => {
  // 1,200 transfers; 553 open across two pages, two of them voided.
  const statuses = Array.from({ length: 1200 }, (_, i): Status =>
    i + 1 === 3 || (i + 1 >= 600 && i + 1 <= 1150) ? P : M,
  );
  statuses[699] = V;
  statuses[1199] = V;
  const { akash, reads, pages } = ledger(statuses);
  const observer = new FakeObserver();
  for (let id = 1n; id <= 1200n; id++)
    if (statuses[Number(id) - 1] === M)
      observer.solana.set(id, Outcome.OUTCOME_MINTED);
  const { failures, lines } = await scanner(akash, observer, {
    maxTransfers: 10,
    auditPerScan: 5,
  }).run();
  assert.equal(failures, 0);
  assert.equal(akash.transferCalls, 0);
  assert.deepEqual(pages, [0n, 1098n]);
  assert.ok(OPEN_PAGE_LIMIT === 500);
  // Voided transfers come first, then pending, then the audit (1, 2, 4, 5).
  assert.deepEqual(reads.slice(0, 3), [700n, 1200n, 3n]);
  assert.equal(reads.length, 553 + 4);
  assert.deepEqual(reads.slice(-4), [1n, 2n, 4n, 5n]);
  // Consistent terminal audits need no transaction evidence.
  assert.ok(!observer.evidence.some((id) => [1n, 2n, 4n, 5n].includes(id)));
  assert.deepEqual(lines.at(-1), {
    scanned: 553,
    audited: 4,
    index: "open-transfers",
    failures: 0,
    lightClientProgram: observer.program.toBase58(),
  });
});

test("the terminal audit is round-robin and skips open transfers", async () => {
  const { akash } = ledger([M, P, M, M, M, M]);
  const observer = new FakeObserver();
  for (const id of [1n, 3n, 4n, 5n, 6n])
    observer.solana.set(id, Outcome.OUTCOME_MINTED);
  const scan = scanner(akash, observer, { auditPerScan: 4 });
  const audited = async () =>
    (await scan.run()).lines.filter((line) => line.audit).map((l) => l.id);
  assert.deepEqual(await audited(), [1n, 3n, 4n]);
  assert.deepEqual(await audited(), [5n, 6n, 1n]);
  assert.deepEqual(await audited(), [3n, 4n, 5n, 6n]);
  // A budget above the ledger size visits each ID once per pass.
  const all = scanner(akash, observer, { auditPerScan: 50 });
  const lines = (await all.run()).lines.filter((line) => line.audit);
  assert.deepEqual(
    lines.map((line) => line.id),
    [1n, 3n, 4n, 5n, 6n],
  );
});

test("the audit reports a contradiction after a false terminal confirmation", async () => {
  const { akash } = ledger([M, M, M]);
  const observer = new FakeObserver();
  observer.solana.set(1n, Outcome.OUTCOME_MINTED);
  observer.solana.set(2n, Outcome.OUTCOME_VOIDED);
  const { failures, lines } = await scanner(akash, observer).run();
  assert.equal(failures, 0);
  assert.deepEqual(observer.evidence, [2n, 3n]);
  const byId = (id: bigint) => lines.find((line) => line.id === id);
  assert.deepEqual(byId(1n)?.messages, []);
  const contradiction = byId(2n);
  assert.equal(contradiction?.audit, true);
  assert.match(
    String(contradiction?.alerts),
    /contradicts finalized Solana outcome/,
  );
  assert.deepEqual(
    (contradiction?.messages as { typeUrl: string }[]).map((m) => m.typeUrl),
    ["/akash.sink.v1.MsgReportOutcome"],
  );
  assert.match(
    String(byId(3n)?.alerts),
    /outcome without a finalized Solana status/,
  );
  assert.deepEqual(byId(3n)?.messages, []);
});

test("an inconsistent open listing alerts and falls back to the full scan", async (t) => {
  type Page = Awaited<ReturnType<ScanAkash["openTransfers"]>>;
  const tampers: [string, (page: Page, startAfter: bigint) => Page][] = [
    [
      "missing transfer",
      (page) => page && { ...page, transfers: page.transfers.slice(1) },
    ],
    [
      "terminal transfer",
      (page) =>
        page && { ...page, transfers: [...page.transfers, transfer(1n, M)] },
    ],
    [
      "unordered IDs",
      (page) => page && { ...page, transfers: [...page.transfers].reverse() },
    ],
    [
      "ID beyond the ledger",
      (page) =>
        page && { ...page, transfers: [...page.transfers, transfer(9n, P)] },
    ],
    ["looping cursor", (page) => page && { ...page, next: 1n }],
    ["vanishing query", (page, startAfter) => (startAfter ? undefined : page)],
  ];
  for (const [name, tamper] of tampers)
    await t.test(name, async () => {
      const { akash, reads } = ledger([M, P, V, P]);
      const listing = akash.openTransfers.bind(akash);
      akash.openTransfers = async (startAfter, limit, height) => {
        const page = await listing(startAfter, 2, height);
        return tamper(page, startAfter);
      };
      const { failures, lines } = await scanner(akash, new FakeObserver(), {
        maxTransfers: 4,
      }).run();
      assert.equal(lines[0]?.alert, "open-transfers-inconsistent", name);
      assert.equal(failures, 1);
      assert.equal(akash.transferCalls, 4);
      assert.deepEqual(reads, [3n, 1n, 2n, 4n]);
      assert.equal(lines.at(-1)?.index, "full-scan");
      assert.equal(lines.at(-1)?.audited, 0);
    });
});

test("without OpenTransfers the full scan and maxTransfers stop remain", async () => {
  const { akash, reads } = ledger([P, V, M], { open: false });
  const observer = new FakeObserver();
  observer.solana.set(3n, Outcome.OUTCOME_MINTED);
  const ok = await scanner(akash, observer, { maxTransfers: 3 }).run();
  assert.equal(ok.failures, 0);
  assert.deepEqual(reads, [2n, 1n, 3n]);
  assert.equal(ok.lines.at(-1)?.index, "full-scan");
  const capped = await scanner(akash, observer, { maxTransfers: 2 }).run();
  assert.equal(capped.failures, 1);
  assert.match(String(capped.lines.at(-1)?.error), /exceeds scan limit/);
  // An RPC failure is not mistaken for a missing query.
  akash.openTransfers = async () => {
    throw new Error("connection reset");
  };
  const failed = await scanner(akash, observer).run();
  assert.equal(failed.failures, 1);
  assert.deepEqual(failed.lines, [{ error: "connection reset" }]);
});
