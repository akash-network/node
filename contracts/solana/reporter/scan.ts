import assert from "node:assert/strict";
import { Sink_Status as Status } from "@akashnetwork/chain-sdk/private-types/akash.v1";
import type { AkashClient } from "./akash";
import {
  planOutcome,
  reportedObservations,
  settledConsistently,
  type Message,
} from "./policy";
import {
  LightClientMismatchError,
  decodeRecord,
  type SolanaObserver,
} from "./solana";
import {
  openTransfers,
  discoverTransfers,
  type DiscoveryCursor,
} from "../local/open-transfers";
import type { SinkId } from "../shared/id";
export type ScanAkash = Pick<
  AkashClient,
  "state" | "transfer" | "snapshot" | "transfers" | "disputes"
>;
export type ScanObserver = Pick<
  SolanaObserver,
  "lightClient" | "takeLightClientChanges" | "observe" | "observeFinalized"
>;
export interface ScanOptions {
  reporter: string;
  auditPerScan: number;
  output: (value: unknown) => void;
  signal: AbortSignal;
  submit?: (message: Message) => Promise<string>;
}
const PENDING = [
  Status.STATUS_TRANSFER_PENDING,
  Status.STATUS_VOIDED,
  Status.STATUS_REFUND_PENDING,
];
export class Scanner {
  private auditCursor?: DiscoveryCursor;
  constructor(
    private readonly akash: ScanAkash,
    private readonly observer: ScanObserver,
    private readonly options: ScanOptions,
  ) {}
  private changes() {
    for (const change of this.observer.takeLightClientChanges())
      this.options.output({
        alert: "light-client-changed",
        previous: change.previous.toBase58(),
        current: change.current.toBase58(),
        slot: change.contextSlot,
      });
  }
  private async review(id: SinkId, audit: boolean) {
    const { reporter, output, signal, submit } = this.options;
    const snapshot = await this.akash.snapshot(id),
      record = decodeRecord(snapshot.transfer),
      settlement = snapshot.transfer.settlement;
    assert.ok(settlement, "missing sink settlement");
    if (audit && !PENDING.includes(snapshot.transfer.status)) {
      const finalized = await this.observer.observeFinalized(record);
      this.changes();
      if (
        finalized.kind === "resolved" &&
        settledConsistently(snapshot.transfer, finalized.outcome)
      ) {
        output({
          id,
          audit,
          observed: finalized.kind,
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
    output({
      id,
      mode: submit ? "broadcast" : "preview",
      audit,
      observed,
      ...plan,
    });
    for (const message of plan.messages)
      if (submit && !signal.aborted)
        output({ id, transactionHash: await submit(message) });
  }
  async scan() {
    const { reporter, output, signal } = this.options;
    let failures = 0;
    try {
      const lightClientProgram = await this.observer.lightClient();
      this.changes();
      const state = await this.akash.state();
      assert.ok(
        state.policy.members.includes(reporter),
        "not a current reporter",
      );
      const open = await openTransfers(this.akash, state.height);
      const queue = new Map(open.map((entry) => [entry.id, false]));
      let audited = 0;
      try {
        const audit = await discoverTransfers(
          this.akash,
          state.height,
          this.auditCursor,
          Math.min(500, this.options.auditPerScan),
        );
        this.auditCursor = audit.hasMore ? audit.cursor : undefined;
        audited = audit.entries.length;
        for (const entry of audit.entries)
          if (!queue.has(entry.id)) queue.set(entry.id, true);
      } catch (error) {
        failures++;
        output({
          alert: "audit-unavailable",
          error: error instanceof Error ? error.message : "audit query failed",
        });
      }
      for (const [id, isAudit] of queue) {
        if (signal.aborted) break;
        try {
          await this.review(id, isAudit);
        } catch (error) {
          this.changes();
          if (error instanceof LightClientMismatchError) throw error;
          failures++;
          output({
            id,
            error:
              error instanceof Error ? error.message : "observation failed",
          });
        }
      }
      output({
        scanned: open.length,
        audited,
        index: "canonical",
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
      output({ error: error instanceof Error ? error.message : "scan failed" });
    }
    return failures;
  }
}
