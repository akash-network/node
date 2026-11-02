import assert from "node:assert/strict";
import { readFile, writeFile } from "node:fs/promises";
import { resolve, join } from "node:path";
import { AnchorProvider, Wallet, web3 } from "@coral-xyz/anchor";
import { DirectSecp256k1Wallet } from "@cosmjs/proto-signing";
import { Comet38Client, HttpClient } from "@cosmjs/tendermint-rpc";
import { SigningStargateClient } from "@cosmjs/stargate";
import { reporterRegistry } from "../reporter/akash";

export const REPO = resolve(__dirname, "..");
export const PROFILE = process.env.AKT_LOCAL_PROFILE ?? "single";
assert.ok(
  PROFILE === "single" || PROFILE === "testnetify",
  "unknown local profile",
);
const run = process.env.AKT_LOCAL_RUN ?? "";
assert.ok(
  run === "" || /^[a-z0-9][a-z0-9-]{0,31}$/.test(run),
  "invalid isolated run name",
);
export const ROOT = join(
  REPO,
  (PROFILE === "single" ? ".cache/live-stack" : ".cache/testnetify-stack") +
    (run ? `-${run}` : ""),
);
export const AKASH_HOME = join(ROOT, "akash");
export const NODE_REPO = resolve(REPO, process.env.NODE_SINK_DIR ?? "../..");
export const AKASH = join(NODE_REPO, ".cache/bin/akash");
export const CHAIN =
  (PROFILE === "single" ? "akash-bridge-local" : "akash-bridge-testnetify") +
  (run ? `-${run}` : "") +
  "-1";
export const AKASH_RPC_PORT = PROFILE === "single" ? 26657 : 27657;
export const AKASH_GRPC_PORT = PROFILE === "single" ? 9090 : 9190;
export const AKASH_REST_PORT = PROFILE === "single" ? 1318 : 1317;
export const SOLANA_RPC_PORT = Number(
  process.env.AKT_SOLANA_RPC_PORT ?? (PROFILE === "single" ? 8899 : 18999),
);
assert.ok(
  Number.isInteger(SOLANA_RPC_PORT) &&
    SOLANA_RPC_PORT > 1024 &&
    SOLANA_RPC_PORT < 65535,
  "AKT_SOLANA_RPC_PORT must be an unprivileged port with room for WebSocket",
);
export const AKASH_RPC = `http://127.0.0.1:${AKASH_RPC_PORT}`;
export const SOLANA_RPC = `http://127.0.0.1:${SOLANA_RPC_PORT}`;
export const CLIENT_ID = new web3.PublicKey(
  "HqPcGpVHxNNFfVatjhG78dFVMwjyZixoKPdZSt3d3TdD",
);
export const ACCESS_ID = new web3.PublicKey(
  "4fMih2CidrXPeRx77kj3QcuBZwREYtxEbXjURUgadoe1",
);
export const PROGRAM_ID = new web3.PublicKey(
  "FfFAPUKqjf68ggj1HvpyzMHFDRnT2cFAdZYYQb7mK9i5",
);
export const LE64 = (value: bigint) => {
  const bytes = Buffer.alloc(8);
  bytes.writeBigUInt64LE(value);
  return bytes;
};
export const pda = (seed: string, program = PROGRAM_ID, suffix?: Buffer) =>
  web3.PublicKey.findProgramAddressSync(
    suffix ? [Buffer.from(seed), suffix] : [Buffer.from(seed)],
    program,
  )[0];
export const programData = (program: web3.PublicKey) =>
  web3.PublicKey.findProgramAddressSync(
    [program.toBuffer()],
    new web3.PublicKey("BPFLoaderUpgradeab1e11111111111111111111111"),
  )[0];
export const sleep = (ms: number) =>
  new Promise<void>((resolve) => setTimeout(resolve, ms));
export function object(value: unknown): Record<string, unknown> {
  assert.ok(
    typeof value === "object" && value !== null && !Array.isArray(value),
    "JSON object required",
  );
  return Object.fromEntries(Object.entries(value));
}
export async function json(path: string): Promise<unknown> {
  return JSON.parse(await readFile(path, "utf8"));
}
export async function save(path: string, value: unknown) {
  await writeFile(path, JSON.stringify(value, null, 2) + "\n", { mode: 0o600 });
}
export function hex(value: unknown, length?: number) {
  assert.ok(
    typeof value === "string" && /^(?:[0-9a-fA-F]{2})+$/.test(value),
    "hex bytes required",
  );
  const raw = Buffer.from(value, "hex");
  if (length !== undefined) assert.equal(raw.length, length);
  return raw;
}
export async function provider() {
  const raw = await json(join(ROOT, "solana-keypair.json"));
  assert.ok(
    Array.isArray(raw) &&
      raw.length === 64 &&
      raw.every(
        (v: unknown) =>
          typeof v === "number" && Number.isInteger(v) && v >= 0 && v < 256,
      ),
  );
  const key = web3.Keypair.fromSecretKey(Uint8Array.from(raw));
  return new AnchorProvider(
    new web3.Connection(SOLANA_RPC, "confirmed"),
    new Wallet(key),
    { commitment: "confirmed", preflightCommitment: "confirmed" },
  );
}
export async function cosmosWallet(name: string) {
  const raw = (await readFile(join(ROOT, `${name}.key`), "utf8")).trim();
  return DirectSecp256k1Wallet.fromKey(hex(raw, 32), "akash");
}
export async function cosmosSigner(name: string) {
  const wallet = await cosmosWallet(name);
  const address = (await wallet.getAccounts())[0].address;
  const comet = Comet38Client.create(new HttpClient(AKASH_RPC, 15_000));
  assert.equal(
    (await comet.status()).nodeInfo.network,
    CHAIN,
    "wrong local Akash chain",
  );
  const client = SigningStargateClient.createWithSigner(comet, wallet, {
    registry: reporterRegistry(),
    broadcastTimeoutMs: 30_000,
    broadcastPollIntervalMs: 500,
  });
  return { client, address };
}
export const fee = {
  amount: [{ denom: "uakt", amount: "12500" }],
  gas: "500000",
};
