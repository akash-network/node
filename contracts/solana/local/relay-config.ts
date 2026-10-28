import assert from "node:assert/strict";
import { constants, open } from "node:fs/promises";
import { isAbsolute, join, resolve } from "node:path";
import { web3 } from "@coral-xyz/anchor";
import { AKASH_RPC, CHAIN, PROGRAM_ID, REPO, ROOT, SOLANA_RPC } from "./common";
import {
  DEFAULT_MAX_HEADER_SIGNATURES,
  DEFAULT_MAX_LAMPORTS_PER_TRANSFER,
  DEFAULT_REFRESH_FRACTION,
  relaySettings,
} from "./relay-policy";

export interface RelayConfig {
  akashRpc: string; // CometBFT RPC with proofs and historical state
  akashChainId: string;
  solanaRpc: string;
  // Unset only for the local stack, which trusts the validator it started.
  solanaGenesisHash?: string;
  programId: web3.PublicKey; // AKT program
  payerKeyFile: string;
  stateDir: string; // header journals and the spend ledger
  // A pin: a bridge naming another light client stops the relayer.
  lightClientProgram?: web3.PublicKey;
  maxLamportsPerTransfer: number;
  // Commits with more signatures submit only a verifying subset.
  maxHeaderSignatures: number;
  refreshFraction: number;
  retryMaxMs: number;
  pruneIntervalMs: number;
  maxAgeSeconds: number; // oldest acceptable Akash block
  pollMs: number;
  headerEncoder: string;
}

export const DEFAULT_HEADER_ENCODER = join(
  REPO,
  "tools/header-encoder/target/debug/akt-header-encoder",
);

const FIELDS = new Set([
  "akashRpc",
  "akashChainId",
  "solanaRpc",
  "solanaGenesisHash",
  "programId",
  "payerKeyFile",
  "stateDir",
  "lightClientProgram",
  "maxLamportsPerTransfer",
  "maxHeaderSignatures",
  "refreshFraction",
  "retryMaxSeconds",
  "pruneIntervalSeconds",
  "maxAgeSeconds",
  "pollSeconds",
  "headerEncoder",
]);

// A relayer configuration file. Relative paths resolve against baseDir, the
// file's directory. Unknown fields fail so a misspelt limit is not ignored.
export function parseRelayConfig(value: unknown, baseDir: string): RelayConfig {
  assert.ok(
    typeof value === "object" && value !== null && !Array.isArray(value),
    "config must be an object",
  );
  const fields = value;
  for (const name of Object.keys(fields))
    assert.ok(FIELDS.has(name), `unknown config field ${name}`);
  const has = (name: string) => name in fields;
  function field(name: string): unknown {
    assert.ok(has(name), `missing config field ${name}`);
    return Reflect.get(fields, name);
  }
  function string(name: string) {
    const result = field(name);
    assert.ok(
      typeof result === "string" && result.length > 0,
      `invalid ${name}`,
    );
    return result;
  }
  function integer(name: string, min: number, max: number, fallback: number) {
    if (!has(name)) return fallback;
    const result = field(name);
    assert.ok(
      typeof result === "number" &&
        Number.isSafeInteger(result) &&
        result >= min &&
        result <= max,
      `invalid ${name}`,
    );
    return result;
  }
  function url(name: string) {
    const result = string(name);
    const parsed = new URL(result);
    assert.ok(
      ["http:", "https:"].includes(parsed.protocol),
      `invalid ${name} protocol`,
    );
    assert.ok(
      !parsed.username && !parsed.password,
      "use a local authenticated proxy instead of credentials in RPC URLs",
    );
    return result;
  }
  function pubkey(name: string) {
    const result = string(name);
    const key = new web3.PublicKey(result);
    assert.equal(key.toBase58(), result, `noncanonical ${name}`);
    return key;
  }
  const path = (name: string) => resolve(baseDir, string(name));
  const akashChainId = string("akashChainId");
  assert.ok(akashChainId.length <= 64, "invalid akashChainId");
  const solanaGenesisHash = string("solanaGenesisHash");
  assert.equal(
    new web3.PublicKey(solanaGenesisHash).toBase58(),
    solanaGenesisHash,
    "invalid solanaGenesisHash",
  );
  let refreshFraction = DEFAULT_REFRESH_FRACTION;
  if (has("refreshFraction")) {
    const result = field("refreshFraction");
    assert.ok(
      typeof result === "number" && result > 0 && result < 1,
      "invalid refreshFraction",
    );
    refreshFraction = result;
  }
  return {
    akashRpc: url("akashRpc"),
    akashChainId,
    solanaRpc: url("solanaRpc"),
    solanaGenesisHash,
    programId: pubkey("programId"),
    payerKeyFile: path("payerKeyFile"),
    stateDir: path("stateDir"),
    lightClientProgram: has("lightClientProgram")
      ? pubkey("lightClientProgram")
      : undefined,
    maxLamportsPerTransfer: integer(
      "maxLamportsPerTransfer",
      1,
      1_000_000_000,
      DEFAULT_MAX_LAMPORTS_PER_TRANSFER,
    ),
    maxHeaderSignatures: integer(
      "maxHeaderSignatures",
      1,
      128,
      DEFAULT_MAX_HEADER_SIGNATURES,
    ),
    refreshFraction,
    retryMaxMs: integer("retryMaxSeconds", 6, 86_400, 900) * 1000,
    pruneIntervalMs: integer("pruneIntervalSeconds", 60, 86_400, 600) * 1000,
    maxAgeSeconds: integer("maxAgeSeconds", 10, 600, 60),
    pollMs: integer("pollSeconds", 1, 600, 3) * 1000,
    headerEncoder: has("headerEncoder")
      ? path("headerEncoder")
      : DEFAULT_HEADER_ENCODER,
  };
}

// The team and development stacks: the same settings derived from
// local/common.ts and the AKT_RELAY_* variables, with no file to maintain.
export function localRelayConfig(
  env: Record<string, string | undefined>,
): RelayConfig {
  return {
    akashRpc: AKASH_RPC,
    akashChainId: CHAIN,
    solanaRpc: SOLANA_RPC,
    programId: PROGRAM_ID,
    payerKeyFile: join(ROOT, "solana-keypair.json"),
    stateDir: ROOT,
    ...relaySettings(env),
    maxAgeSeconds: 60,
    pollMs: 3000,
    headerEncoder: DEFAULT_HEADER_ENCODER,
  };
}

// A Solana CLI keypair file readable only by its owner.
export async function loadPayer(path: string) {
  assert.ok(isAbsolute(path), "payer key path must be absolute");
  const file = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW);
  try {
    const stat = await file.stat();
    assert.ok(
      stat.isFile() &&
        (stat.mode & 0o077) === 0 &&
        stat.uid === process.getuid?.(),
      "payer key must be an owner-only regular file",
    );
    const raw: unknown = JSON.parse(await file.readFile("utf8"));
    assert.ok(
      Array.isArray(raw) &&
        raw.length === 64 &&
        raw.every(
          (v: unknown) =>
            typeof v === "number" && Number.isInteger(v) && v >= 0 && v < 256,
        ),
      "payer key must be a 64-byte Solana keypair array",
    );
    // fromSecretKey checks that the public half matches the secret.
    return web3.Keypair.fromSecretKey(Uint8Array.from(raw));
  } finally {
    await file.close();
  }
}
