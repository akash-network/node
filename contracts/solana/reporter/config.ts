import assert from "node:assert/strict";
import { web3 } from "@coral-xyz/anchor";
import { fromBech32, toBech32 } from "@cosmjs/encoding";
import {
  reporterPublicKey,
  strictBase64,
  type ExternalSignerConfig,
} from "./signer";

const SIGNER_FIELDS = ["command", "args", "timeoutSeconds", "publicKeyBase64"];

// The command runs without a shell, so require an absolute path.
export function signerCommand(command: unknown) {
  assert.ok(
    typeof command === "string" && command.startsWith("/"),
    "external signer command must be an absolute path",
  );
  return command;
}

export function parseExternalSigner(
  value: unknown,
  reporter: string,
): ExternalSignerConfig {
  assert.ok(
    typeof value === "object" && value !== null && !Array.isArray(value),
    "externalSigner must be an object",
  );
  for (const name of Object.keys(value))
    assert.ok(SIGNER_FIELDS.includes(name), `unknown externalSigner.${name}`);
  const fields: { [name: string]: unknown } = { ...value };
  const args = fields.args ?? [];
  assert.ok(
    Array.isArray(args) &&
      args.length <= 32 &&
      args.every((arg) => typeof arg === "string"),
    "invalid externalSigner.args",
  );
  const timeoutSeconds = fields.timeoutSeconds ?? 10;
  assert.ok(
    typeof timeoutSeconds === "number" &&
      Number.isSafeInteger(timeoutSeconds) &&
      timeoutSeconds > 0 &&
      timeoutSeconds <= 120,
    "invalid externalSigner.timeoutSeconds",
  );
  const publicKeyBase64 = fields.publicKeyBase64;
  if (publicKeyBase64 !== undefined) {
    assert.ok(
      typeof publicKeyBase64 === "string",
      "invalid externalSigner.publicKeyBase64",
    );
    reporterPublicKey(
      strictBase64(publicKeyBase64, "externalSigner.publicKeyBase64"),
      reporter,
    );
  }
  return {
    command: signerCommand(fields.command),
    args: args.slice(),
    timeoutSeconds,
    publicKeyBase64,
  };
}

export function parseConfig(value: unknown) {
  assert.ok(
    typeof value === "object" && value !== null,
    "config must be an object",
  );
  const fields = value;
  function field(name: string): unknown {
    assert.ok(name in fields, `missing config field ${name}`);
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
  function integer(name: string, max: number) {
    const result = field(name);
    assert.ok(
      typeof result === "number" &&
        Number.isSafeInteger(result) &&
        result > 0 &&
        result <= max,
      `invalid ${name}`,
    );
    return result;
  }
  function optionalInteger(name: string, max: number) {
    return name in fields ? integer(name, max) : undefined;
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
  function optionalPubkey(name: string) {
    return name in fields ? pubkey(name) : undefined;
  }
  const reporter = string("reporter");
  const decoded = fromBech32(reporter, 90);
  assert.ok(
    decoded.prefix === "akash" &&
      decoded.data.length === 20 &&
      toBech32("akash", decoded.data) === reporter,
    "invalid reporter address",
  );
  const feeAmountUakt = string("feeAmountUakt");
  const gasLimit = string("gasLimit");
  assert.match(feeAmountUakt, /^(0|[1-9][0-9]*)$/);
  assert.match(gasLimit, /^[1-9][0-9]*$/);
  assert.ok(
    BigInt(gasLimit) <= 20_000_000n && BigInt(feeAmountUakt) <= 1_000_000n,
    "local reporter fee bounds exceeded",
  );
  const genesisHash = string("solanaGenesisHash");
  assert.equal(
    new web3.PublicKey(genesisHash).toBase58(),
    genesisHash,
    "invalid genesis hash",
  );
  return {
    akashRpc: url("akashRpc"),
    solanaRpc: url("solanaRpc"),
    akashChainId: string("akashChainId"),
    reporter,
    genesisHash,
    programId: pubkey("programId"),
    mint: pubkey("mint"),
    // Unset follows the light client the bridge account currently names.
    lightClientProgram: optionalPubkey("lightClientProgram"),
    maxAgeSeconds: integer("maxAgeSeconds", 600),
    maxSlotLead: optionalInteger("maxSlotLead", 100_000),
    // Applies only to nodes without the OpenTransfers query.
    maxTransfers: integer("maxTransfers", 10_000),
    // Transfer IDs the rolling terminal audit rechecks per pass.
    auditPerScan: optionalInteger("auditPerScan", 10_000) ?? 100,
    pollSeconds: integer("pollSeconds", 60),
    feeAmountUakt,
    gasLimit,
    externalSigner:
      "externalSigner" in fields
        ? parseExternalSigner(field("externalSigner"), reporter)
        : undefined,
  };
}
