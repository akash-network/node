import assert from "node:assert/strict";
import {
  Hold,
  Outcome,
  type Observation,
  Sink_Status as Status,
  type SettlementData,
  type Sink_Params as Params,
  type Totals,
  type ReporterPolicy,
  type EpochUsage,
  type TransferState,
  MsgReportOutcome,
  MsgVetoVoid,
  MsgFinalizeRefund,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { decodeRecord, type Observed } from "./solana";
import { coinAmount } from "../shared/proof";
import { sinkIdParts, type SinkId } from "../shared/id";

export type Message =
  | { typeUrl: "/akash.sink.v1.MsgReportOutcome"; value: MsgReportOutcome }
  | { typeUrl: "/akash.sink.v1.MsgVetoVoid"; value: MsgVetoVoid }
  | { typeUrl: "/akash.sink.v1.MsgFinalizeRefund"; value: MsgFinalizeRefund };
export interface Snapshot {
  params: Params;
  totals: Totals;
  policy: ReporterPolicy;
  refundUsage: EpochUsage;
  epoch: bigint;
  transfer: TransferState;
  time: bigint;
  // x/sink pays refunds oldest first: only the head of its refund queue can be
  // finalized.
  refundHead?: SinkId;
}
export type Plan = { messages: Message[]; alerts: string[] };

export function challengeSeconds(settlement: SettlementData): bigint {
  assert.ok(settlement.challengeEnd >= 0n, "invalid challenge deadline");
  return settlement.challengeEnd;
}

function reporterRatio(ratio: string): bigint {
  assert.match(ratio, /^(0|1)(\.[0-9]{1,18})?$/, "invalid reporter ratio");
  const [whole, fraction = ""] = ratio.split(".");
  const value = BigInt(whole) * 10n ** 18n + BigInt(fraction.padEnd(18, "0"));
  assert.ok(value <= 10n ** 18n, "reporter ratio must be at most one");
  return value;
}

export function reporterThreshold(ratio: string, count: number): number {
  const scale = 10n ** 18n,
    value = reporterRatio(ratio);
  const members = BigInt(count);
  const threshold = (value * members + scale - 1n) / scale;
  assert.ok(
    count === 0 || 3n * threshold > 2n * members,
    "reporter quorum must exceed two thirds of the members",
  );
  return Number(threshold);
}

export function reportedObservations(transfer: TransferState): Observation[] {
  const settlement = transfer.settlement;
  assert.ok(settlement, "missing settlement");
  return [
    ...(settlement.governanceObservation
      ? [settlement.governanceObservation]
      : []),
    ...settlement.votes.map((vote) => vote.observation),
  ].filter((observation) => observation !== undefined);
}

// Highest reported slot, ignoring slots above slotLimit when one is given.
export function minimumSlot(transfer: TransferState, slotLimit?: bigint) {
  return reportedObservations(transfer).reduce((slot, observation) => {
    const observedSlot = observation.slot;
    return observedSlot > slot &&
      (slotLimit === undefined || observedSlot <= slotLimit)
      ? observedSlot
      : slot;
  }, 0n);
}

// How a finalized Solana outcome relates to the Akash settlement and its
// reported observations.
export function compareOutcome(transfer: TransferState, outcome: Outcome) {
  const settlement = transfer.settlement;
  assert.ok(settlement, "missing settlement");
  const mint = outcome === Outcome.OUTCOME_MINTED;
  return {
    contradicts:
      (transfer.status === Status.STATUS_MINTED && !mint) ||
      ((transfer.status === Status.STATUS_REFUND_PENDING ||
        transfer.status === Status.STATUS_REFUNDED) &&
        mint) ||
      reportedObservations(transfer).some(
        (observation) => observation.outcome !== outcome,
      ),
    terminalMatches:
      (mint && transfer.status === Status.STATUS_MINTED) ||
      (!mint && transfer.status === Status.STATUS_REFUNDED),
  };
}

// A terminal, unheld settlement that agrees with the finalized outcome and
// every report. planOutcome would return no message and no alert, so an audit
// can skip transaction discovery and its block history.
export function settledConsistently(transfer: TransferState, outcome: Outcome) {
  const { contradicts, terminalMatches } = compareOutcome(transfer, outcome);
  return (
    terminalMatches &&
    !contradicts &&
    transfer.settlement?.hold === Hold.HOLD_NONE
  );
}

// Pure policy; the chain remains authoritative for membership, rounds, caps,
// challenge deadlines and exact-once payment when these messages execute.
export function planOutcome(
  reporter: string,
  snapshot: Snapshot,
  observed: Observed,
): Plan {
  const { params, policy, refundUsage, transfer, time, epoch } = snapshot;
  const settlement = transfer.settlement;
  assert.ok(
    settlement &&
      settlement.policy &&
      settlement.policy.version > 0n &&
      settlement.round > 0n,
    "invalid sink settlement",
  );
  assert.ok(
    policy.members.includes(reporter),
    "account is not a current sink reporter",
  );
  reporterThreshold(policy.threshold, policy.members.length);
  assert.equal(
    reporterRatio(policy.threshold),
    reporterRatio(params.reportersThreshold),
    "current policy threshold mismatch",
  );
  const record = decodeRecord(transfer);
  const challengeEnd = challengeSeconds(settlement);
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
      (transfer.status !== Status.STATUS_TRANSFER_PENDING &&
        transfer.status !== Status.STATUS_VOIDED) ||
      reportedObservations(transfer).length > 0
    ) {
      result.alerts.push(
        "Akash reports an outcome without a finalized Solana status",
      );
      if (
        transfer.status === Status.STATUS_REFUND_PENDING &&
        settlement.hold === Hold.HOLD_NONE &&
        time < challengeEnd
      ) {
        result.messages.push({
          typeUrl: "/akash.sink.v1.MsgVetoVoid",
          value: MsgVetoVoid.fromPartial({
            signer: reporter,
            id: sinkIdParts(record.id),
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
  const { contradicts } = compareOutcome(transfer, observation.outcome);
  if (contradicts)
    result.alerts.push("Akash settlement contradicts finalized Solana outcome");
  if (settlement.hold !== Hold.HOLD_NONE)
    result.alerts.push("Transfer requires governance review");
  const currentPolicy = settlement.policy.version === policy.version;
  const alreadyReported = settlement.votes.some(
    (vote) =>
      vote.reporter === reporter &&
      vote.policyVersion === policy.version &&
      vote.observation?.outcome === observation.outcome,
  );
  const active =
    settlement.hold === Hold.HOLD_NONE &&
    (transfer.status === Status.STATUS_TRANSFER_PENDING ||
      transfer.status === Status.STATUS_VOIDED);
  const refresh =
    !currentPolicy &&
    settlement.hold === Hold.HOLD_NONE &&
    !settlement.governanceObservation &&
    (active || transfer.status === Status.STATUS_REFUND_PENDING);
  if (
    settlement.hold !== Hold.HOLD_CONFLICT &&
    (refresh || (!alreadyReported && (contradicts || active)))
  ) {
    result.messages.push({
      typeUrl: "/akash.sink.v1.MsgReportOutcome",
      value: MsgReportOutcome.fromPartial({
        reporter,
        id: sinkIdParts(record.id),
        policyVersion: policy.version,
        round: settlement.round,
        observation,
      }),
    });
    return result; // Re-read after voting before considering any refund.
  }
  const spent = coinAmount(refundUsage.amount);
  if (
    !mint &&
    transfer.status === Status.STATUS_REFUND_PENDING &&
    settlement.hold === Hold.HOLD_NONE &&
    !contradicts &&
    (currentPolicy || settlement.governanceObservation !== undefined) &&
    params.refundsEnabled &&
    epoch > 0n &&
    time >= challengeEnd &&
    challengeEnd > 0n &&
    spent + record.amount <= coinAmount(params.maxEpochRefund, true) &&
    snapshot.refundHead === record.id
  ) {
    result.messages.push({
      typeUrl: "/akash.sink.v1.MsgFinalizeRefund",
      value: MsgFinalizeRefund.fromPartial({
        executor: reporter,
        id: sinkIdParts(record.id),
      }),
    });
  }
  return result;
}
