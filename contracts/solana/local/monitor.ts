import { statfs } from "node:fs/promises";
import { join } from "node:path";
import { web3 } from "@coral-xyz/anchor";
import { AKASH_RPC, ROOT, object, provider, sleep } from "./common";
import { atomicJson } from "./durable";
import {
  clientProgram,
  discoverLightClient,
  lightClientAccounts,
  readRelayerAccess,
  type RelayerAccess,
} from "./light-client";

// Health checks for the supervised stack. They only read and report: nothing
// here deletes data, prunes ledgers or changes chain state.

export type Level = "ok" | "warning" | "critical";
const rank: Record<Level, number> = { ok: 0, warning: 1, critical: 2 };
export const worst = (levels: Level[]) =>
  levels.reduce<Level>((a, b) => (rank[b] > rank[a] ? b : a), "ok");

function positive(
  env: Record<string, string | undefined>,
  name: string,
  fallback: number,
) {
  const value = Number(env[name] ?? fallback);
  if (!(Number.isFinite(value) && value > 0))
    throw new Error(`${name} must be a positive number`);
  return value;
}
export function monitorSettings(env: Record<string, string | undefined>) {
  const settings = {
    intervalSeconds: positive(env, "AKT_MONITOR_INTERVAL_SECONDS", 60),
    stallSeconds: positive(env, "AKT_MONITOR_STALL_SECONDS", 300),
    payerWarnLamports:
      positive(env, "AKT_MONITOR_PAYER_WARN_SOL", 5) * web3.LAMPORTS_PER_SOL,
    payerCriticalLamports:
      positive(env, "AKT_MONITOR_PAYER_CRITICAL_SOL", 1) *
      web3.LAMPORTS_PER_SOL,
  };
  if (settings.payerCriticalLamports > settings.payerWarnLamports)
    throw new Error("payer critical balance must not exceed its warning");
  return settings;
}

export function diskLevel(usedFraction: number): Level {
  if (usedFraction >= 0.9) return "critical";
  return usedFraction >= 0.8 ? "warning" : "ok";
}

export function clientLevel(
  ageSeconds: number,
  trustingPeriodSeconds: number,
  frozen: boolean,
): Level {
  if (frozen || ageSeconds >= trustingPeriodSeconds * 0.5) return "critical";
  return ageSeconds >= trustingPeriodSeconds * 0.25 ? "warning" : "ok";
}

export function balanceLevel(
  lamports: number,
  warnLamports: number,
  criticalLamports: number,
): Level {
  if (lamports < criticalLamports) return "critical";
  return lamports < warnLamports ? "warning" : "ok";
}

// The client the bridge pins. A change since the last pass warns once; a
// payer that may not update the client warns while it lasts.
export function pinLevel(
  previous: string | undefined,
  current: string,
  access: RelayerAccess,
): Level {
  if (previous !== undefined && previous !== current) return "warning";
  return access === "none" ? "warning" : "ok";
}

// Seconds since a height or slot last increased. An unreachable node reports
// undefined and keeps aging from its last observed advance.
export class Progress {
  private value?: number;
  private changedAt?: number;
  observe(value: number | undefined, now: number) {
    this.changedAt ??= now;
    if (
      value !== undefined &&
      (this.value === undefined || value > this.value)
    ) {
      this.value = value;
      this.changedAt = now;
    }
    return { value: this.value, stalledSeconds: (now - this.changedAt) / 1000 };
  }
}

// Log a check when its level changes and repeat while it stays unhealthy.
export class AlertLog {
  private readonly last = new Map<string, { level: Level; at: number }>();
  constructor(private readonly repeatMs: number) {}
  due(check: string, level: Level, now: number) {
    const previous = this.last.get(check);
    const changed = (previous?.level ?? "ok") !== level;
    const repeat =
      level !== "ok" &&
      previous !== undefined &&
      now - previous.at >= this.repeatMs;
    if (changed || repeat) this.last.set(check, { level, at: now });
    return changed || repeat;
  }
}

type Check = { level: Level } & Record<string, unknown>;
const failure = (error: unknown) =>
  error instanceof Error ? error.message : String(error);

async function akashHeight() {
  const reply = await fetch(AKASH_RPC, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      jsonrpc: "2.0",
      id: 1,
      method: "status",
      params: {},
    }),
    signal: AbortSignal.timeout(10_000),
  });
  if (!reply.ok) throw new Error(`Akash RPC HTTP ${reply.status}`);
  const body = object(await reply.json());
  const height = Number(
    object(object(body.result).sync_info).latest_block_height,
  );
  if (!Number.isSafeInteger(height)) throw new Error("invalid Akash height");
  return height;
}

export function monitorChecks(settings: ReturnType<typeof monitorSettings>) {
  const akash = new Progress();
  const solana = new Progress();
  let pinned: string | undefined;
  const stalled = (stalledSeconds: number): Level =>
    stalledSeconds >= settings.stallSeconds ? "critical" : "ok";
  return async function collect(): Promise<Record<string, Check>> {
    const checks: Record<string, Check> = {};
    try {
      const fs = await statfs(ROOT);
      const used = (fs.blocks - fs.bfree) * fs.bsize;
      const free = fs.bavail * fs.bsize;
      const usedFraction = used / (used + free);
      checks.disk = {
        level: diskLevel(usedFraction),
        path: ROOT,
        usedPercent: Math.round(usedFraction * 1000) / 10,
        freeBytes: free,
      };
    } catch (error) {
      checks.disk = { level: "warning", path: ROOT, error: failure(error) };
    }
    let height: number | undefined;
    let error: string | undefined;
    try {
      height = await akashHeight();
    } catch (cause) {
      error = failure(cause);
    }
    const akashProgress = akash.observe(height, Date.now());
    checks.akash = {
      level: stalled(akashProgress.stalledSeconds),
      height: akashProgress.value,
      stalledSeconds: Math.round(akashProgress.stalledSeconds),
      ...(error ? { error } : {}),
    };
    let wallet;
    try {
      wallet = await provider();
    } catch (cause) {
      checks.solana = { level: "warning", error: failure(cause) };
      return checks;
    }
    let slot: number | undefined;
    error = undefined;
    try {
      slot = await wallet.connection.getSlot("confirmed");
    } catch (cause) {
      error = failure(cause);
    }
    const solanaProgress = solana.observe(slot, Date.now());
    checks.solana = {
      level: stalled(solanaProgress.stalledSeconds),
      slot: solanaProgress.value,
      stalledSeconds: Math.round(solanaProgress.stalledSeconds),
      ...(error ? { error } : {}),
    };
    try {
      const found = await discoverLightClient(wallet.connection);
      const accounts = lightClientAccounts(found);
      const client = clientProgram(wallet, found.program);
      const state = await client.account.clientState.fetch(
        accounts.clientState,
      );
      const latest = BigInt(state.latestHeight.revisionHeight.toString());
      const consensus = await client.account.consensusStateStore.fetch(
        accounts.consensusAt(latest),
      );
      const program = found.program.toBase58();
      const access = await readRelayerAccess(wallet, found);
      const changedFrom =
        pinned !== undefined && pinned !== program ? pinned : undefined;
      pinned = program;
      // Wall time is at least the Solana clock while the validator lags.
      const ageSeconds =
        Date.now() / 1000 -
        Number(
          BigInt(consensus.consensusState.timestamp.toString()) /
            1_000_000_000n,
        );
      const trustingPeriodSeconds = state.trustingPeriod.toNumber();
      const frozen = !state.frozenHeight.revisionHeight.isZero();
      checks.lightClient = {
        level: worst([
          clientLevel(ageSeconds, trustingPeriodSeconds, frozen),
          pinLevel(changedFrom, program, access),
        ]),
        program,
        ...(changedFrom ? { changedFrom } : {}),
        chainId: found.chainId,
        relayerAccess: access,
        latestHeight: latest.toString(),
        ageSeconds: Math.round(ageSeconds),
        trustingPeriodSeconds,
        usedPercent:
          Math.round((ageSeconds / trustingPeriodSeconds) * 1000) / 10,
        frozen,
      };
    } catch (cause) {
      checks.lightClient = { level: "warning", error: failure(cause) };
    }
    try {
      const lamports = await wallet.connection.getBalance(wallet.publicKey);
      checks.payer = {
        level: balanceLevel(
          lamports,
          settings.payerWarnLamports,
          settings.payerCriticalLamports,
        ),
        address: wallet.publicKey.toBase58(),
        lamports,
      };
    } catch (cause) {
      checks.payer = { level: "warning", error: failure(cause) };
    }
    return checks;
  };
}

// journald reads a leading <N> as the syslog priority, so
// `journalctl -u akt-bridge -p warning` shows only these alerts.
const priority: Record<Level, string> = {
  ok: "<5>",
  warning: "<4>",
  critical: "<2>",
};

export async function monitor(
  signal: AbortSignal,
  settings: ReturnType<typeof monitorSettings>,
) {
  const collect = monitorChecks(settings);
  const alerts = new AlertLog(15 * 60_000);
  while (!signal.aborted) {
    try {
      const checks = await collect();
      const now = Date.now();
      for (const [check, value] of Object.entries(checks))
        if (alerts.due(check, value.level, now))
          console.log(
            priority[value.level] +
              JSON.stringify({
                event: value.level === "ok" ? "monitor-clear" : "monitor-alert",
                check,
                ...value,
              }),
          );
      const status = {
        checkedAt: new Date(now).toISOString(),
        status: worst(Object.values(checks).map((value) => value.level)),
        checks,
      };
      try {
        await atomicJson(join(ROOT, "monitor.json"), status);
      } catch (error) {
        console.log(
          priority.critical +
            JSON.stringify({
              event: "monitor-write-failed",
              error: failure(error),
            }),
        );
      }
    } catch (error) {
      console.log(
        priority.warning +
          JSON.stringify({ event: "monitor-failed", error: failure(error) }),
      );
    }
    for (let i = 0; i < settings.intervalSeconds && !signal.aborted; i++)
      await sleep(1000);
  }
}
