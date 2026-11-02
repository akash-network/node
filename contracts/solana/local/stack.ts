import assert from "node:assert/strict";
import { execFile, spawn } from "node:child_process";
import { existsSync, openSync, closeSync } from "node:fs";
import { mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import { randomBytes, createHash } from "node:crypto";
import { createConnection } from "node:net";
import { join } from "node:path";
import { promisify } from "node:util";
import { web3 } from "@coral-xyz/anchor";
import { toBech32 } from "@cosmjs/encoding";
import { AkashClient } from "../reporter/akash";
import {
  ROOT,
  PROFILE,
  AKASH_RPC_PORT,
  AKASH_GRPC_PORT,
  AKASH_REST_PORT,
  SOLANA_RPC_PORT,
  REPO,
  AKASH,
  AKASH_HOME,
  CHAIN,
  AKASH_RPC,
  SOLANA_RPC,
  PROGRAM_ID,
  CLIENT_ID,
  ACCESS_ID,
  cosmosWallet,
  json,
  object,
  save,
  provider,
  pda,
  sleep,
} from "./common";

const exec = promisify(execFile);
type ProcessEntry = { name: string; pid: number; marker: string };
export async function processes(): Promise<ProcessEntry[]> {
  if (!existsSync(join(ROOT, "processes.json"))) return [];
  const value = await json(join(ROOT, "processes.json"));
  assert.ok(Array.isArray(value));
  return value.map((entry: unknown) => {
    const row = object(entry);
    assert.ok(
      typeof row.name === "string" &&
        typeof row.pid === "number" &&
        Number.isSafeInteger(row.pid) &&
        row.pid > 1 &&
        typeof row.marker === "string" &&
        row.marker.startsWith(ROOT),
    );
    return { name: row.name, pid: row.pid, marker: row.marker };
  });
}
export async function running(entry: ProcessEntry) {
  try {
    return (
      await exec("ps", ["-p", String(entry.pid), "-o", "args="])
    ).stdout.includes(entry.marker);
  } catch {
    return false;
  }
}
export async function start(
  name: string,
  command: string,
  args: string[],
  marker: string,
  ports: number[] = [],
) {
  const entries = await processes();
  const existing = entries.find((entry) => entry.name === name);
  if (existing && (await running(existing))) return;
  // Never bootstrap or fund a different local validator that owns these ports.
  for (const port of ports) {
    const occupied = await new Promise<boolean>((resolve, reject) => {
      const socket = createConnection({ host: "127.0.0.1", port });
      socket.once("connect", () => {
        socket.destroy();
        resolve(true);
      });
      socket.once("error", (error: NodeJS.ErrnoException) => {
        socket.destroy();
        if (error.code === "ECONNREFUSED") resolve(false);
        else reject(error);
      });
      socket.setTimeout(1000, () => {
        socket.destroy();
        reject(new Error(`port ${port} probe timed out`));
      });
    });
    assert.ok(
      !occupied,
      `port ${port} is occupied by an unmanaged process; stop its owner before starting ${name}`,
    );
  }
  const log = openSync(join(ROOT, `${name}.log`), "a", 0o600);
  const child = spawn(command, args, {
    cwd: REPO,
    detached: true,
    stdio: ["ignore", log, log],
    env: process.env,
  });
  closeSync(log);
  await new Promise<void>((resolve, reject) => {
    child.once("spawn", resolve);
    child.once("error", reject);
  });
  assert.ok(child.pid);
  child.unref();
  await save(join(ROOT, "processes.json"), [
    ...entries.filter((entry) => entry.name !== name),
    { name, pid: child.pid, marker },
  ]);
  console.log(`${name} started, PID ${child.pid}`);
}
export async function ready(name: string, check: () => Promise<unknown>) {
  const deadline = Date.now() + 60_000;
  while (Date.now() < deadline) {
    const entry = (await processes()).find((entry) => entry.name === name);
    assert.ok(
      entry && (await running(entry)),
      `${name} exited; inspect ${join(ROOT, `${name}.log`)}`,
    );
    try {
      await check();
      return;
    } catch {
      await sleep(500);
    }
  }
  throw new Error(
    `${name} did not become ready; inspect ${join(ROOT, `${name}.log`)}`,
  );
}

export async function init() {
  await mkdir(ROOT, { recursive: true, mode: 0o700 });
  assert.ok(existsSync(AKASH), `Missing ${AKASH}; run npm run setup first`);
  if (!existsSync(join(ROOT, "solana-keypair.json")))
    await save(
      join(ROOT, "solana-keypair.json"),
      Array.from(web3.Keypair.generate().secretKey),
    );
  const names = ["sender", "reporter0", "reporter1", "reporter2", "reporter3"];
  const addresses = new Map<string, string>();
  for (const name of names) {
    const path = join(ROOT, `${name}.key`);
    if (!existsSync(path))
      await writeFile(path, randomBytes(32).toString("hex") + "\n", {
        mode: 0o600,
      });
    addresses.set(
      name,
      (await (await cosmosWallet(name)).getAccounts())[0].address,
    );
  }
  if (existsSync(join(ROOT, "initialized.json"))) return;
  const cli = (args: string[]) =>
    exec(AKASH, [...args, "--home", AKASH_HOME], {
      maxBuffer: 4 * 1024 * 1024,
    });
  const genesisPath = join(AKASH_HOME, "config/genesis.json");
  if (!existsSync(genesisPath))
    await cli(["genesis", "init", "akt-bridge-local", "--chain-id", CHAIN]);
  let validator: string;
  try {
    validator = (
      await cli([
        "keys",
        "show",
        "validator",
        "-a",
        "--keyring-backend",
        "test",
      ])
    ).stdout.trim();
  } catch {
    // CLI output includes a new development mnemonic; keep it out of logs.
    await cli([
      "keys",
      "add",
      "validator",
      "--keyring-backend",
      "test",
      "--output",
      "json",
    ]);
    validator = (
      await cli([
        "keys",
        "show",
        "validator",
        "-a",
        "--keyring-backend",
        "test",
      ])
    ).stdout.trim();
  }
  addresses.set("validator", validator);
  for (const [name, address] of addresses) {
    const genesis = object(await json(genesisPath));
    const bank = object(object(genesis.app_state).bank);
    assert.ok(Array.isArray(bank.balances));
    if (
      !bank.balances.some((value: unknown) => object(value).address === address)
    ) {
      const amount =
        name === "validator" || name === "sender"
          ? "100000000000"
          : "100000000";
      await cli(["genesis", "add-account", address, `${amount}uakt`]);
    }
  }
  const genesis = object(await json(genesisPath));
  const app = object(genesis.app_state);
  const sink = object(app.sink);
  sink.params = {
    ...object(sink.params),
    enabled: true,
    refunds_enabled: true,
    reporters: names
      .filter((name) => name.startsWith("reporter"))
      .map((name) => addresses.get(name)),
    reporter_threshold: 3,
    // x/sink requires a minimum while deposits are on; 1 uakt changes nothing here.
    min_transfer: "1",
    challenge_period_seconds: "20",
    // 1,000,000 AKT per transfer, 10,000,000 AKT minted on Solana and
    // 1,000,000 AKT refunded per day, matching the Solana mint policy.
    max_transfer: "1000000000000",
    solana_daily_mint_cap: "10000000000000",
    daily_refund_cap: "1000000000000",
  };
  app.sink = sink;
  const mint = object(app.mint);
  mint.minter = {
    ...object(mint.minter),
    inflation: "0.000000000000000000",
    annual_provisions: "0.000000000000000000",
  };
  mint.params = {
    ...object(mint.params),
    inflation_min: "0.000000000000000000",
    inflation_max: "0.000000000000000000",
  };
  app.mint = mint;
  genesis.app_state = app;
  await save(genesisPath, genesis);
  const gentxs = object(app.genutil).gen_txs;
  if (Array.isArray(gentxs) && gentxs.length === 0) {
    const gentxDir = join(AKASH_HOME, "config/gentx");
    if (
      !existsSync(gentxDir) ||
      !(await readdir(gentxDir)).some((name) => name.endsWith(".json"))
    )
      await cli([
        "genesis",
        "gentx",
        "validator",
        "10000000000uakt",
        "--chain-id",
        CHAIN,
        "--keyring-backend",
        "test",
        "--ip",
        "127.0.0.1",
        "--min-self-delegation",
        "1",
        "--commission-rate",
        "0.1",
        "--commission-max-rate",
        "0.2",
        "--commission-max-change-rate",
        "0.01",
      ]);
    await cli(["genesis", "collect"]);
  }
  await cli(["genesis", "validate"]);
  const configPath = join(AKASH_HOME, "config/config.toml");
  let config = await readFile(configPath, "utf8");
  config = config
    .replace(/^timeout_commit = .*$/m, 'timeout_commit = "1s"')
    .replace(/^timeout_propose = .*$/m, 'timeout_propose = "1s"');
  await writeFile(configPath, config);
  const sinkAddress = toBech32(
    "akash",
    createHash("sha256").update("sink").digest().subarray(0, 20),
  );
  await save(join(ROOT, "initialized.json"), {
    chainId: CHAIN,
    sinkAddress,
    addresses: Object.fromEntries(addresses),
  });
  console.log(
    `Initialized local development keys and funded sink genesis in ${ROOT}`,
  );
}

export async function up(signal?: AbortSignal) {
  await init();
  const wallet = await provider();
  const artifacts = join(REPO, ".deps/ibc-contracts/ibc-solana/target/deploy");
  if (PROFILE === "testnetify") {
    await (await import("./testnetify")).startNetwork();
  } else
    await start(
      "akash",
      AKASH,
      [
        "start",
        "--home",
        AKASH_HOME,
        "--rpc.laddr",
        `tcp://127.0.0.1:${AKASH_RPC_PORT}`,
        "--p2p.laddr",
        `tcp://127.0.0.1:${AKASH_RPC_PORT - 1}`,
        "--grpc.address",
        `127.0.0.1:${AKASH_GRPC_PORT}`,
        "--api.enable",
        "--api.address",
        `tcp://127.0.0.1:${AKASH_REST_PORT}`,
        "--minimum-gas-prices",
        "0.025uakt",
        "--pruning",
        "nothing",
        "--log_level",
        "warn",
      ],
      AKASH_HOME,
      [AKASH_RPC_PORT, AKASH_RPC_PORT - 1, AKASH_GRPC_PORT, AKASH_REST_PORT],
    );
  const ledger = join(ROOT, "solana");
  await start(
    "solana",
    "solana-test-validator",
    [
      "--ledger",
      ledger,
      "--bind-address",
      "127.0.0.1",
      "--rpc-port",
      String(SOLANA_RPC_PORT),
      ...(PROFILE === "testnetify"
        ? [
            "--gossip-port",
            "18901",
            "--dynamic-port-range",
            "18902-18950",
            "--faucet-port",
            "18900",
          ]
        : []),
      "--limit-ledger-size",
      ledgerShreds(),
      "--mint",
      web3.Keypair.generate().publicKey.toBase58(),
      "--bpf-program",
      "metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s",
      join(REPO, ".deps/metaplex/mpl_token_metadata.so"),
      "--upgradeable-program",
      CLIENT_ID.toBase58(),
      join(artifacts, "ics07_tendermint.so"),
      wallet.publicKey.toBase58(),
      "--upgradeable-program",
      ACCESS_ID.toBase58(),
      join(artifacts, "access_manager.so"),
      wallet.publicKey.toBase58(),
      "--upgradeable-program",
      PROGRAM_ID.toBase58(),
      join(REPO, "target/deploy/akt_solana.so"),
      wallet.publicKey.toBase58(),
      "--quiet",
    ],
    ledger,
    [SOLANA_RPC_PORT, SOLANA_RPC_PORT + 1],
  );
  const akash = new AkashClient(AKASH_RPC, CHAIN, 60);
  try {
    await ready("akash", () => akash.state());
  } finally {
    akash.close();
  }
  await ready("solana", async () => {
    assert.ok((await wallet.connection.getSlot("confirmed")) > 1);
  });
  if (
    (await wallet.connection.getBalance(wallet.publicKey)) <
    10 * web3.LAMPORTS_PER_SOL
  ) {
    const funding = await wallet.connection.requestAirdrop(
      wallet.publicKey,
      100 * web3.LAMPORTS_PER_SOL,
    );
    await wallet.connection.confirmTransaction(funding, "confirmed");
  }
  const marker = join(ROOT, "services");
  await mkdir(marker, { recursive: true });
  await exec(
    process.execPath,
    [
      "--import",
      "tsx",
      join(REPO, "local/relay.ts"),
      "--bootstrap",
      "--state",
      marker,
    ],
    { cwd: REPO, timeout: 600_000, maxBuffer: 2 * 1024 * 1024, signal },
  ).then((r) => console.log(r.stdout.trim()));
  if (PROFILE === "testnetify")
    await (await import("./testnetify")).activateValidators();
  await start(
    "relayer",
    process.execPath,
    [
      "--import",
      "tsx",
      join(REPO, "local/relay.ts"),
      "--watch",
      "--state",
      marker,
    ],
    marker,
  );
  const genesisHash = await wallet.connection.getGenesisHash();
  for (let i = 0; i < 3; i++) {
    const reporter = (
      await (await cosmosWallet(`reporter${i}`)).getAccounts()
    )[0].address;
    const config = join(ROOT, `reporter${i}.json`);
    await save(config, {
      akashRpc: AKASH_RPC,
      akashChainId: CHAIN,
      solanaRpc: SOLANA_RPC,
      solanaGenesisHash: genesisHash,
      programId: PROGRAM_ID.toBase58(),
      mint: pda("mint").toBase58(),
      // No lightClientProgram pin: reporters follow the bridge account.
      reporter,
      maxAgeSeconds: 60,
      maxTransfers: 1000,
      pollSeconds: 3,
      feeAmountUakt: "12500",
      gasLimit: "500000",
    });
    await start(
      `reporter${i}`,
      process.execPath,
      [
        "--import",
        "tsx",
        join(REPO, "reporter/run.ts"),
        "--config",
        config,
        "--watch",
        "--broadcast",
        "--key-file",
        join(ROOT, `reporter${i}.key`),
      ],
      config,
    );
  }
  await start(
    "reconciler",
    process.execPath,
    [
      "--import",
      "tsx",
      join(REPO, "local/reconcile.ts"),
      "--watch",
      "--state",
      marker,
    ],
    marker,
  );
  await status();
}
function ledgerShreds() {
  const value = process.env.AKT_SOLANA_LEDGER_SHREDS ?? "1000000";
  assert.ok(
    /^[1-9][0-9]*$/.test(value) && Number.isSafeInteger(Number(value)),
    "AKT_SOLANA_LEDGER_SHREDS must be a positive integer",
  );
  return value;
}
async function status() {
  for (const entry of await processes()) {
    if (
      entry.name === "testnetify" &&
      existsSync(join(ROOT, "testnetify-completed.json"))
    ) {
      console.log("testnetify: completed");
      continue;
    }
    console.log(
      `${entry.name}: ${(await running(entry)) ? "running" : "stopped"} (PID ${entry.pid})`,
    );
  }
  const client = new AkashClient(AKASH_RPC, CHAIN, 60);
  try {
    const state = await client.state();
    console.log(
      JSON.stringify(
        {
          akash: AKASH_RPC,
          height: state.height,
          totals: Object.fromEntries(
            Object.entries(state.totals).map(([key, value]) => [
              key,
              value.toString(),
            ]),
          ),
        },
        (_, value: unknown) =>
          typeof value === "bigint" ? value.toString() : value,
      ),
    );
  } finally {
    client.close();
  }
  const wallet = await provider();
  console.log(
    JSON.stringify({
      solana: SOLANA_RPC,
      slot: await wallet.connection.getSlot(),
      genesisHash: await wallet.connection.getGenesisHash(),
      mint: pda("mint").toBase58(),
    }),
  );
}
export async function stop(entry: ProcessEntry) {
  if (!(await running(entry))) return;
  process.kill(-entry.pid, "SIGTERM");
  console.log(`${entry.name} stopping`);
  const deadline = Date.now() + (entry.name === "relayer" ? 180_000 : 30_000);
  while (await running(entry)) {
    assert.ok(Date.now() < deadline, `${entry.name} did not stop`);
    await sleep(250);
  }
}
async function checkHeader() {
  const relayer = (await processes()).find((entry) => entry.name === "relayer");
  assert.ok(relayer, "start the local stack before checking headers");
  // The diagnostic and watcher write the same header-upload PDAs. Keep a
  // single writer and resume normal relaying even if the diagnostic fails.
  await stop(relayer);
  try {
    const result = await exec(
      process.execPath,
      [
        "--import",
        "tsx",
        join(REPO, "local/relay.ts"),
        "--check-invalid-header",
      ],
      // Two uploads, each waiting for finalized lookup, assembly and cleanup
      // transactions, took 150 s on a local validator.
      { cwd: REPO, timeout: 360_000, maxBuffer: 2 * 1024 * 1024 },
    );
    console.log(result.stdout.trim());
  } finally {
    await up();
  }
}
async function main() {
  switch (process.argv[2]) {
    case "init":
      await init();
      break;
    case "up":
      await up();
      break;
    case "status":
      await status();
      break;
    case "down":
      for (const entry of (await processes()).reverse()) await stop(entry);
      break;
    case "check-header":
      await checkHeader();
      break;
    default:
      throw new Error(
        "usage: tsx local/stack.ts init|up|status|down|check-header",
      );
  }
}
if (require.main === module)
  void main().catch((error: unknown) => {
    console.error(
      error instanceof Error ? error.message : "local stack failed",
    );
    process.exitCode = 1;
  });
