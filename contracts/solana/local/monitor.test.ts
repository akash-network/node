import assert from "node:assert/strict";
import { test } from "node:test";
import {
  AlertLog,
  Progress,
  balanceLevel,
  clientLevel,
  diskLevel,
  monitorSettings,
  pinLevel,
  worst,
} from "./monitor";

test("disk warns at 80% and is critical at 90% used", () => {
  assert.equal(diskLevel(0.79), "ok");
  assert.equal(diskLevel(0.8), "warning");
  assert.equal(diskLevel(0.899), "warning");
  assert.equal(diskLevel(0.9), "critical");
  assert.equal(diskLevel(1), "critical");
});

test("client age warns at 25% and is critical at 50% of trust", () => {
  assert.equal(clientLevel(21_599, 86_400, false), "ok");
  assert.equal(clientLevel(21_600, 86_400, false), "warning");
  assert.equal(clientLevel(43_200, 86_400, false), "critical");
  // The 2026-10-18 outage: 29 hours against a 24-hour client.
  assert.equal(clientLevel(29 * 3600, 86_400, false), "critical");
  assert.equal(clientLevel(0, 86_400, true), "critical");
});

test("payer balance thresholds", () => {
  const settings = monitorSettings({});
  assert.equal(settings.payerWarnLamports, 5_000_000_000);
  assert.equal(settings.payerCriticalLamports, 1_000_000_000);
  const level = (lamports: number) =>
    balanceLevel(
      lamports,
      settings.payerWarnLamports,
      settings.payerCriticalLamports,
    );
  assert.equal(level(5_000_000_000), "ok");
  assert.equal(level(4_999_999_999), "warning");
  assert.equal(level(999_999_999), "critical");
});

test("settings reject invalid values", () => {
  assert.equal(monitorSettings({}).stallSeconds, 300);
  assert.equal(
    monitorSettings({ AKT_MONITOR_STALL_SECONDS: "600" }).stallSeconds,
    600,
  );
  for (const value of ["", "0", "-1", "x"])
    assert.throws(() =>
      monitorSettings({ AKT_MONITOR_INTERVAL_SECONDS: value }),
    );
  assert.throws(() =>
    monitorSettings({
      AKT_MONITOR_PAYER_WARN_SOL: "1",
      AKT_MONITOR_PAYER_CRITICAL_SOL: "2",
    }),
  );
});

test("a height that stops advancing ages, including while unreachable", () => {
  const progress = new Progress();
  assert.deepEqual(progress.observe(10, 0), { value: 10, stalledSeconds: 0 });
  assert.deepEqual(progress.observe(12, 5000), {
    value: 12,
    stalledSeconds: 0,
  });
  assert.equal(progress.observe(12, 65_000).stalledSeconds, 60);
  assert.equal(progress.observe(undefined, 305_000).stalledSeconds, 300);
  // A lower value from a lagging node is not progress.
  assert.equal(progress.observe(11, 306_000).stalledSeconds, 301);
  assert.equal(progress.observe(13, 307_000).stalledSeconds, 0);
  const unreachable = new Progress();
  assert.equal(unreachable.observe(undefined, 1000).stalledSeconds, 0);
  assert.equal(unreachable.observe(undefined, 301_000).stalledSeconds, 300);
});

test("alerts log on change and repeat while unhealthy", () => {
  const log = new AlertLog(60_000);
  assert.equal(log.due("disk", "ok", 0), false);
  assert.equal(log.due("disk", "warning", 1000), true);
  assert.equal(log.due("disk", "warning", 30_000), false);
  assert.equal(log.due("disk", "critical", 31_000), true);
  assert.equal(log.due("disk", "critical", 90_000), false);
  assert.equal(log.due("disk", "critical", 91_000), true);
  assert.equal(log.due("disk", "ok", 92_000), true);
  assert.equal(log.due("disk", "ok", 500_000), false);
  assert.equal(log.due("payer", "warning", 0), true);
});

test("overall status is the worst check", () => {
  assert.equal(worst([]), "ok");
  assert.equal(worst(["ok", "warning", "ok"]), "warning");
  assert.equal(worst(["warning", "critical", "ok"]), "critical");
});

test("a re-pinned client warns once; a payer without update rights warns", () => {
  assert.equal(pinLevel(undefined, "A", "member"), "ok");
  assert.equal(pinLevel("A", "A", "open"), "ok");
  assert.equal(pinLevel("A", "B", "member"), "warning");
  assert.equal(pinLevel("B", "B", "member"), "ok");
  assert.equal(pinLevel("B", "B", "none"), "warning");
});
