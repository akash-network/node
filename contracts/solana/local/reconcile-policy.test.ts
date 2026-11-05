import { testId } from "../tests/sink-fixtures";
import assert from "node:assert/strict";
import { test } from "node:test";
import {
  Sink_Status as Status,
  Hold,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import {
  inSnapshot,
  reconcileSettings,
  transferAlerts,
} from "./reconcile-policy";

const base = {
  id: testId(7),
  hold: Hold.HOLD_NONE,
  slot: 10_000,
  reportLagSeconds: 600,
};

test("a void on Akash without a Solana status is unhealthy", () => {
  const alerts = transferAlerts({
    ...base,
    akash: Status.STATUS_REFUND_PENDING,
  });
  assert.equal(alerts.length, 1);
  assert.match(
    alerts[0],
    /void .*\/1\/7 on Akash lacks any finalized Solana status/,
  );
  assert.deepEqual(
    transferAlerts({
      ...base,
      akash: Status.STATUS_REFUND_PENDING,
      solana: "voided",
      resolvedAtSlot: 9_990n,
    }),
    [],
  );
});

test("existing settlement contradictions remain alerts", () => {
  assert.match(
    transferAlerts({
      ...base,
      akash: Status.STATUS_REFUNDED,
      solana: "minted",
      resolvedAtSlot: 1n,
    }).join(";"),
    /minted transfer .*\/1\/7 has STATUS_REFUNDED.*refund .*\/1\/7 lacks finalized Solana void/,
  );
  assert.match(
    transferAlerts({ ...base, akash: Status.STATUS_MINTED }).join(";"),
    /mint confirmation .*\/1\/7 lacks finalized Solana mint/,
  );
  assert.deepEqual(
    transferAlerts({ ...base, akash: Status.STATUS_TRANSFER_PENDING }),
    [],
  );
});

test("a Solana outcome still pending on Akash alerts after the lag", () => {
  const pending = { ...base, akash: Status.STATUS_TRANSFER_PENDING };
  // 1,450 slots is about 580 s at 400 ms; 1,500 slots is 600 s.
  assert.deepEqual(
    transferAlerts({ ...pending, solana: "minted", resolvedAtSlot: 8_550n }),
    [],
  );
  const late = transferAlerts({
    ...pending,
    solana: "voided",
    resolvedAtSlot: 8_500n,
  });
  assert.equal(late.length, 1);
  assert.match(late[0], /transfer .*\/1\/7 voided on Solana about 600 s ago/);
  assert.deepEqual(
    transferAlerts({
      ...base,
      akash: Status.STATUS_MINTED,
      solana: "minted",
      resolvedAtSlot: 1n,
    }),
    [],
  );
});

test("paged statuses resolved after the pinned slot are excluded", () => {
  assert.ok(inSnapshot(100n, 100));
  assert.equal(inSnapshot(101n, 100), false);
});

test("report lag threshold defaults to ten minutes", () => {
  assert.equal(reconcileSettings({}).reportLagSeconds, 600);
  assert.equal(
    reconcileSettings({ AKT_RECONCILE_REPORT_LAG_SECONDS: "120" })
      .reportLagSeconds,
    120,
  );
  for (const value of ["", "0", "1.5", "x"])
    assert.throws(() =>
      reconcileSettings({ AKT_RECONCILE_REPORT_LAG_SECONDS: value }),
    );
});

test("terminal conflicts remain monitoring alerts after financial settlement", () => {
  for (const [akash, solana] of [
    [Status.STATUS_MINTED, "minted"],
    [Status.STATUS_REFUNDED, "voided"],
  ] as const) {
    const alerts = transferAlerts({
      ...base,
      akash,
      solana,
      hold: Hold.HOLD_CONFLICT,
    });
    assert.equal(alerts.length, 1);
    assert.match(alerts[0], /HOLD_CONFLICT/);
  }
});

test("a governance void request alone is not a missing Solana outcome", () => {
  assert.deepEqual(
    transferAlerts({ ...base, akash: Status.STATUS_VOIDED }),
    [],
  );
  assert.match(
    transferAlerts({
      ...base,
      akash: Status.STATUS_VOIDED,
      solana: "voided",
      resolvedAtSlot: 1n,
    }).join(),
    /reporters missed/,
  );
});
