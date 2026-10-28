import assert from "node:assert/strict";
import { processes, running, stop, up } from "./stack";
import { PROFILE, sleep } from "./common";
import { monitor, monitorSettings } from "./monitor";

// Run in a systemd cgroup so an unexpected supervisor exit also stops children.
async function main() {
  assert.equal(PROFILE, "testnetify");
  let stopping = false;
  const shutdown = new AbortController();
  for (const signal of ["SIGINT", "SIGTERM"])
    process.on(signal, () => {
      stopping = true;
      shutdown.abort();
    });
  // Health checks run beside process recovery, including while up() waits,
  // and write monitor.json in the runtime directory.
  const settings = monitorSettings(process.env);
  const monitoring = new AbortController();
  const checks = monitor(
    AbortSignal.any([shutdown.signal, monitoring.signal]),
    settings,
  );
  try {
    await up(shutdown.signal);
    while (!stopping) {
      const services = (await processes()).filter(
        (p) => p.name !== "testnetify",
      );
      if (
        services.length !== 10 ||
        (await Promise.all(services.map(running))).some((ok) => !ok)
      ) {
        console.log(
          "A bridge process stopped; restoring the existing network.",
        );
        await up(shutdown.signal);
      }
      for (let i = 0; i < 15 && !stopping; i++) await sleep(1000);
    }
  } catch (error) {
    if (!stopping) throw error;
  } finally {
    monitoring.abort();
    await checks;
    for (const entry of (await processes()).reverse()) await stop(entry);
  }
}

void main().catch((error: unknown) => {
  console.error(error);
  process.exitCode = 1;
});
