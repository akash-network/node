import assert from "node:assert/strict";
import { test } from "node:test";
import { Sink_Status as Status } from "@akashnetwork/chain-sdk/private-types/akash.v1";
import {
  inSnapshot,
  reconcileSettings,
  transferAlerts,
} from "./reconcile-policy";

const base = { id: 7n, slot: 10_000, reportLagSeconds: 600 };

test("a void on Akash without a Solana status is unhealthy", () => {
  const alerts = transferAlerts({ ...base, akash: Status.STATUS_VOIDED });
  assert.equal(alerts.length, 1);
  assert.match(alerts[0], /void 7 on Akash lacks any finalized Solana status/);
  assert.deepEqual(
    transferAlerts({
      ...base,
      akash: Status.STATUS_VOIDED,
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
    /minted transfer 7 has STATUS_REFUNDED.*refund 7 lacks finalized Solana void/,
  );
  assert.match(
    transferAlerts({ ...base, akash: Status.STATUS_MINTED }).join(";"),
    /mint confirmation 7 lacks finalized Solana mint/,
  );
  assert.deepEqual(
    transferAlerts({ ...base, akash: Status.STATUS_PENDING }),
    [],
  );
});

test("a Solana outcome still pending on Akash alerts after the lag", () => {
  const pending = { ...base, akash: Status.STATUS_PENDING };
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
  assert.match(late[0], /transfer 7 voided on Solana about 600 s ago/);
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
