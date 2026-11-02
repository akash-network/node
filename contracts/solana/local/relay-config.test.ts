import assert from "node:assert/strict";
import { test } from "node:test";
import { chmod, mkdtemp, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { web3 } from "@coral-xyz/anchor";
import example from "../relayer/config.example.json";
import {
  DEFAULT_HEADER_ENCODER,
  loadPayer,
  localRelayConfig,
  parseRelayConfig,
} from "./relay-config";
import { AKASH_RPC, CHAIN, PROGRAM_ID, ROOT, SOLANA_RPC } from "./common";

const genesis = web3.Keypair.generate().publicKey.toBase58();
const base = { ...example, solanaGenesisHash: genesis };

test("the example config parses once its genesis hash is filled in", () => {
  assert.throws(
    () => parseRelayConfig(example, "/srv"),
    /invalid|Non-base58|Invalid public key/,
  );
  const config = parseRelayConfig(base, "/srv");
  assert.equal(config.solanaGenesisHash, genesis);
  assert.ok(config.programId.equals(PROGRAM_ID));
  assert.equal(config.payerKeyFile, "/etc/akt-relayer/payer-keypair.json");
  assert.equal(config.stateDir, "/var/lib/akt-relayer");
  assert.equal(config.lightClientProgram, undefined);
  assert.equal(config.maxLamportsPerTransfer, 5_000_000);
  assert.equal(config.retryMaxMs, 900_000);
  assert.equal(config.pruneIntervalMs, 600_000);
  assert.equal(config.pollMs, 3000);
  assert.equal(config.headerEncoder, DEFAULT_HEADER_ENCODER);
});

test("optional settings default and relative paths follow the config", () => {
  const minimal = Object.fromEntries(
    Object.entries(base).filter(
      ([key]) =>
        ![
          "maxLamportsPerTransfer",
          "refreshFraction",
          "retryMaxSeconds",
          "pruneIntervalSeconds",
          "maxAgeSeconds",
          "pollSeconds",
        ].includes(key),
    ),
  );
  const config = parseRelayConfig(
    {
      ...minimal,
      payerKeyFile: "keys/payer.json",
      stateDir: "state",
      headerEncoder: "../bin/encoder",
    },
    "/srv/relayer",
  );
  assert.equal(config.payerKeyFile, "/srv/relayer/keys/payer.json");
  assert.equal(config.stateDir, "/srv/relayer/state");
  assert.equal(config.headerEncoder, "/srv/bin/encoder");
  assert.equal(config.maxLamportsPerTransfer, 5_000_000);
  assert.equal(config.maxHeaderSignatures, 100);
  assert.equal(config.refreshFraction, 1 / 5);
  assert.equal(config.maxAgeSeconds, 60);
});

test("a light-client pin is optional and must be canonical", () => {
  const pin = web3.Keypair.generate().publicKey;
  assert.ok(
    parseRelayConfig(
      { ...base, lightClientProgram: pin.toBase58() },
      "/",
    ).lightClientProgram?.equals(pin),
  );
  assert.throws(() =>
    parseRelayConfig({ ...base, lightClientProgram: "not-a-key" }, "/"),
  );
});

test("config rejects unknown, missing and out-of-range fields", () => {
  assert.throws(
    () => parseRelayConfig({ ...base, maxLamportPerTransfer: 1 }, "/"),
    /unknown config field maxLamportPerTransfer/,
  );
  const { payerKeyFile: _, ...noPayer } = base;
  assert.throws(
    () => parseRelayConfig(noPayer, "/"),
    /missing config field payerKeyFile/,
  );
  for (const [field, value] of [
    ["akashRpc", "ftp://node"],
    ["solanaRpc", "https://user:secret@rpc.example"],
    ["akashChainId", ""],
    ["akashChainId", "a".repeat(65)],
    ["maxLamportsPerTransfer", 0],
    ["maxLamportsPerTransfer", 1.5],
    ["maxLamportsPerTransfer", "5000000"],
    ["maxHeaderSignatures", 0],
    ["maxHeaderSignatures", 129],
    ["refreshFraction", 1],
    ["refreshFraction", 0],
    ["retryMaxSeconds", 5],
    ["pruneIntervalSeconds", 59],
    ["maxAgeSeconds", 601],
    ["pollSeconds", 0],
  ] as const)
    assert.throws(
      () => parseRelayConfig({ ...base, [field]: value }, "/"),
      `${field}=${String(value)} accepted`,
    );
  assert.throws(() => parseRelayConfig([], "/"));
});

test("the local stack derives the same settings without a file", () => {
  const config = localRelayConfig({ AKT_RELAY_REFRESH_FRACTION: "0.25" });
  assert.equal(config.akashRpc, AKASH_RPC);
  assert.equal(config.akashChainId, CHAIN);
  assert.equal(config.solanaRpc, SOLANA_RPC);
  assert.equal(config.solanaGenesisHash, undefined);
  assert.equal(config.lightClientProgram, undefined);
  assert.ok(config.programId.equals(PROGRAM_ID));
  assert.equal(config.stateDir, ROOT);
  assert.equal(config.payerKeyFile, join(ROOT, "solana-keypair.json"));
  assert.equal(config.refreshFraction, 0.25);
  assert.equal(config.maxLamportsPerTransfer, 5_000_000);
  assert.equal(config.pollMs, 3000);
});

test("the payer key must be an owner-only keypair file", async () => {
  const directory = await mkdtemp(join(tmpdir(), "akt-relay-config-"));
  try {
    const key = web3.Keypair.generate();
    const path = join(directory, "payer.json");
    await writeFile(path, JSON.stringify(Array.from(key.secretKey)), {
      mode: 0o600,
    });
    assert.ok((await loadPayer(path)).publicKey.equals(key.publicKey));
    await assert.rejects(() => loadPayer("payer.json"), /absolute/);
    await chmod(path, 0o640);
    await assert.rejects(() => loadPayer(path), /owner-only/);
    await chmod(path, 0o600);
    const link = join(directory, "link.json");
    await symlink(path, link);
    await assert.rejects(() => loadPayer(link));
    const short = join(directory, "short.json");
    await writeFile(short, JSON.stringify([1, 2, 3]), { mode: 0o600 });
    await assert.rejects(() => loadPayer(short), /64-byte/);
    const mismatched = Array.from(key.secretKey);
    mismatched[40] ^= 1;
    const bad = join(directory, "bad.json");
    await writeFile(bad, JSON.stringify(mismatched), { mode: 0o600 });
    await assert.rejects(() => loadPayer(bad));
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});
