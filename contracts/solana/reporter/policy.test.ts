import assert from "node:assert/strict";
import test from "node:test";
import { web3 } from "@coral-xyz/anchor";
import Long from "long";
import {
  Hold,
  Outcome,
  Observation,
  Sink_Params as Params,
  Settlement,
  Sink_Status as Status,
  Totals,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import {
  minimumSlot,
  planOutcome,
  refundQueueHead,
  settledConsistently,
  type Snapshot,
} from "./policy";
import type { Observed } from "./solana";

function fixture(): Snapshot {
  const record = Buffer.alloc(77);
  record[0] = 1;
  record.writeBigUInt64LE(1n, 1);
  web3.Keypair.generate().publicKey.toBuffer().copy(record, 29);
  record.writeBigUInt64LE(60n, 61);
  record.writeBigUInt64LE(1n, 69);
  return {
    params: Params.fromPartial({
      reporters: ["reporter", "b", "c", "d"],
      reporterThreshold: 3,
      refundsEnabled: true,
      dailyRefundCap: 100n,
    }),
    totals: Totals.fromPartial({ reporterEpoch: 1n, refundDay: 1n }),
    time: 86410n,
    transfer: {
      record,
      voidOrdered: false,
      settlement: Settlement.fromPartial({
        round: 1n,
        policy: {
          epoch: 1n,
          members: ["reporter", "b", "c", "d"],
          threshold: 3,
        },
      }),
    },
  };
}
function resolved(outcome: Outcome): Extract<Observed, { kind: "resolved" }> {
  return {
    kind: "resolved",
    contextSlot: 110n,
    observation: Observation.fromPartial({
      outcome,
      slot: 100n,
      transactionSignature: new Uint8Array(64).fill(7),
    }),
  };
}
function voided() {
  const snapshot = fixture();
  const settlement = Settlement.fromPartial({
    ...snapshot.transfer.settlement,
    status: Status.STATUS_VOIDED,
    challengeEnd: snapshot.time,
    votes: ["reporter", "b", "c"].map((reporter) => ({
      reporter,
      observation: resolved(Outcome.OUTCOME_VOIDED).observation,
    })),
  });
  snapshot.transfer.settlement = settlement;
  return { snapshot, settlement };
}

test("reports finalized mints with epoch/round and suppresses repeats after restart", () => {
  const snapshot = fixture();
  const observation = resolved(Outcome.OUTCOME_MINTED);
  const plan = planOutcome("reporter", snapshot, observation);
  assert.equal(plan.messages[0]?.typeUrl, "/akash.sink.v1.MsgReportOutcome");
  const settlement = snapshot.transfer.settlement;
  assert.ok(settlement);
  settlement.votes.push({
    reporter: "reporter",
    observation: observation.observation,
  });
  assert.deepEqual(planOutcome("reporter", snapshot, observation).messages, []);
  snapshot.totals.reporterEpoch = snapshot.totals.reporterEpoch.add(1);
  settlement.round = settlement.round.add(1);
  const next = planOutcome("reporter", snapshot, observation).messages[0];
  assert.ok(next?.typeUrl === "/akash.sink.v1.MsgReportOutcome");
  assert.equal(next.value.reporterEpoch.toBigInt(), 2n);
  assert.equal(next.value.round.toBigInt(), 2n);
});

test("absence never becomes a void report or a timeout refund", () => {
  const snapshot = fixture();
  snapshot.transfer.voidOrdered = true;
  assert.deepEqual(
    planOutcome("reporter", snapshot, { kind: "absent", contextSlot: 110n }),
    { messages: [], alerts: [] },
  );
});

test("fresh absence vetoes a false void during its challenge, lagging reads fail", () => {
  const { snapshot, settlement } = voided();
  snapshot.time--;
  assert.equal(minimumSlot(snapshot.transfer), 100n);
  assert.throws(
    () =>
      planOutcome("reporter", snapshot, { kind: "absent", contextSlot: 99n }),
    /behind/,
  );
  const plan = planOutcome("reporter", snapshot, {
    kind: "absent",
    contextSlot: 110n,
  });
  assert.equal(plan.messages[0]?.typeUrl, "/akash.sink.v1.MsgVetoVoid");
  assert.equal(plan.alerts.length, 1);
  settlement.hold = Hold.HOLD_VETO;
  assert.deepEqual(
    planOutcome("reporter", snapshot, { kind: "absent", contextSlot: 110n })
      .messages,
    [],
  );
  settlement.hold = Hold.HOLD_NONE;
  snapshot.time++;
  assert.deepEqual(
    planOutcome("reporter", snapshot, { kind: "absent", contextSlot: 110n })
      .messages,
    [],
  );
});

test("slots beyond a fresh node's plausible limit cannot block a veto", () => {
  // SDK fromPartial reads bigint uint64 values as signed; use unsigned Longs.
  function withSlots(...slots: string[]) {
    const { snapshot, settlement } = voided();
    snapshot.time--;
    settlement.votes.forEach((vote, i) => {
      vote.observation = Observation.fromPartial({
        ...vote.observation,
        slot: Long.fromString(slots[i] ?? slots[0], true),
      });
    });
    return snapshot;
  }
  const absent: Observed = {
    kind: "absent",
    contextSlot: 110n,
    slotLimit: 710n,
  };
  const huge = withSlots("9223372036854775808");
  assert.equal(minimumSlot(huge.transfer), 9_223_372_036_854_775_808n);
  assert.equal(minimumSlot(huge.transfer, 710n), 0n);
  const plan = planOutcome("reporter", huge, absent);
  assert.equal(plan.messages[0]?.typeUrl, "/akash.sink.v1.MsgVetoVoid");
  assert.deepEqual(plan.alerts, [
    "Akash reports a Solana slot beyond any plausible finalized slot",
    "Akash reports an outcome without a finalized Solana status",
  ]);
  assert.equal(
    planOutcome("reporter", withSlots("711"), absent).messages[0]?.typeUrl,
    "/akash.sink.v1.MsgVetoVoid",
  );
  // Without a limit every reported slot bounds absence, as before.
  assert.throws(
    () => planOutcome("reporter", huge, { kind: "absent", contextSlot: 110n }),
    /behind/,
  );
  // A genuine slot slightly ahead of this node, alone or beside a fabricated
  // one, still means the node is behind and must not veto.
  for (const snapshot of [
    withSlots("111"),
    withSlots("710"),
    withSlots("9223372036854775808", "400", "400"),
  ])
    assert.throws(() => planOutcome("reporter", snapshot, absent), /behind/);
  assert.throws(
    () =>
      planOutcome("reporter", huge, {
        kind: "absent",
        contextSlot: 110n,
        slotLimit: 109n,
      }),
    /invalid plausible slot limit/,
  );
});

test("contradictory finalized mint reports conflict even after a refund", () => {
  const { snapshot, settlement } = voided();
  settlement.status = Status.STATUS_REFUNDED;
  assert.ok(settlement.votes[0].observation);
  settlement.votes[0].observation = Observation.fromPartial({
    ...settlement.votes[0].observation,
    slot: 18_446_744_073_709_551_615n,
  });
  const plan = planOutcome(
    "reporter",
    snapshot,
    resolved(Outcome.OUTCOME_MINTED),
  );
  assert.equal(plan.alerts.length, 1);
  assert.equal(plan.messages[0]?.typeUrl, "/akash.sink.v1.MsgReportOutcome");
  assert.equal(plan.messages.length, 1);
});

test("refund needs observed void, current epoch, challenge, cap, no hold and enablement", () => {
  const observed = resolved(Outcome.OUTCOME_VOIDED);
  const { snapshot, settlement } = voided();
  assert.equal(
    planOutcome("reporter", snapshot, observed).messages[0]?.typeUrl,
    "/akash.sink.v1.MsgFinalizeRefund",
  );
  for (const change of [
    (s: Snapshot) => {
      s.time--;
    },
    (s: Snapshot) => {
      s.params.refundsEnabled = false;
    },
    (s: Snapshot) => {
      s.totals = Totals.fromPartial({ ...s.totals, refundedToday: 41n });
    },
    (s: Snapshot) => {
      assert.ok(s.transfer.settlement);
      s.transfer.settlement.hold = Hold.HOLD_VETO;
    },
    (s: Snapshot) => {
      s.totals.reporterEpoch = s.totals.reporterEpoch.add(1);
    },
    (s: Snapshot) => {
      s.refundHead = 999n; // x/sink pays an older refund first
    },
  ]) {
    const next = voided().snapshot;
    change(next);
    assert.ok(
      planOutcome("reporter", next, observed).messages.every(
        (msg) => msg.typeUrl !== "/akash.sink.v1.MsgFinalizeRefund",
      ),
    );
  }
  snapshot.totals = Totals.fromPartial({
    ...snapshot.totals,
    refundedToday: 100n,
    refundDay: 0n,
  });
  assert.equal(
    planOutcome("reporter", snapshot, observed).messages[0]?.typeUrl,
    "/akash.sink.v1.MsgFinalizeRefund",
  );
  settlement.status = Status.STATUS_REFUNDED;
  assert.deepEqual(planOutcome("reporter", snapshot, observed).messages, []);
  assert.throws(
    () => planOutcome("outsider", snapshot, observed),
    /not a current/,
  );
});

test("SDK uint64 epochs and refund limits retain precision above Number.MAX_SAFE_INTEGER", () => {
  const high = 9_007_199_254_740_993n;
  const { snapshot, settlement } = voided();
  const record = Buffer.from(snapshot.transfer.record);
  record.writeBigUInt64LE(high, 61);
  snapshot.transfer.record = record;
  snapshot.params = Params.fromPartial({
    ...snapshot.params,
    dailyRefundCap: high,
  });
  snapshot.totals = Totals.fromPartial({
    ...snapshot.totals,
    reporterEpoch: high,
  });
  snapshot.transfer.settlement = Settlement.fromPartial({
    ...settlement,
    policy: { ...settlement.policy, epoch: high },
  });
  const observed = resolved(Outcome.OUTCOME_VOIDED);
  assert.equal(
    planOutcome("reporter", snapshot, observed).messages[0]?.typeUrl,
    "/akash.sink.v1.MsgFinalizeRefund",
  );
  snapshot.params = Params.fromPartial({
    ...snapshot.params,
    dailyRefundCap: high - 1n,
  });
  assert.deepEqual(planOutcome("reporter", snapshot, observed).messages, []);
});

test("an audit skips evidence only where the plan would do nothing", () => {
  const { MINTED, VOIDED } = {
    MINTED: Outcome.OUTCOME_MINTED,
    VOIDED: Outcome.OUTCOME_VOIDED,
  };
  let skipped = 0;
  for (const status of [
    Status.STATUS_PENDING,
    Status.STATUS_MINTED,
    Status.STATUS_VOIDED,
    Status.STATUS_REFUNDED,
  ])
    for (const hold of [Hold.HOLD_NONE, Hold.HOLD_CONFLICT])
      for (const votes of [[], [MINTED], [VOIDED], [MINTED, VOIDED]])
        for (const governance of [undefined, MINTED, VOIDED])
          for (const outcome of [MINTED, VOIDED]) {
            const snapshot = fixture();
            const settlement = Settlement.fromPartial({
              ...snapshot.transfer.settlement,
              status,
              hold,
              votes: votes.map((vote, i) => ({
                reporter: ["b", "c"][i],
                observation: resolved(vote).observation,
              })),
              governanceObservation:
                governance === undefined
                  ? undefined
                  : resolved(governance).observation,
            });
            snapshot.transfer.settlement = settlement;
            if (!settledConsistently(settlement, outcome)) continue;
            skipped++;
            assert.deepEqual(
              planOutcome("reporter", snapshot, resolved(outcome)),
              { messages: [], alerts: [] },
            );
          }
  // Minted or refunded, unheld, with no report or only agreeing ones.
  assert.equal(skipped, 2 * 2 * 2);
});

test("refund queue head is the oldest unheld void, then the lowest ID", () => {
  const entry = (
    id: bigint,
    status: Status,
    end: bigint,
    hold = Hold.HOLD_NONE,
  ) => ({
    id,
    transfer: {
      record: new Uint8Array(),
      voidOrdered: false,
      settlement: Settlement.fromPartial({
        status,
        hold,
        challengeEnd: Long.fromValue(end),
        round: Long.fromValue(1),
      }),
    },
  });
  assert.equal(refundQueueHead([]), undefined);
  assert.equal(
    refundQueueHead([
      entry(1n, Status.STATUS_PENDING, 0n),
      entry(2n, Status.STATUS_VOIDED, 50n, Hold.HOLD_VETO), // held: out of the queue
      entry(4n, Status.STATUS_VOIDED, 60n),
      entry(3n, Status.STATUS_VOIDED, 60n),
      entry(5n, Status.STATUS_VOIDED, 70n),
      entry(6n, Status.STATUS_REFUNDED, 0n),
    ]),
    3n,
  );
});
