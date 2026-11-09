import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  chmod,
  readFile,
  rm,
  stat,
  symlink,
  writeFile,
} from "node:fs/promises";
import { join } from "node:path";
import test from "node:test";
import { makeSignBytes, makeSignDoc } from "@cosmjs/proto-signing";
import { secp256k1 } from "@noble/curves/secp256k1";
import { ed25519 } from "@noble/curves/ed25519";
import { temporaryDirectory } from "../local/test-workspace";
import { loadPayer } from "../local/relay-config";
import { loadSigner } from "../reporter/key";
import { createKey, keyAddress } from "./keys";

test("generated reporter key signs the exact native SignDoc", async (t) => {
  const directory = await temporaryDirectory("operator-keys-");
  t.after(() => rm(directory, { recursive: true, force: true }));
  const path = join(directory, "reporter.key");
  const address = await createKey("reporter", path);
  assert.equal(await keyAddress("reporter", path), address);
  assert.equal((await stat(path)).mode & 0o777, 0o600);
  const signer = await loadSigner(path, address);
  const document = makeSignDoc(
    new Uint8Array([1, 2]),
    new Uint8Array([3]),
    "akashnet-2",
    7,
  );
  const response = await signer.signDirect(address, document);
  assert.deepEqual(response.signed, document);
  const digest = createHash("sha256").update(makeSignBytes(document)).digest();
  assert.ok(
    secp256k1.verify(
      Buffer.from(response.signature.signature, "base64"),
      digest,
      Buffer.from(response.signature.pub_key.value, "base64"),
    ),
  );
  await assert.rejects(
    loadSigner(path, "another-reporter"),
    /does not match reporter/,
  );
});

test("generated relayer key is a standard usable Solana keypair", async (t) => {
  const directory = await temporaryDirectory("operator-payer-");
  t.after(() => rm(directory, { recursive: true, force: true }));
  const path = join(directory, "payer.json");
  const address = await createKey("relayer", path);
  const payer = await loadPayer(path);
  assert.equal(payer.publicKey.toBase58(), address);
  const message = new Uint8Array([1, 2, 3]);
  const signature = ed25519.sign(message, payer.secretKey.subarray(0, 32));
  assert.ok(ed25519.verify(signature, message, payer.publicKey.toBytes()));
});

test("keys cannot be overwritten, loaded through symlinks, or read with broad permissions", async (t) => {
  const directory = await temporaryDirectory("operator-custody-");
  t.after(() => rm(directory, { recursive: true, force: true }));
  for (const role of ["relayer", "reporter"] as const) {
    const path = join(directory, role);
    await createKey(role, path);
    const before = await readFile(path);
    await assert.rejects(createKey(role, path), { code: "EEXIST" });
    assert.deepEqual(await readFile(path), before);
    const link = `${path}.link`;
    await symlink(path, link);
    await assert.rejects(createKey(role, link), { code: "EEXIST" });
    await assert.rejects(keyAddress(role, link));
    await chmod(path, 0o640);
    await assert.rejects(keyAddress(role, path), /owner-only regular file/);
  }
});

test("reporter key files reject seed phrases", async (t) => {
  const directory = await temporaryDirectory("operator-key-format-");
  t.after(() => rm(directory, { recursive: true, force: true }));
  const path = join(directory, "reporter.key");
  await writeFile(path, "example seed phrase is not a raw private key", {
    mode: 0o600,
  });
  await assert.rejects(loadSigner(path), /32-byte lowercase hex secp256k1 key/);
});
