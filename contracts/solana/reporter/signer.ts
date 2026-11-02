import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { secp256k1 } from "@noble/curves/secp256k1";
import { toBech32 } from "@cosmjs/encoding";
import {
  makeSignBytes,
  type AccountData,
  type DirectSignResponse,
  type OfflineDirectSigner,
} from "@cosmjs/proto-signing";
import { PubKey } from "cosmjs-types/cosmos/crypto/secp256k1/keys";
import { SignMode } from "cosmjs-types/cosmos/tx/signing/v1beta1/signing";
import { AuthInfo, type SignDoc } from "cosmjs-types/cosmos/tx/v1beta1/tx";

export interface ExternalSignerConfig {
  command: string;
  args: readonly string[];
  timeoutSeconds: number;
  publicKeyBase64?: string;
}

const MAX_OUTPUT = 64 * 1024;

function sha256(bytes: Uint8Array) {
  return createHash("sha256").update(bytes).digest();
}

export function akashAddress(publicKey: Uint8Array) {
  return toBech32(
    "akash",
    createHash("ripemd160").update(sha256(publicKey)).digest(),
  );
}

export function strictBase64(value: string, name: string) {
  const bytes = Buffer.from(value, "base64");
  assert.equal(bytes.toString("base64"), value, `invalid ${name}`);
  return Uint8Array.from(bytes);
}

// A 33-byte compressed point on secp256k1 that derives `address`.
export function reporterPublicKey(bytes: Uint8Array, address: string) {
  assert.ok(
    bytes.length === 33 && (bytes[0] === 2 || bytes[0] === 3),
    "signer public key must be a 33-byte compressed secp256k1 key",
  );
  try {
    secp256k1.ProjectivePoint.fromHex(bytes).assertValidity();
  } catch {
    throw new Error("signer public key is not on secp256k1");
  }
  assert.equal(
    akashAddress(bytes),
    address,
    "signer public key does not match reporter",
  );
  return Uint8Array.from(bytes);
}

// Runs the command without a shell, writes `input` to stdin and returns
// stdout. A timeout, nonzero exit or oversized output rejects. The signer's
// stderr passes through to the reporter's stderr.
export function runSigner(
  command: string,
  args: readonly string[],
  input: string,
  timeoutMs: number,
) {
  return new Promise<string>((resolve, reject) => {
    const child = spawn(command, args, {
      stdio: ["pipe", "pipe", "inherit"],
      shell: false,
    });
    const chunks: Buffer[] = [];
    let size = 0;
    let settled = false;
    const finish = (error?: Error) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      if (error) {
        child.kill("SIGKILL");
        reject(error);
      } else resolve(Buffer.concat(chunks).toString("utf8"));
    };
    const timer = setTimeout(
      () => finish(new Error("external signer timed out")),
      timeoutMs,
    );
    child.on("error", () => finish(new Error("external signer did not run")));
    // A signer that exits without reading stdin is judged by its exit status.
    child.stdin.on("error", () => undefined);
    child.stdout.on("data", (chunk: Buffer) => {
      size += chunk.length;
      if (size > MAX_OUTPUT)
        finish(new Error("external signer output is too large"));
      else chunks.push(chunk);
    });
    child.on("close", (code, signal) =>
      finish(
        code === 0
          ? undefined
          : new Error(`external signer exited with ${code ?? signal}`),
      ),
    );
    child.stdin.end(input);
  });
}

// Checks a signer response for `signBytes`. Returns the low-S signature; the
// Cosmos SDK rejects high S, and normalizing preserves validity.
export function verifiedSignature(
  output: string,
  signBytes: Uint8Array,
  publicKey: Uint8Array,
  address: string,
) {
  let response: unknown;
  try {
    response = JSON.parse(output);
  } catch {
    throw new Error("external signer returned invalid JSON");
  }
  assert.ok(
    typeof response === "object" &&
      response !== null &&
      "publicKeyBase64" in response &&
      "signatureBase64" in response &&
      typeof response.publicKeyBase64 === "string" &&
      typeof response.signatureBase64 === "string",
    "external signer response needs publicKeyBase64 and signatureBase64",
  );
  const key = reporterPublicKey(
    strictBase64(response.publicKeyBase64, "signer public key"),
    address,
  );
  assert.ok(
    Buffer.from(key).equals(Buffer.from(publicKey)),
    "external signer used another key",
  );
  const raw = strictBase64(response.signatureBase64, "signer signature");
  assert.equal(raw.length, 64, "signer signature must be 64-byte r||s");
  let signature;
  try {
    signature = secp256k1.Signature.fromCompact(raw);
  } catch {
    throw new Error("signer signature is out of range");
  }
  if (signature.hasHighS()) signature = signature.normalizeS();
  const normalized = signature.toCompactRawBytes();
  assert.ok(
    secp256k1.verify(normalized, sha256(signBytes), key, {
      prehash: false,
      lowS: true,
    }),
    "external signer signature does not verify over the sign doc",
  );
  return normalized;
}

// SIGN_MODE_DIRECT signing through an operator command (KMS, HSM or other
// custody). Nothing is returned for broadcast unless the response carries the
// configured key and a valid signature over the exact sign doc bytes.
export class ExternalSigner implements OfflineDirectSigner {
  private readonly publicKey: Uint8Array;
  constructor(
    private readonly options: {
      command: string;
      args: readonly string[];
      timeoutMs: number;
      address: string;
      chainId: string;
      publicKey: Uint8Array;
    },
  ) {
    this.publicKey = reporterPublicKey(options.publicKey, options.address);
  }

  async getAccounts(): Promise<readonly AccountData[]> {
    return [
      {
        address: this.options.address,
        algo: "secp256k1",
        pubkey: this.publicKey,
      },
    ];
  }

  async signDirect(
    signerAddress: string,
    signDoc: SignDoc,
  ): Promise<DirectSignResponse> {
    const { address, chainId } = this.options;
    assert.equal(signerAddress, address, "sign request for another account");
    assert.equal(signDoc.chainId, chainId, "sign request for another chain");
    const { signerInfos } = AuthInfo.decode(signDoc.authInfoBytes);
    const info = signerInfos[0];
    assert.ok(
      signerInfos.length === 1 &&
        info?.modeInfo?.single?.mode === SignMode.SIGN_MODE_DIRECT &&
        info.publicKey?.typeUrl === "/cosmos.crypto.secp256k1.PubKey" &&
        Buffer.from(PubKey.decode(info.publicKey.value).key).equals(
          Buffer.from(this.publicKey),
        ),
      "sign doc does not name this signer",
    );
    const signBytes = makeSignBytes(signDoc);
    const output = await runSigner(
      this.options.command,
      this.options.args,
      JSON.stringify({
        chainId,
        accountNumber: signDoc.accountNumber.toString(),
        sequence: info.sequence.toString(),
        address,
        signDocBase64: Buffer.from(signBytes).toString("base64"),
      }),
      this.options.timeoutMs,
    );
    const signature = verifiedSignature(
      output,
      signBytes,
      this.publicKey,
      address,
    );
    return {
      signed: signDoc,
      signature: {
        pub_key: {
          type: "tendermint/PubKeySecp256k1",
          value: Buffer.from(this.publicKey).toString("base64"),
        },
        signature: Buffer.from(signature).toString("base64"),
      },
    };
  }
}
