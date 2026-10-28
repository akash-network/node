import assert from "node:assert/strict";
import {
  Sink_Status as Status,
  type TransferState,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import type { AkashClient } from "./akash";
import {
  planOutcome,
  refundQueueHead,
  reportedObservations,
  settledConsistently,
  type Message,
} from "./policy";
import {
  LightClientMismatchError,
  decodeRecord,
  type SolanaObserver,
} from "./solana";

export type ScanAkash = Pick<
  AkashClient,
  "state" | "transfer" | "snapshot" | "openTransfers"
>;
export type ScanObserver = Pick<
  SolanaObserver,
  "lightClient" | "takeLightClientChanges" | "observe" | "observeFinalized"
>;
export interface ScanOptions {
  reporter: string;
  // Hard stop for the full scan used by nodes without OpenTransfers.
  maxTransfers: number;
  // Transfer IDs the rolling terminal audit visits per pass.
  auditPerScan: number;
  output: (value: unknown) => void;
  signal: AbortSignal;
  submit?: (message: Message) => Promise<string>;
}

// The node's open-transfer listing contradicts its own committed state.
export class OpenIndexError extends Error {}

export const OPEN_PAGE_LIMIT = 500;
const PENDING = [Status.STATUS_PENDING, Status.STATUS_VOIDED];

function message(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

function check(condition: boolean, reason: string): asserts condition {
  if (!condition) throw new OpenIndexError(reason);
}

type State = Awaited<ReturnType<ScanAkash["state"]>>;
type Entry = { id: bigint; transfer: TransferState };

export class Scanner {
  // Last ID the terminal audit visited. Memory only; a restart starts over.
  private auditCursor = 0n;
  constructor(
    private readonly akash: ScanAkash,
    private readonly observer: ScanObserver,
    private readonly options: ScanOptions,
  ) {}

  // Every non-terminal transfer at the state height, checked against the
  // pending count. Undefined when the node lacks the query.
  async openTransfers(state: State): Promise<Entry[] | undefined> {
    const last = state.totals.lastTransferId.toBigInt();
    const pending = state.totals.pendingCount.toBigInt();
    const pages = pending / BigInt(OPEN_PAGE_LIMIT) + 1n;
    const result: Entry[] = [];
    let startAfter = 0n;
    for (let page = 0n; ; page++) {
      const reply = await this.akash.openTransfers(
        startAfter,
        OPEN_PAGE_LIMIT,
        state.height,
      );
      if (!reply && page === 0n) return undefined;
      check(reply !== undefined, "open transfer query disappeared");
      check(
        reply.transfers.length <= OPEN_PAGE_LIMIT,
        "open transfer page exceeds its limit",
      );
      for (const transfer of reply.transfers) {
        let id: bigint;
        try {
          id = decodeRecord(transfer.record).id;
        } catch {
          throw new OpenIndexError("malformed open transfer record");
        }
        check(
          id > (result.at(-1)?.id ?? startAfter) && id <= last,
          "open transfer IDs out of order or range",
        );
        check(
          transfer.settlement !== undefined &&
            PENDING.includes(transfer.settlement.status),
          `open transfer ${id} is terminal`,
        );
        result.push({ id, transfer });
      }
      if (reply.next === 0n) break;
      check(
        reply.transfers.length > 0 && reply.next === result.at(-1)?.id,
        "open transfer cursor does not follow the page",
      );
      check(page + 1n < pages, "open transfer pages exceed pending count");
      startAfter = reply.next;
    }
    check(
      BigInt(result.length) === pending,
      `open transfers (${result.length}) differ from pending count (${pending})`,
    );
    return result;
  }

  // The next auditPerScan IDs in round-robin order, skipping open transfers
  // that this pass already reviews.
  auditIds(last: bigint, open: ReadonlySet<bigint>) {
    const ids: bigint[] = [];
    const count = BigInt(this.options.auditPerScan);
    for (let i = 0n; i < count && i < last; i++) {
      this.auditCursor = this.auditCursor >= last ? 1n : this.auditCursor + 1n;
      if (!open.has(this.auditCursor)) ids.push(this.auditCursor);
    }
    return ids;
  }

  private async fullScan(state: State) {
    const last = state.totals.lastTransferId.toBigInt();
    assert.ok(
      last <= BigInt(this.options.maxTransfers),
      "ledger exceeds scan limit; increase maxTransfers or use a node with OpenTransfers",
    );
    const transfers: Entry[] = [];
    for (let id = 1n; id <= last && !this.options.signal.aborted; id++) {
      transfers.push({
        id,
        transfer: await this.akash.transfer(id, state.height),
      });
    }
    return transfers;
  }

  private changes() {
    for (const change of this.observer.takeLightClientChanges())
      this.options.output({
        alert: "light-client-changed",
        previous: change.previous.toBase58(),
        current: change.current.toBase58(),
        slot: change.contextSlot,
      });
  }

  // Rereads one transfer and acts on it. An audited terminal transfer that
  // agrees with its finalized Solana status needs no transaction evidence.
  private async review(id: bigint, audit: boolean, refundHead?: bigint) {
    const { reporter, output, signal, submit } = this.options;
    const mode = submit ? "broadcast" : "preview";
    const snapshot = { ...(await this.akash.snapshot(id)), refundHead };
    const record = decodeRecord(snapshot.transfer.record);
    const settlement = snapshot.transfer.settlement;
    assert.ok(settlement, "missing sink settlement");
    if (audit && !PENDING.includes(settlement.status)) {
      const finalized = await this.observer.observeFinalized(record);
      this.changes();
      if (
        finalized.kind === "resolved" &&
        settledConsistently(settlement, finalized.outcome)
      ) {
        output({
          id,
          mode,
          audit,
          observed: {
            kind: finalized.kind,
            contextSlot: finalized.contextSlot,
            outcome: finalized.outcome,
            slot: finalized.slot,
          },
          messages: [],
          alerts: [],
        });
        return;
      }
    }
    const observed = await this.observer.observe(
      record,
      reportedObservations(snapshot.transfer),
      settlement.votes.find((vote) => vote.reporter === reporter)?.observation,
    );
    this.changes();
    const plan = planOutcome(reporter, snapshot, observed);
    output({ id, mode, ...(audit && { audit }), observed, ...plan });
    for (const message of plan.messages) {
      if (submit && !signal.aborted)
        output({ id, transactionHash: await submit(message) });
    }
  }

  // One pass. Returns the number of failures. A light client other than the
  // configured pin stops the pass before any further decision.
  async scan() {
    const { reporter, output, signal } = this.options;
    let failures = 0;
    try {
      const lightClientProgram = await this.observer.lightClient();
      this.changes();
      const state = await this.akash.state();
      assert.ok(
        state.params.reporters.includes(reporter),
        "not a current reporter",
      );
      let open: Entry[] | undefined;
      try {
        open = await this.openTransfers(state);
      } catch (error) {
        if (!(error instanceof OpenIndexError)) throw error;
        // Every decision still rereads its transfer; only the listing is
        // doubted, so fall back to reading every ID.
        failures++;
        output({ alert: "open-transfers-inconsistent", error: error.message });
      }
      const transfers = open ?? (await this.fullScan(state));
      const audit = open
        ? this.auditIds(
            state.totals.lastTransferId.toBigInt(),
            new Set(open.map(({ id }) => id)),
          )
        : [];
      // Review refund windows before normal pending/terminal bookkeeping.
      transfers.sort(
        (a, b) =>
          Number(b.transfer.settlement?.status === Status.STATUS_VOIDED) -
          Number(a.transfer.settlement?.status === Status.STATUS_VOIDED),
      );
      // Only the oldest matured refund can be paid; the next one waits for
      // the following pass.
      const refundHead = refundQueueHead(transfers);
      const queue = [
        ...transfers.map(({ id }) => ({ id, audit: false })),
        ...audit.map((id) => ({ id, audit: true })),
      ];
      for (const { id, audit } of queue) {
        if (signal.aborted) break;
        try {
          await this.review(id, audit, refundHead);
        } catch (error) {
          this.changes();
          if (error instanceof LightClientMismatchError) throw error;
          failures++;
          output({ id, error: message(error, "observation failed") });
        }
      }
      output({
        scanned: transfers.length,
        audited: audit.length,
        index: open ? "open-transfers" : "full-scan",
        failures,
        lightClientProgram: lightClientProgram.toBase58(),
      });
    } catch (error) {
      failures++;
      if (error instanceof LightClientMismatchError)
        output({
          alert: "light-client-mismatch",
          pinned: error.pinned.toBase58(),
          current: error.current.toBase58(),
        });
      output({ error: message(error, "scan failed") });
    }
    return failures;
  }
}
