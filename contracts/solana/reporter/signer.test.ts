import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { secp256k1 } from "@noble/curves/secp256k1";
import {
  makeAuthInfoBytes,
  makeSignBytes,
  makeSignDoc,
} from "@cosmjs/proto-signing";
import { SigningStargateClient } from "@cosmjs/stargate";
import { PubKey } from "cosmjs-types/cosmos/crypto/secp256k1/keys";
import { TxRaw } from "cosmjs-types/cosmos/tx/v1beta1/tx";
import {
  MsgVetoVoid,
  Sink_Params as Params,
  Totals,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { AkashClient, reporterRegistry } from "./akash";
import { parseExternalSigner } from "./config";
import { ExternalSigner, akashAddress } from "./signer";

const FAKE = join(__dirname, "testdata", "fake-signer.mjs");
// Throwaway test key; the fake signer reads it from the environment.
const secret = secp256k1.utils.randomPrivateKey();
const publicKey = secp256k1.getPublicKey(secret, true);
const address = akashAddress(publicKey);
process.env.FAKE_SIGNER_KEY = Buffer.from(secret).toString("hex");

const fee = { amount: [{ denom: "uakt", amount: "10" }], gas: "500000" };
const message = {
  typeUrl: "/akash.sink.v1.MsgVetoVoid" as const,
  value: MsgVetoVoid.fromPartial({ signer: address, id: 1n, reason: "test" }),
};

function signer(mode: string, timeoutMs = 10_000, key = publicKey) {
  return new ExternalSigner({
    command: process.execPath,
    args: [FAKE, mode],
    timeoutMs,
    address,
    chainId: "akash-1",
    publicKey: key,
  });
}

async function sign(mode: string, timeoutMs?: number) {
  const client = await SigningStargateClient.offline(signer(mode, timeoutMs), {
    registry: reporterRegistry(),
  });
  return client.sign(address, [message], fee, "AKT sink reporter", {
    accountNumber: 7n,
    sequence: 3,
    chainId: "akash-1",
  });
}

function verifies(tx: TxRaw) {
  const bytes = makeSignBytes(
    makeSignDoc(tx.bodyBytes, tx.authInfoBytes, "akash-1", 7),
  );
  const signature = tx.signatures[0];
  assert.ok(signature);
  return secp256k1.verify(
    signature,
    createHash("sha256").update(bytes).digest(),
    publicKey,
    { prehash: false, lowS: true },
  );
}

test("external signer receives the sign doc and returns a verified signature", async (t) => {
  const directory = await mkdtemp(join(tmpdir(), "akt-signer-test-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  process.env.FAKE_SIGNER_LOG = join(directory, "requests.jsonl");
  t.after(() => delete process.env.FAKE_SIGNER_LOG);
  for (const mode of ["ok", "high-s"]) {
    const tx = await sign(mode);
    assert.ok(verifies(tx), mode);
    const signature = tx.signatures[0];
    assert.ok(signature);
    assert.ok(!secp256k1.Signature.fromCompact(signature).hasHighS());
  }
  const requests = (await readFile(process.env.FAKE_SIGNER_LOG, "utf8"))
    .trim()
    .split("\n")
    .map((line) => JSON.parse(line) as Record<string, string>);
  assert.equal(requests.length, 2);
  const tx = await sign("ok");
  assert.deepEqual(requests[0], {
    chainId: "akash-1",
    accountNumber: "7",
    sequence: "3",
    address,
    signDocBase64: Buffer.from(
      makeSignBytes(makeSignDoc(tx.bodyBytes, tx.authInfoBytes, "akash-1", 7)),
    ).toString("base64"),
  });
});

test("external signer failures never yield a signature", async () => {
  const cases: [string, RegExp, number?][] = [
    ["other-key", /does not match reporter/],
    ["wrong-doc", /does not verify over the sign doc/],
    ["uncompressed", /33-byte compressed/],
    ["short", /64-byte r\|\|s/],
    ["garbage", /invalid JSON/],
    ["huge", /too large/],
    ["fail", /exited with 3/],
    ["hang", /timed out/, 300],
  ];
  for (const [mode, error, timeoutMs] of cases)
    await assert.rejects(() => sign(mode, timeoutMs), error, mode);
  const missing = new ExternalSigner({
    command: "/nonexistent/akt-signer",
    args: [],
    timeoutMs: 1000,
    address,
    chainId: "akash-1",
    publicKey,
  });
  const client = await SigningStargateClient.offline(missing, {
    registry: reporterRegistry(),
  });
  await assert.rejects(
    () =>
      client.sign(address, [message], fee, "", {
        accountNumber: 7n,
        sequence: 3,
        chainId: "akash-1",
      }),
    /did not run/,
  );
});

test("external signer refuses keys, accounts and chains it was not configured for", async () => {
  const other = secp256k1.getPublicKey(
    secp256k1.utils.randomPrivateKey(),
    true,
  );
  assert.throws(() => signer("ok", 1000, other), /does not match reporter/);
  assert.throws(
    () => signer("ok", 1000, secp256k1.getPublicKey(secret, false)),
    /33-byte compressed/,
  );
  const tx = await sign("ok");
  const doc = (chainId: string) =>
    makeSignDoc(tx.bodyBytes, tx.authInfoBytes, chainId, 7);
  await assert.rejects(
    () => signer("ok").signDirect(akashAddress(other), doc("akash-1")),
    /another account/,
  );
  await assert.rejects(
    () => signer("ok").signDirect(address, doc("other-chain")),
    /another chain/,
  );
  // A signer that answers with the right key for another reporter.
  const foreign = await SigningStargateClient.offline(
    new ExternalSigner({
      command: process.execPath,
      args: [FAKE, "ok"],
      timeoutMs: 10_000,
      address: akashAddress(other),
      chainId: "akash-1",
      publicKey: other,
    }),
    { registry: reporterRegistry() },
  );
  await assert.rejects(
    () =>
      foreign.sign(akashAddress(other), [message], fee, "", {
        accountNumber: 7n,
        sequence: 3,
        chainId: "akash-1",
      }),
    /does not match reporter/,
  );
  // A sign doc whose auth info names another key is never sent to the signer.
  const authInfo = makeAuthInfoBytes(
    [
      {
        pubkey: {
          typeUrl: "/cosmos.crypto.secp256k1.PubKey",
          value: PubKey.encode({ key: other }).finish(),
        },
        sequence: 3,
      },
    ],
    fee.amount,
    500_000,
    undefined,
    undefined,
  );
  await assert.rejects(
    () =>
      signer("ok").signDirect(
        address,
        makeSignDoc(tx.bodyBytes, authInfo, "akash-1", 7),
      ),
    /does not name this signer/,
  );
});

test("the broadcaster sends nothing unless the external signature verifies", async (t) => {
  const akash = new AkashClient("http://127.0.0.1:9", "akash-1", 60);
  t.after(() => akash.close());
  t.mock.method(akash, "state", async () => ({
    params: Params.fromPartial({ reporters: [address] }),
    totals: Totals.fromPartial({ reporterEpoch: 1n }),
    height: 10,
    time: 1n,
  }));
  const prototype = SigningStargateClient.prototype;
  t.mock.method(prototype, "getSequence", async () => ({
    accountNumber: 7n,
    sequence: 3,
  }));
  t.mock.method(prototype, "getChainId", async () => "akash-1");
  const sent: Uint8Array[] = [];
  t.mock.method(prototype, "broadcastTx", async (bytes: Uint8Array) => {
    sent.push(bytes);
    return { code: 0, transactionHash: "AB" };
  });
  const fees = { amount: "10", gas: "500000" };
  for (const mode of ["other-key", "wrong-doc", "garbage", "fail"])
    await assert.rejects(() =>
      akash.broadcaster(signer(mode), address, fees)(message),
    );
  assert.equal(sent.length, 0);
  assert.equal(
    await akash.broadcaster(signer("ok"), address, fees)(message),
    "AB",
  );
  assert.equal(sent.length, 1);
  assert.ok(verifies(TxRaw.decode(sent[0]!)));
});

test("externalSigner config is validated before any signer runs", () => {
  const command = "/usr/local/bin/akt-sign";
  assert.deepEqual(parseExternalSigner({ command }, address), {
    command,
    args: [],
    timeoutSeconds: 10,
    publicKeyBase64: undefined,
  });
  const publicKeyBase64 = Buffer.from(publicKey).toString("base64");
  assert.deepEqual(
    parseExternalSigner(
      {
        command,
        args: ["--key", "reporter"],
        timeoutSeconds: 30,
        publicKeyBase64,
      },
      address,
    ),
    {
      command,
      args: ["--key", "reporter"],
      timeoutSeconds: 30,
      publicKeyBase64,
    },
  );
  const other = Buffer.from(
    secp256k1.getPublicKey(secp256k1.utils.randomPrivateKey(), true),
  ).toString("base64");
  for (const [value, error] of [
    [command, /must be an object/],
    [{}, /absolute path/],
    [{ command: "akt-sign" }, /absolute path/],
    [{ command, args: "--key" }, /externalSigner.args/],
    [{ command, args: [1] }, /externalSigner.args/],
    [{ command, timeoutSeconds: 0 }, /timeoutSeconds/],
    [{ command, timeoutSeconds: 121 }, /timeoutSeconds/],
    [{ command, publicKeyBase64: other }, /does not match reporter/],
    [{ command, publicKeyBase64: `${publicKeyBase64}=` }, /invalid/],
    [
      { command, publicKey: publicKeyBase64 },
      /unknown externalSigner.publicKey/,
    ],
  ] as const)
    assert.throws(() => parseExternalSigner(value, address), error);
});
