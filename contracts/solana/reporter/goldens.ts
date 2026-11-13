// Development only: writes the parity goldens of the Go reporter (operator/reporter/testdata). For each simnode
// scenario it runs this TS reporter in preview against `simnode serve` and records every event it writes. It also
// writes PDA vectors derived by the TS helpers. Run from contracts/solana: npx tsx reporter/goldens.ts
import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { once } from "node:events";
import { mkdir, writeFile } from "node:fs/promises";
import { resolve } from "node:path";
import { createInterface } from "node:readline";
import { web3 } from "@coral-xyz/anchor";
import { toBech32 } from "@cosmjs/encoding";
import Long from "long";
import { AkashClient } from "./akash";
import { Scanner } from "./scan";
import { SolanaObserver } from "./solana";
import { statusAddress } from "../shared/proof";
import { OWNER_BECH32_LIMIT, parseSinkId } from "../shared/id";

const root = resolve(__dirname, "../../..");
const testdata = resolve(root, "operator/reporter/testdata");
const simnode = ["run", "./operator/reporter/internal/simnode/cmd/simnode"];
const env = { ...process.env, GOWORK: "off" };
// The development program ID of the local network.
const devProgram = "FfFAPUKqjf68ggj1HvpyzMHFDRnT2cFAdZYYQb7mK9i5";

// The line `simnode serve` prints once it listens.
interface Ready {
  comet: string;
  grpc: string;
  solana: string;
  passes: number;
  chainId: string;
  reporter: string;
  genesisHash: string;
  programId: string;
  mint: string;
  maxAgeSeconds: number;
  lightClientPin?: string;
}

// The event encoding of run.ts.
function encode(value: unknown) {
  return JSON.stringify(value, (_, item: unknown) => {
    if (typeof item === "bigint" || Long.isLong(item)) return item.toString();
    if (item instanceof Uint8Array) return Buffer.from(item).toString("base64");
    return item;
  });
}

async function serve(scenario: string, run: (ready: Ready) => Promise<void>) {
  const child = spawn("go", [...simnode, "serve", "--scenario", scenario], {
    cwd: root,
    env,
    stdio: ["pipe", "pipe", "inherit"],
  });
  const exited = once(child, "exit");
  try {
    const lines = createInterface({ input: child.stdout });
    const [first] = (await once(lines, "line")) as [string];
    lines.close();
    await run(JSON.parse(first) as Ready);
  } finally {
    child.stdin.end();
    const [code] = (await exited) as [number | null];
    assert.equal(code, 0, `simnode serve --scenario ${scenario} failed`);
  }
}

async function golden(scenario: string) {
  const events: string[] = [];
  let programId = "";
  await serve(scenario, async (ready) => {
    programId = ready.programId;
    const akash = new AkashClient(
      ready.comet,
      ready.chainId,
      ready.maxAgeSeconds,
    );
    const connection = new web3.Connection(ready.solana, {
      commitment: "finalized",
      disableRetryOnRateLimit: true,
      fetch: (url, init) =>
        fetch(url, { ...init, signal: AbortSignal.timeout(15_000) }),
    });
    const observer = new SolanaObserver(connection, {
      genesisHash: ready.genesisHash,
      programId: new web3.PublicKey(ready.programId),
      mint: new web3.PublicKey(ready.mint),
      lightClientProgram: ready.lightClientPin
        ? new web3.PublicKey(ready.lightClientPin)
        : undefined,
      akashChainId: ready.chainId,
      maxAgeSeconds: ready.maxAgeSeconds,
    });
    const scanner = new Scanner(akash, observer, {
      reporter: ready.reporter,
      auditPerScan: 100,
      output: (value) => events.push(encode(value)),
      signal: new AbortController().signal,
    });
    try {
      for (let pass = 0; pass < ready.passes; pass++) await scanner.scan();
    } finally {
      akash.close();
    }
  });
  await writeFile(
    resolve(testdata, "goldens", `${scenario}.jsonl`),
    events.map((line) => `${line}\n`).join(""),
  );
  console.log(`${scenario}: ${events.length} events`);
  return programId;
}

function owner(length: number, fill: (i: number) => number) {
  return toBech32(
    "akash",
    Uint8Array.from({ length }, (_, i) => fill(i)),
    OWNER_BECH32_LIMIT,
  );
}

function pdaVectors(programs: string[]) {
  const ids = [
    `${owner(1, () => 1)}/1/1`,
    `${owner(20, (i) => i)}/9007199254740993/18446744073709551615`,
    `${owner(32, (i) => 255 - i)}/9223372036854775807/1`,
    `${owner(255, (i) => (i * 7) % 256)}/2147483648/4294967296`,
  ].map((id) => parseSinkId(id));
  const status = programs.flatMap((programId) =>
    ids.map((id) => ({
      programId,
      id,
      address: statusAddress(new web3.PublicKey(programId), id).toBase58(),
    })),
  );
  const seeds = programs.flatMap((programId) =>
    ["config", "bridge", "mint", "mint_authority"].map((seed) => ({
      programId,
      seed,
      address: web3.PublicKey.findProgramAddressSync(
        [Buffer.from(seed)],
        new web3.PublicKey(programId),
      )[0].toBase58(),
    })),
  );
  return { status, seeds };
}

// Arguments name the scenarios to regenerate; none regenerates all.
async function main(names = process.argv.slice(2)) {
  const all = execFileSync("go", [...simnode, "list"], {
    cwd: root,
    env,
    encoding: "utf8",
  })
    .split("\n")
    .filter(Boolean);
  for (const name of names)
    assert.ok(all.includes(name), `unknown scenario ${name}`);
  const scenarios = names.length > 0 ? names : all;
  await mkdir(resolve(testdata, "goldens"), { recursive: true });
  const programs = new Set([devProgram]);
  for (const scenario of scenarios) programs.add(await golden(scenario));
  await writeFile(
    resolve(testdata, "pda_vectors.json"),
    `${JSON.stringify(pdaVectors([...programs]), null, 2)}\n`,
  );
}

void main().catch((error: unknown) => {
  console.error(error);
  process.exitCode = 1;
});
