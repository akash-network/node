import assert from "node:assert/strict";
import { constants, open, readFile } from "node:fs/promises";
import { parseArgs } from "node:util";
import { setTimeout } from "node:timers/promises";
import { web3 } from "@coral-xyz/anchor";
import {
  DirectSecp256k1Wallet,
  type OfflineSigner,
} from "@cosmjs/proto-signing";
import Long from "long";
import { AkashClient } from "./akash";
import { parseConfig, signerCommand } from "./config";
import type { Message } from "./policy";
import { Scanner } from "./scan";
import { ExternalSigner } from "./signer";
import { SolanaObserver } from "./solana";

function output(value: unknown) {
  console.log(
    JSON.stringify(value, (_, item: unknown) => {
      if (typeof item === "bigint" || Long.isLong(item)) return item.toString();
      if (item instanceof Uint8Array)
        return Buffer.from(item).toString("base64");
      return item;
    }),
  );
}

async function loadSigner(path: string, expected: string) {
  const file = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW);
  try {
    const stat = await file.stat();
    assert.ok(
      stat.isFile() &&
        (stat.mode & 0o077) === 0 &&
        stat.uid === process.getuid?.(),
      "signing key must be an owner-only regular file",
    );
    const raw = (await file.readFile("utf8")).trim();
    assert.ok(
      /^[0-9a-f]{64}$/.test(raw),
      "key file must contain a 32-byte lowercase hex secp256k1 key",
    );
    const scalar = BigInt(`0x${raw}`);
    assert.ok(
      scalar > 0n &&
        scalar <
          0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141n,
      "invalid signing key scalar",
    );
    const key = Buffer.from(raw, "hex");
    const wallet = await DirectSecp256k1Wallet.fromKey(key, "akash");
    // The wallet retains the key for signing; never write it to output.
    assert.equal(
      (await wallet.getAccounts())[0]?.address,
      expected,
      "signer does not match reporter",
    );
    return wallet;
  } finally {
    await file.close();
  }
}

async function main() {
  const { values } = parseArgs({
    options: {
      config: { type: "string" },
      broadcast: { type: "boolean", default: false },
      "key-file": { type: "string" },
      "signer-command": { type: "string" },
      watch: { type: "boolean", default: false },
    },
  });
  assert.ok(
    values.config,
    "usage: npm run reporter -- --config PATH [--watch] [--broadcast (--key-file PATH | --signer-command PATH)]",
  );
  const keyFile = values["key-file"];
  assert.ok(
    values.broadcast || (!keyFile && !values["signer-command"]),
    "signing requires --broadcast; preview never loads keys or runs signers",
  );
  const raw: unknown = JSON.parse(await readFile(values.config, "utf8"));
  const config = parseConfig(raw);
  // --signer-command replaces the configured command and its arguments.
  const external = values["signer-command"]
    ? {
        timeoutSeconds: 10,
        ...config.externalSigner,
        command: signerCommand(values["signer-command"]),
        args: [],
      }
    : config.externalSigner;
  if (values.broadcast) {
    assert.ok(
      Boolean(keyFile) !== Boolean(external),
      "broadcast requires exactly one of --key-file or an external signer",
    );
  }
  const akash = new AkashClient(
    config.akashRpc,
    config.akashChainId,
    config.maxAgeSeconds,
  );
  const connection = new web3.Connection(config.solanaRpc, {
    commitment: "finalized",
    disableRetryOnRateLimit: true,
    fetch: (url, init) =>
      fetch(url, { ...init, signal: AbortSignal.timeout(15_000) }),
  });
  const observer = new SolanaObserver(connection, config);
  let submit: ((message: Message) => Promise<string>) | undefined;
  if (values.broadcast) {
    let signer: OfflineSigner;
    if (keyFile) signer = await loadSigner(keyFile, config.reporter);
    else {
      assert.ok(external);
      const publicKey = external.publicKeyBase64
        ? Buffer.from(external.publicKeyBase64, "base64")
        : await akash.accountPublicKey(config.reporter);
      assert.ok(
        publicKey,
        "reporter account has no public key on chain; set externalSigner.publicKeyBase64",
      );
      signer = new ExternalSigner({
        command: external.command,
        args: external.args,
        timeoutMs: external.timeoutSeconds * 1000,
        address: config.reporter,
        chainId: config.akashChainId,
        publicKey,
      });
    }
    submit = akash.broadcaster(signer, config.reporter, {
      amount: config.feeAmountUakt,
      gas: config.gasLimit,
    });
  }
  const abort = new AbortController();
  const stop = () => abort.abort();
  process.once("SIGINT", stop);
  process.once("SIGTERM", stop);
  const scanner = new Scanner(akash, observer, {
    reporter: config.reporter,
    maxTransfers: config.maxTransfers,
    auditPerScan: config.auditPerScan,
    output,
    signal: abort.signal,
    submit,
  });
  try {
    do {
      const failures = await scanner.scan();
      if (!values.watch && failures) process.exitCode = 1;
      if (values.watch && !abort.signal.aborted) {
        await setTimeout(config.pollSeconds * 1000, undefined, {
          signal: abort.signal,
        }).catch((error: unknown) => {
          if (!abort.signal.aborted) throw error;
        });
      }
    } while (values.watch && !abort.signal.aborted);
  } finally {
    akash.close();
    process.removeListener("SIGINT", stop);
    process.removeListener("SIGTERM", stop);
  }
}

void main().catch((error: unknown) => {
  console.error(error instanceof Error ? error.message : "reporter failed");
  process.exitCode = 1;
});
