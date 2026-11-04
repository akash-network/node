import {
  Hold,
  Outcome,
  SettlementData,
  TransferState,
  Sink_Status as Status,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import assert from "node:assert/strict";
import test from "node:test";
import { testId, testSnapshot } from "../tests/sink-fixtures";
import {
  minimumSlot,
  planOutcome,
  reporterThreshold,
  settledConsistently,
  type Snapshot,
} from "./policy";
import type { Observed } from "./solana";
function resolved(
  outcome = Outcome.OUTCOME_VOIDED,
): Extract<Observed, { kind: "resolved" }> {
  return {
    kind: "resolved",
    contextSlot: 110n,
    observation: {
      outcome,
      slot: 100n,
      transactionSignature: new Uint8Array(64).fill(7),
    },
  };
}
function voided() {
  const snapshot = testSnapshot();
  const settlement = SettlementData.fromPartial({
    ...snapshot.transfer.settlement,
    challengeEnd: snapshot.time,
    votes: ["reporter", "b", "c"].map((reporter) => ({
      reporter,
      policyVersion: 1n,
      observation: resolved().observation,
    })),
  });
  snapshot.transfer.settlement = settlement;
  snapshot.transfer.status = Status.STATUS_REFUND_PENDING;
  return { snapshot, settlement };
}
test("reports exact composite identity, current policy and round; dedupes only same outcome/version", () => {
  const snapshot = testSnapshot(),
    observed = resolved(Outcome.OUTCOME_MINTED);
  const message = planOutcome("reporter", snapshot, observed).messages[0];
  assert.ok(message?.typeUrl === "/akash.sink.v1.MsgReportOutcome");
  assert.deepEqual(message.value.id, snapshot.transfer.id);
  assert.deepEqual(message.value.observation, observed.observation);
  assert.equal(message.value.policyVersion, 1n);
  assert.equal(message.value.round, 1n);
  const settlement = snapshot.transfer.settlement;
  assert.ok(settlement);
  settlement.votes.push({
    reporter: "reporter",
    policyVersion: 1n,
    observation: observed.observation,
  });
  assert.equal(planOutcome("reporter", snapshot, observed).messages.length, 0);
  snapshot.policy.version = 9007199254740993n;
  const next = planOutcome("reporter", snapshot, observed).messages[0];
  assert.ok(next?.typeUrl === "/akash.sink.v1.MsgReportOutcome");
  assert.equal(next.value.policyVersion, snapshot.policy.version);
  assert.equal(
    planOutcome("reporter", snapshot, resolved()).messages.length,
    1,
  );
});
test("absence never creates a void or deadline refund", () => {
  const snapshot = testSnapshot();
  assert.ok(snapshot.transfer.settlement);
  snapshot.transfer.status = Status.STATUS_VOIDED;
  snapshot.transfer.settlement.voidAuthorization = {
    requestedHeight: 1n,
    requestedRound: 1n,
  };
  assert.deepEqual(
    planOutcome("reporter", snapshot, { kind: "absent", contextSlot: 110n }),
    { messages: [], alerts: [] },
  );
  const { snapshot: unpaid } = voided();
  unpaid.time += 86400n;
  assert.equal(
    planOutcome("reporter", unpaid, { kind: "absent", contextSlot: 110n })
      .messages.length,
    0,
  );
});
test("fresh absence vetoes only during challenge and never behind plausible evidence", () => {
  const { snapshot, settlement } = voided();
  snapshot.time--;
  assert.equal(minimumSlot(snapshot.transfer), 100n);
  assert.throws(
    () =>
      planOutcome("reporter", snapshot, { kind: "absent", contextSlot: 99n }),
    /behind/,
  );
  assert.equal(
    planOutcome("reporter", snapshot, { kind: "absent", contextSlot: 110n })
      .messages[0]?.typeUrl,
    "/akash.sink.v1.MsgVetoVoid",
  );
  settlement.votes[0].observation = {
    outcome: Outcome.OUTCOME_VOIDED,
    slot: 18446744073709551615n,
    transactionSignature: new Uint8Array(64).fill(8),
  };
  const plan = planOutcome("reporter", snapshot, {
    kind: "absent",
    contextSlot: 110n,
    slotLimit: 710n,
  });
  assert.match(plan.alerts.join(), /beyond any plausible/);
  assert.equal(plan.messages.length, 1);
  settlement.hold = Hold.HOLD_VETO;
  assert.equal(
    planOutcome("reporter", snapshot, {
      kind: "absent",
      contextSlot: 110n,
      slotLimit: 710n,
    }).messages.length,
    0,
  );
});
test("refund requires finalized void, queue head, authorization, challenge, cap, enablement and no hold", () => {
  assert.equal(
    planOutcome("reporter", voided().snapshot, resolved()).messages[0]?.typeUrl,
    "/akash.sink.v1.MsgFinalizeRefund",
  );
  const changes: ((s: Snapshot) => void)[] = [
    (s) => {
      s.time--;
    },
    (s) => {
      s.params.refundsEnabled = false;
    },
    (s) => {
      s.refundUsage.amount = { denom: "uakt", amount: "41" };
    },
    (s) => {
      s.policy.version++;
    },
    (s) => {
      s.refundHead = testId(9);
    },
    (s) => {
      s.refundHead = undefined;
    },
    (s) => {
      assert.ok(s.transfer.settlement);
      s.transfer.settlement.hold = Hold.HOLD_CONFLICT;
    },
  ];
  for (const change of changes) {
    const { snapshot } = voided();
    change(snapshot);
    assert.ok(
      planOutcome("reporter", snapshot, resolved()).messages.every(
        (m) => m.typeUrl !== "/akash.sink.v1.MsgFinalizeRefund",
      ),
    );
  }
});
test("refund capacity follows the native epoch without shortening the challenge", () => {
  const { snapshot, settlement } = voided();
  snapshot.refundUsage.amount = { denom: "uakt", amount: "100" };
  snapshot.time += 86400n;
  assert.equal(
    planOutcome("reporter", snapshot, resolved()).messages.length,
    0,
  );

  snapshot.epoch++;
  snapshot.refundUsage = {
    epoch: snapshot.epoch,
    amount: { denom: "uakt", amount: "0" },
  };
  settlement.challengeEnd = snapshot.time + 1n;
  assert.equal(
    planOutcome("reporter", snapshot, resolved()).messages.length,
    0,
  );
  snapshot.time++;
  assert.equal(
    planOutcome("reporter", snapshot, resolved()).messages[0]?.typeUrl,
    "/akash.sink.v1.MsgFinalizeRefund",
  );

  snapshot.epoch = 0n;
  snapshot.refundUsage.epoch = 0n;
  assert.equal(
    planOutcome("reporter", snapshot, resolved()).messages.length,
    0,
  );
});
test("governance refund authorization survives policy rotation; paid and minted authorization is retained", () => {
  const { snapshot, settlement } = voided();
  settlement.governanceObservation = resolved().observation;
  settlement.votes = [];
  snapshot.policy.version++;
  assert.equal(
    planOutcome("reporter", snapshot, resolved()).messages[0]?.typeUrl,
    "/akash.sink.v1.MsgFinalizeRefund",
  );
  snapshot.transfer.status = Status.STATUS_REFUNDED;
  settlement.challengeEnd = 0n;
  assert.equal(
    planOutcome("reporter", snapshot, resolved()).messages.length,
    0,
  );
  const conflict = planOutcome(
    "reporter",
    snapshot,
    resolved(Outcome.OUTCOME_MINTED),
  );
  assert.match(conflict.alerts.join(), /contradicts/);
  assert.equal(
    conflict.messages[0]?.typeUrl,
    "/akash.sink.v1.MsgReportOutcome",
  );
  assert.equal(settlement.policy?.version, 1n);
  assert.ok(settlement.governanceObservation);
});
test("decimal quorum is exact and financial terminal state does not conceal holds or opposing votes", () => {
  assert.equal(reporterThreshold("0.75", 4), 3);
  assert.equal(reporterThreshold("0.6", 4), 3);
  assert.equal(reporterThreshold("0.666666666666666666", 4), 3);
  assert.equal(reporterThreshold("0.666666666666666667", 3), 3);
  assert.throws(() => reporterThreshold("0.666666666666666666", 3));
  assert.throws(() => reporterThreshold("1.01", 0));
  for (const ratio of ["0.5", "1.01", "NaN"])
    assert.throws(() => reporterThreshold(ratio, 4));
  const snapshot = testSnapshot();
  snapshot.policy.threshold = "0.750000000000000000";
  assert.equal(
    planOutcome("reporter", snapshot, resolved()).messages.length,
    1,
  );
  snapshot.params.reportersThreshold = "0.74";
  assert.throws(
    () => planOutcome("reporter", snapshot, resolved()),
    /threshold mismatch/,
  );
  snapshot.policy.threshold = "0.5";
  snapshot.params.reportersThreshold = "0.5";
  assert.throws(() => planOutcome("reporter", snapshot, resolved()), /quorum/);
  const transfer = TransferState.fromPartial({
    status: Status.STATUS_MINTED,
    settlement: { policy: { version: 1n } },
  });
  const settlement = transfer.settlement;
  assert.ok(settlement);
  assert.ok(settledConsistently(transfer, Outcome.OUTCOME_MINTED));
  settlement.hold = Hold.HOLD_CONFLICT;
  assert.ok(!settledConsistently(transfer, Outcome.OUTCOME_MINTED));
  settlement.hold = Hold.HOLD_NONE;
  settlement.votes.push({
    reporter: "b",
    policyVersion: 1n,
    observation: resolved().observation,
  });
  assert.ok(!settledConsistently(transfer, Outcome.OUTCOME_MINTED));
});

test("a frozen certificate does not buy repeated same-outcome votes from the remaining reporter", () => {
  const { snapshot } = voided();
  snapshot.time--;
  assert.equal(planOutcome("d", snapshot, resolved()).messages.length, 0);
  const opposite = planOutcome("d", snapshot, resolved(Outcome.OUTCOME_MINTED));
  assert.equal(
    opposite.messages[0]?.typeUrl,
    "/akash.sink.v1.MsgReportOutcome",
  );
  const settlement = snapshot.transfer.settlement;
  assert.ok(settlement);
  settlement.hold = Hold.HOLD_CONFLICT;
  settlement.votes.push({
    reporter: "d",
    policyVersion: snapshot.policy.version,
    observation: resolved(Outcome.OUTCOME_MINTED).observation,
  });
  for (const outcome of [Outcome.OUTCOME_MINTED, Outcome.OUTCOME_VOIDED]) {
    const plan = planOutcome("reporter", snapshot, resolved(outcome));
    assert.equal(plan.messages.length, 0);
    assert.match(plan.alerts.join(), /governance review/);
  }
});

test("policy rotation refreshes only unheld unpaid attempts and retains held evidence", () => {
  const { snapshot, settlement } = voided();
  snapshot.policy.version++;
  assert.equal(
    planOutcome("d", snapshot, resolved()).messages[0]?.typeUrl,
    "/akash.sink.v1.MsgReportOutcome",
  );
  settlement.hold = Hold.HOLD_VETO;
  assert.equal(planOutcome("d", snapshot, resolved()).messages.length, 0);
  assert.equal(
    planOutcome("d", snapshot, resolved(Outcome.OUTCOME_MINTED)).messages[0]
      ?.typeUrl,
    "/akash.sink.v1.MsgReportOutcome",
  );
  settlement.hold = Hold.HOLD_CONFLICT;
  assert.equal(
    planOutcome("d", snapshot, resolved(Outcome.OUTCOME_MINTED)).messages
      .length,
    0,
  );
});
