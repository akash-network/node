import assert from "node:assert/strict";
import test from "node:test";
import { web3 } from "@coral-xyz/anchor";
import { toBech32 } from "@cosmjs/encoding";
import {
  Hold,
  Outcome,
  Sink_Status as Status,
  TransferState,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { Scanner, type ScanAkash, type ScanObserver } from "./scan";
import { LightClientMismatchError } from "./solana";
import {
  collectPages,
  discoverTransfers,
  openTransfers,
} from "../local/open-transfers";
import { parseSinkId, type SinkId } from "../shared/id";
import { testId, testSnapshot, testTransfer } from "../tests/sink-fixtures";
test("chronological discovery reuses the server's last-page cursor and finds later earlier-sorting owners", async () => {
  const first = testTransfer(1n, 10n),
    second = testTransfer(2n, 10n),
    third = testTransfer(3n, 10n),
    later = testTransfer(
      1n,
      11n,
      toBech32("akash", new Uint8Array(20).fill(1)),
    ),
    token = new Uint8Array([255, 3, 19]);
  let tail = [third];
  const akash = {
    async transfers(height: number, options: { key?: Uint8Array } = {}) {
      assert.equal(height, 20);
      if (!options.key?.length)
        return { transfers: [first, second], nextKey: token };
      assert.deepEqual(options.key, token);
      return { transfers: tail, nextKey: new Uint8Array() };
    },
  };
  const start = await discoverTransfers(akash, 20);
  assert.deepEqual(start.cursor, {
    kind: "next",
    key: token,
    lastId: parseSinkId(second.id),
  });
  assert.equal(start.hasMore, true);
  const end = await discoverTransfers(akash, 20, start.cursor);
  assert.deepEqual(end.cursor, {
    kind: "tail",
    key: token,
    lastId: parseSinkId(third.id),
  });
  assert.equal(end.hasMore, false);
  assert.equal(
    (await discoverTransfers(akash, 20, end.cursor)).entries.length,
    0,
  );
  tail = [third, later];
  const next = await discoverTransfers(akash, 20, end.cursor);
  assert.deepEqual(
    next.entries.map((entry) => entry.id),
    [parseSinkId(later.id)],
  );
  assert.equal(next.cursor?.lastId, parseSinkId(later.id));
  tail = [later];
  await assert.rejects(
    () => discoverTransfers(akash, 20, end.cursor),
    /checkpoint disappeared/,
  );
});
test("native pages preserve fixed height, reject duplicates, stalled cursors and pruned state", async () => {
  const a = testTransfer(1),
    b = testTransfer(2);
  const keys: Uint8Array[] = [];
  const result = await collectPages(async (key) => {
    keys.push(key);
    return key.length
      ? { transfers: [b], nextKey: new Uint8Array() }
      : { transfers: [a], nextKey: new Uint8Array([2]) };
  });
  assert.equal(result.length, 2);
  assert.deepEqual(keys, [new Uint8Array(), new Uint8Array([2])]);
  const tokens = [new Uint8Array([255]), new Uint8Array([1]), new Uint8Array()];
  let calls = 0;
  const unordered = await collectPages(async () => ({
    transfers: [testTransfer(++calls)],
    nextKey: tokens[calls - 1],
  }));
  assert.equal(unordered.length, 3, "opaque cursor bytes need not increase");
  calls = 0;
  await assert.rejects(
    () =>
      collectPages(async () => ({
        transfers: [testTransfer(++calls)],
        nextKey: tokens[(calls - 1) % 2],
      })),
    /cursor did not advance/,
  );
  await assert.rejects(
    () =>
      collectPages(async () => ({
        transfers: [a],
        nextKey: new Uint8Array([2]),
      })),
    /duplicate/,
  );
  await assert.rejects(
    () =>
      collectPages(async () => ({
        transfers: [],
        nextKey: new Uint8Array([2]),
      })),
    /advance/,
  );
  await assert.rejects(
    () =>
      collectPages(async () => {
        throw new Error("height pruned");
      }),
    /pruned/,
  );
});
function fixture() {
  const base = testSnapshot(),
    held = testTransfer(2);
  assert.ok(held.settlement);
  held.status = Status.STATUS_REFUNDED;
  held.settlement.hold = Hold.HOLD_CONFLICT;
  const rows = [base.transfer, held],
    reads: SinkId[] = [];
  const state = {
    ...base,
    height: 20,
    epoch: 1n,
    sendUsage: base.refundUsage,
    refundHead: base.transfer.id,
  };
  const akash: ScanAkash = {
    async state() {
      return state;
    },
    async transfer(id) {
      const value = rows.find((t) => parseSinkId(t.id) === id);
      assert.ok(value);
      return value;
    },
    async snapshot(id) {
      reads.push(id);
      return { ...base, transfer: await this.transfer(id, 20) };
    },
    async transfers(_height, options = {}) {
      return {
        transfers:
          options.status === undefined
            ? rows
            : rows.filter((t) => t.status === options.status),
        nextKey: new Uint8Array(),
      };
    },
    async disputes() {
      return { transfers: [held], nextKey: new Uint8Array() };
    },
  };
  const observer: ScanObserver = {
    async lightClient() {
      return web3.PublicKey.default;
    },
    takeLightClientChanges() {
      return [];
    },
    async observe() {
      return { kind: "absent", contextSlot: 100n };
    },
    async observeFinalized() {
      return { kind: "absent", contextSlot: 100n };
    },
  };
  const output: unknown[] = [];
  const scanner = new Scanner(akash, observer, {
    reporter: "reporter",
    auditPerScan: 10,
    output: (v) => output.push(v),
    signal: new AbortController().signal,
  });
  return { base, akash, observer, reads, output, scanner };
}
test("all held terminal disputes are reviewed first and reread before decisions", async () => {
  const f = fixture();
  assert.equal((await openTransfers(f.akash, 20)).length, 2);
  assert.equal(await f.scanner.scan(), 0);
  assert.deepEqual(f.reads, [testId(2), testId()]);
});
test("pruned queries and light-client pin mismatches stop the scan without fallback", async () => {
  const f = fixture();
  f.akash.transfers = async () => {
    throw new Error("height pruned");
  };
  assert.equal(await f.scanner.scan(), 1);
  assert.equal(f.reads.length, 0);
  const g = fixture();
  g.observer.lightClient = async () => {
    throw new LightClientMismatchError(
      web3.PublicKey.default,
      web3.Keypair.generate().publicKey,
    );
  };
  assert.equal(await g.scanner.scan(), 1);
  assert.equal(g.reads.length, 0);
});
function pages(rows: TransferState[]) {
  return (key: Uint8Array | undefined, limit: number) => {
    const start = key?.length ? Buffer.from(key).readUInt32BE(0) : 0,
      next = start + limit,
      nextKey = Buffer.alloc(next < rows.length ? 4 : 0);
    if (nextKey.length) nextKey.writeUInt32BE(next);
    return { transfers: rows.slice(start, next), nextKey };
  };
}
function backlog(counts: {
  held: number;
  refunds: number;
  ordered: number;
  pending: number;
}) {
  const owner = toBech32("akash", new Uint8Array(20).fill(9)),
    row = (
      sequence: number,
      status: Status,
      hold = Hold.HOLD_NONE,
    ): TransferState => {
      const transfer = testTransfer(sequence, 5n, owner);
      assert.ok(transfer.settlement);
      transfer.status = status;
      transfer.settlement.hold = hold;
      return transfer;
    };
  let sequence = 0;
  const disputes = Array.from({ length: counts.held }, (_, i) =>
    row(
      ++sequence,
      [
        Status.STATUS_REFUND_PENDING,
        Status.STATUS_TRANSFER_PENDING,
        Status.STATUS_MINTED,
      ][i % 3],
      i % 3 === 2 ? Hold.HOLD_CONFLICT : Hold.HOLD_VETO,
    ),
  );
  const refunds = Array.from({ length: counts.refunds }, () =>
    row(++sequence, Status.STATUS_REFUND_PENDING),
  );
  const ordered = Array.from({ length: counts.ordered }, () =>
    row(++sequence, Status.STATUS_VOIDED),
  );
  const pending = Array.from({ length: counts.pending }, () =>
    row(++sequence, Status.STATUS_TRANSFER_PENDING),
  );
  const all = [...disputes, ...refunds, ...ordered, ...pending];
  const byStatus = (status: Status) =>
    all.filter((transfer) => transfer.status === status);
  const index = {
    disputes: pages(disputes),
    [Status.STATUS_REFUND_PENDING]: pages(
      byStatus(Status.STATUS_REFUND_PENDING),
    ),
    [Status.STATUS_VOIDED]: pages(byStatus(Status.STATUS_VOIDED)),
    [Status.STATUS_TRANSFER_PENDING]: pages(
      byStatus(Status.STATUS_TRANSFER_PENDING),
    ),
    history: pages(all),
  };
  const requests: string[] = [];
  const akash = {
    async transfers(
      _height: number,
      options: { key?: Uint8Array; limit?: number; status?: Status } = {},
    ) {
      requests.push(String(options.status ?? "history"));
      const read =
        options.status === Status.STATUS_REFUND_PENDING ||
        options.status === Status.STATUS_VOIDED ||
        options.status === Status.STATUS_TRANSFER_PENDING
          ? index[options.status]
          : index.history;
      return read(options.key, options.limit ?? 100);
    },
    async disputes(_height: number, key?: Uint8Array, limit = 100) {
      requests.push("disputes");
      return index.disputes(key, limit);
    },
  };
  return { disputes, refunds, ordered, pending, all, akash, requests };
}
test("every open transfer past the old bounds is reviewed, disputes and refunds first", async () => {
  const b = backlog({ held: 501, refunds: 501, ordered: 501, pending: 8_498 });
  assert.equal(b.all.length, 10_001, "one more than the old maximum bound");
  const f = fixture(),
    byId = new Map(
      b.all.map((transfer) => [parseSinkId(transfer.id), transfer]),
    );
  Object.assign(f.akash, b.akash);
  f.akash.transfer = async (id) => {
    const found = byId.get(id);
    assert.ok(found);
    return found;
  };
  assert.equal(await f.scanner.scan(), 0);
  const order = [...b.disputes, ...b.refunds, ...b.ordered, ...b.pending].map(
    (transfer) => parseSinkId(transfer.id),
  );
  assert.deepEqual(f.reads, order, "each open transfer once, urgent first");
  for (const kind of ["disputes", "4", "3", "1"])
    assert.ok(
      b.requests.filter((request) => request === kind).length > 1,
      `${kind} index read across pages`,
    );
  assert.deepEqual(f.output.at(-1), {
    scanned: 10_001,
    audited: 10,
    index: "canonical",
    failures: 0,
    lightClientProgram: web3.PublicKey.default.toBase58(),
  });
});
test("fixed-height listings return every record past the old 100,000 bound", async () => {
  const owner = toBech32("akash", new Uint8Array(20).fill(5)),
    rows = Array.from({ length: 100_001 }, (_, i) =>
      TransferState.fromPartial({
        id: { owner, height: 7n, sequence: BigInt(i + 1) },
        status: Status.STATUS_TRANSFER_PENDING,
      }),
    ),
    read = pages(rows);
  const history = await collectPages((key) => Promise.resolve(read(key, 500)));
  assert.equal(history.length, 100_001);
  assert.equal(history.at(-1)?.id, parseSinkId(rows[100_000].id));
  const open = await openTransfers(
    {
      async transfers(_height, options = {}) {
        return options.status === Status.STATUS_TRANSFER_PENDING
          ? read(options.key, options.limit ?? 100)
          : { transfers: [], nextKey: new Uint8Array() };
      },
      async disputes() {
        return { transfers: [], nextKey: new Uint8Array() };
      },
    },
    20,
  );
  assert.equal(open.length, 100_001);
});
test("an audit query failure still reviews open transfers and retries its window", async () => {
  const f = fixture(),
    terminal = testTransfer(3);
  assert.ok(terminal.settlement);
  terminal.status = Status.STATUS_MINTED;
  const pending = await f.akash.transfer(testId(), 20),
    held = await f.akash.transfer(testId(2), 20),
    history = [pending, held, terminal],
    listed = f.akash.transfers.bind(f.akash),
    found = f.akash.transfer.bind(f.akash),
    auditKeys: (Uint8Array | undefined)[] = [];
  let auditFails = false;
  f.akash.transfer = async (id, height) =>
    id === testId(3) ? terminal : found(id, height);
  f.akash.transfers = async (height, options = {}) => {
    if (options.status !== undefined) return listed(height, options);
    auditKeys.push(options.key);
    if (auditFails) throw new Error("history page unavailable");
    const start = options.key ? 2 : 0;
    if (options.key) assert.deepEqual(options.key, new Uint8Array([255, 7]));
    assert.equal(options.limit, 2);
    return {
      transfers: history.slice(start, start + 2),
      nextKey:
        start + 2 < history.length
          ? new Uint8Array([255, 7])
          : new Uint8Array(),
    };
  };
  const scanner = new Scanner(f.akash, f.observer, {
    reporter: "reporter",
    auditPerScan: 2,
    output: (value) => f.output.push(value),
    signal: new AbortController().signal,
  });
  assert.equal(await scanner.scan(), 0);
  auditFails = true;
  f.reads.length = 0;
  assert.equal(await scanner.scan(), 1);
  assert.deepEqual(f.reads, [testId(2), testId()], "open work still reviewed");
  assert.ok(
    f.output.some(
      (value) =>
        typeof value === "object" &&
        value !== null &&
        "alert" in value &&
        value.alert === "audit-unavailable",
    ),
  );
  auditFails = false;
  f.reads.length = 0;
  assert.equal(await scanner.scan(), 0);
  assert.deepEqual(auditKeys.slice(1), [
    new Uint8Array([255, 7]),
    new Uint8Array([255, 7]),
  ]);
  assert.deepEqual(f.reads, [testId(2), testId(), testId(3)]);
});

test("broadcast scans do not pay for bare void orders, frozen same-outcome votes or existing conflicts", async () => {
  const f = fixture(),
    settlement = f.base.transfer.settlement;
  assert.ok(settlement);
  f.base.transfer.status = Status.STATUS_VOIDED;
  settlement.voidAuthorization = { requestedHeight: 1n, requestedRound: 1n };
  const submitted: unknown[] = [];
  const scanner = new Scanner(f.akash, f.observer, {
    reporter: "d",
    auditPerScan: 10,
    signal: new AbortController().signal,
    output: (value) => f.output.push(value),
    submit: async (message) => {
      submitted.push(message);
      return "signature";
    },
  });
  assert.equal(await scanner.scan(), 0);
  assert.equal(submitted.length, 0);
  const order = f.output.find(
    (value) =>
      typeof value === "object" &&
      value !== null &&
      "id" in value &&
      value.id === testId(),
  );
  assert.ok(order && typeof order === "object" && "alerts" in order);
  assert.deepEqual(order.alerts, []);
  f.base.transfer.status = Status.STATUS_REFUND_PENDING;
  settlement.challengeEnd = f.base.time + 100n;
  const observation = {
    outcome: Outcome.OUTCOME_VOIDED,
    slot: 10n,
    transactionSignature: new Uint8Array(64).fill(1),
  };
  settlement.votes = ["reporter", "b", "c"].map((reporter) => ({
    reporter,
    policyVersion: 1n,
    observation,
  }));
  f.observer.observe = async () => ({
    kind: "resolved",
    contextSlot: 100n,
    observation,
  });
  assert.equal(await scanner.scan(), 0);
  assert.equal(await scanner.scan(), 0);
  assert.equal(submitted.length, 0);
  f.observer.observe = async () => ({
    kind: "resolved",
    contextSlot: 100n,
    observation: { ...observation, outcome: Outcome.OUTCOME_MINTED },
  });
  assert.equal(await scanner.scan(), 0);
  assert.equal(submitted.length, 1, "first opposite witness is actionable");
  settlement.hold = Hold.HOLD_CONFLICT;
  assert.equal(await scanner.scan(), 0);
  assert.equal(await scanner.scan(), 0);
  assert.equal(
    submitted.length,
    1,
    "conflict never causes endless paid reports",
  );
});
