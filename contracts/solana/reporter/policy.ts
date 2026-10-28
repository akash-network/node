import assert from "node:assert/strict";
import {
  Hold,
  Outcome,
  type Observation,
  Sink_Status as Status,
  type Settlement,
  type Sink_Params as Params,
  type Totals,
  type TransferState,
  MsgReportOutcome,
  MsgVetoVoid,
  MsgFinalizeRefund,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { decodeRecord, type Observed } from "./solana";

export type Message =
  | { typeUrl: "/akash.sink.v1.MsgReportOutcome"; value: MsgReportOutcome }
  | { typeUrl: "/akash.sink.v1.MsgVetoVoid"; value: MsgVetoVoid }
  | { typeUrl: "/akash.sink.v1.MsgFinalizeRefund"; value: MsgFinalizeRefund };
export interface Snapshot {
  params: Params;
  totals: Totals;
  transfer: TransferState;
  time: bigint;
  // x/sink pays refunds oldest first: only the head of its refund queue can be
  // finalized. Undefined when the caller did not list open transfers.
  refundHead?: bigint;
}
export type Plan = { messages: Message[]; alerts: string[] };

// refundQueueHead mirrors x/sink's refund queue: voided transfers without a
// hold, ordered by challenge end, then ID.
export function refundQueueHead(
  entries: { id: bigint; transfer: TransferState }[],
): bigint | undefined {
  let head: { id: bigint; end: bigint } | undefined;
  for (const { id, transfer } of entries) {
    const s = transfer.settlement;
    if (s?.status !== Status.STATUS_VOIDED || s.hold !== Hold.HOLD_NONE)
      continue;
    const end = s.challengeEnd.toBigInt();
    if (!head || end < head.end || (end === head.end && id < head.id))
      head = { id, end };
  }
  return head?.id;
}

export function reportedObservations(transfer: TransferState): Observation[] {
  const settlement = transfer.settlement;
  assert.ok(settlement, "missing settlement");
  return [
    settlement.governanceObservation,
    ...settlement.votes.map((vote) => vote.observation),
  ].filter((observation) => observation !== undefined);
}

// Highest reported slot, ignoring slots above slotLimit when one is given.
export function minimumSlot(transfer: TransferState, slotLimit?: bigint) {
  return reportedObservations(transfer).reduce((slot, observation) => {
    const observedSlot = observation.slot.toBigInt();
    return observedSlot > slot &&
      (slotLimit === undefined || observedSlot <= slotLimit)
      ? observedSlot
      : slot;
  }, 0n);
}

// How a finalized Solana outcome relates to the Akash settlement and its
// reported observations.
export function compareOutcome(settlement: Settlement, outcome: Outcome) {
  const mint = outcome === Outcome.OUTCOME_MINTED;
  return {
    contradicts:
      (settlement.status === Status.STATUS_MINTED && !mint) ||
      ((settlement.status === Status.STATUS_VOIDED ||
        settlement.status === Status.STATUS_REFUNDED) &&
        mint) ||
      settlement.votes.some(
        (vote) => vote.observation && vote.observation.outcome !== outcome,
      ) ||
      (settlement.governanceObservation !== undefined &&
        settlement.governanceObservation.outcome !== outcome),
    terminalMatches:
      (mint && settlement.status === Status.STATUS_MINTED) ||
      (!mint && settlement.status === Status.STATUS_REFUNDED),
  };
}

// A terminal, unheld settlement that agrees with the finalized outcome and
// every report. planOutcome would return no message and no alert, so an audit
// can skip transaction discovery and its block history.
export function settledConsistently(settlement: Settlement, outcome: Outcome) {
  const { contradicts, terminalMatches } = compareOutcome(settlement, outcome);
  return terminalMatches && !contradicts && settlement.hold === Hold.HOLD_NONE;
}

// Pure policy; the chain remains authoritative for membership, rounds, caps,
// challenge deadlines and exact-once payment when these messages execute.
export function planOutcome(
  reporter: string,
  snapshot: Snapshot,
  observed: Observed,
): Plan {
  const { params, totals, transfer, time } = snapshot;
  const settlement = transfer.settlement;
  assert.ok(
    settlement?.policy && settlement.round.greaterThan(0),
    "invalid sink settlement",
  );
  assert.ok(
    params.reporters.includes(reporter),
    "account is not a current sink reporter",
  );
  const record = decodeRecord(transfer.record);
  const challengeEnd = settlement.challengeEnd.toBigInt();
  const result: Plan = { messages: [], alerts: [] };
  if (observed.kind === "absent") {
    // A slot beyond what any node could have finalized is false evidence. It
    // must not stop a fresh node from proving absence and vetoing.
    const { contextSlot, slotLimit } = observed;
    assert.ok(
      slotLimit === undefined || slotLimit >= contextSlot,
      "invalid plausible slot limit",
    );
    assert.ok(
      contextSlot >= minimumSlot(transfer, slotLimit),
      "observation is behind reported evidence",
    );
    if (slotLimit !== undefined && minimumSlot(transfer) > slotLimit)
      result.alerts.push(
        "Akash reports a Solana slot beyond any plausible finalized slot",
      );
    if (
      settlement.status !== Status.STATUS_PENDING ||
      settlement.votes.length > 0
    ) {
      result.alerts.push(
        "Akash reports an outcome without a finalized Solana status",
      );
      if (
        settlement.status === Status.STATUS_VOIDED &&
        settlement.hold === Hold.HOLD_NONE &&
        time < challengeEnd
      ) {
        result.messages.push({
          typeUrl: "/akash.sink.v1.MsgVetoVoid",
          value: MsgVetoVoid.fromPartial({
            signer: reporter,
            id: record.id,
            reason: `No Solana status at finalized slot ${observed.contextSlot}`,
          }),
        });
      }
    }
    return result;
  }
  const { observation } = observed;
  // A permanent finalized outcome remains evidence even if another reporter
  // invents a future slot. Only an absence check must wait for reported slots.
  assert.ok(
    observation.outcome === Outcome.OUTCOME_MINTED ||
      observation.outcome === Outcome.OUTCOME_VOIDED,
  );
  const mint = observation.outcome === Outcome.OUTCOME_MINTED;
  const { contradicts, terminalMatches } = compareOutcome(
    settlement,
    observation.outcome,
  );
  if (contradicts)
    result.alerts.push("Akash settlement contradicts finalized Solana outcome");
  if (settlement.hold !== Hold.HOLD_NONE)
    result.alerts.push("Transfer requires governance review");
  const currentEpoch = settlement.policy.epoch.equals(totals.reporterEpoch);
  const alreadyReported =
    currentEpoch &&
    settlement.votes.some(
      (vote) =>
        vote.reporter === reporter &&
        vote.observation?.outcome === observation.outcome,
    );
  if (!alreadyReported && (!terminalMatches || contradicts)) {
    result.messages.push({
      typeUrl: "/akash.sink.v1.MsgReportOutcome",
      value: MsgReportOutcome.fromPartial({
        reporter,
        id: record.id,
        reporterEpoch: totals.reporterEpoch,
        round: settlement.round,
        observation,
      }),
    });
    return result; // Re-read after voting before considering any refund.
  }
  const spent =
    totals.refundDay.toBigInt() === time / 86400n
      ? totals.refundedToday.toBigInt()
      : 0n;
  if (
    !mint &&
    settlement.status === Status.STATUS_VOIDED &&
    settlement.hold === Hold.HOLD_NONE &&
    !contradicts &&
    currentEpoch &&
    params.refundsEnabled &&
    time >= challengeEnd &&
    challengeEnd > 0n &&
    spent + record.amount <= params.dailyRefundCap.toBigInt() &&
    (snapshot.refundHead === undefined || snapshot.refundHead === record.id)
  ) {
    result.messages.push({
      typeUrl: "/akash.sink.v1.MsgFinalizeRefund",
      value: MsgFinalizeRefund.fromPartial({
        executor: reporter,
        id: record.id,
      }),
    });
  }
  return result;
}
