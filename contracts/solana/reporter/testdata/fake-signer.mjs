// Test-only external signer for reporter/signer.test.ts. It signs with the
// throwaway key in FAKE_SIGNER_KEY and misbehaves on request. Never give it a
// real key.
import { createHash } from "node:crypto";
import { appendFileSync } from "node:fs";
import { secp256k1 } from "@noble/curves/secp256k1";

const mode = process.argv[2] ?? "ok";
let input = "";
for await (const chunk of process.stdin) input += chunk;
if (process.env.FAKE_SIGNER_LOG)
  appendFileSync(process.env.FAKE_SIGNER_LOG, `${input}\n`);
const request = JSON.parse(input);
const secret = Buffer.from(process.env.FAKE_SIGNER_KEY ?? "", "hex");
const other = secp256k1.utils.randomPrivateKey();
let bytes = Buffer.from(request.signDocBase64, "base64");
if (mode === "wrong-doc") bytes = Buffer.concat([bytes, Buffer.from([0])]);
const key = mode === "other-key" ? other : secret;
let signature = secp256k1.sign(
  createHash("sha256").update(bytes).digest(),
  key,
  { prehash: false, lowS: true },
);
if (mode === "high-s")
  signature = new secp256k1.Signature(
    signature.r,
    secp256k1.CURVE.n - signature.s,
  );
const base64 = (value) => Buffer.from(value).toString("base64");
const response = {
  publicKeyBase64: base64(secp256k1.getPublicKey(key, mode !== "uncompressed")),
  signatureBase64: base64(signature.toCompactRawBytes()),
};
if (mode === "fail") process.exit(3);
if (mode === "hang") setInterval(() => undefined, 1000);
else if (mode === "garbage") process.stdout.write("signed\n");
else if (mode === "huge") process.stdout.write("x".repeat(100_000));
else if (mode === "short")
  process.stdout.write(
    JSON.stringify({
      ...response,
      signatureBase64: base64(new Uint8Array(63)),
    }),
  );
else process.stdout.write(`${JSON.stringify(response)}\n`);
