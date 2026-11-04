import { createHash } from "node:crypto";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import test from "node:test";
import type { Idl } from "@coral-xyz/anchor";
import idl from "../../../.cache/solana/target/idl/akt.json";
import deployed from "./fixtures/deployed-interface.json";

const declaredAddress = readFileSync(
  join(__dirname, "../programs/akt/src/lib.rs"),
  "utf8",
).match(/declare_id!\("([1-9A-HJ-NP-Za-km-z]+)"\)/);
assert.ok(declaredAddress, "program source must declare its address");
const programAddress = declaredAddress[1];

// Docs are not part of the interface.
function withoutDocs<T>(value: T): T {
  return JSON.parse(
    JSON.stringify(value, (key, field) => (key === "docs" ? undefined : field)),
  );
}

const defined = (name: string) => ({ defined: { name } });
const field = (name: string, type: unknown) => ({ name, type });
const struct = (name: string, fields: ReturnType<typeof field>[]) => ({
  name,
  type: { kind: "struct", fields },
});
const identity = field("id", defined("TransferId"));
const height = field("proof_height", "u64");
const slot = field("resolved_at_slot", "u64");
const outcome = field("outcome", defined("TransferOutcome"));
const nonce = field("nonce", { array: ["u8", 32] });
const compositeId = struct("TransferId", [
  field("owner", "bytes"),
  field("height", "u64"),
  field("sequence", "u64"),
]);
const replacements = new Map([
  [
    "ProofBuffer",
    struct("ProofBuffer", [
      field("submitter", "pubkey"),
      nonce,
      field("expected_len", "u32"),
      field("data", "bytes"),
    ]),
  ],
  [
    "TransferStatus",
    struct("TransferStatus", [identity, height, slot, outcome]),
  ],
  [
    "TransferResolved",
    struct("TransferResolved", [identity, height, slot, outcome]),
  ],
]);
const seed = (value: string) => ({
  kind: "const",
  value: [...Buffer.from(value)],
});
const arg = (path: string) => ({ kind: "arg", path });
const bufferField = (path: string) => ({
  kind: "account",
  path: `proof_buffer.${path}`,
  account: "ProofBuffer",
});

function migratedInstruction(
  instruction: (typeof deployed.instructions)[number],
) {
  const name = instruction.name;
  const resolution = name === "mint_transfer" || name === "void_transfer";
  const writeBuffer = name === "append_proof" || name === "close_proof_buffer";
  const args =
    name === "create_proof_buffer"
      ? [nonce, field("expected_len", "u32")]
      : name === "mint_transfer"
        ? [
            identity,
            field("recipient_key", "pubkey"),
            field("amount", "u64"),
            height,
          ]
        : name === "void_transfer"
          ? [identity, field("settlement_status", "u8"), height]
          : instruction.args;
  return {
    ...instruction,
    args,
    accounts: instruction.accounts.map((account) => {
      if (resolution && account.name === "status" && "pda" in account) {
        // Anchor cannot encode hash-derived seeds; callers supply the PDA.
        const { pda: _pda, ...status } = account;
        return status;
      }
      if ((resolution || writeBuffer) && account.name === "proof_buffer") {
        return {
          ...account,
          pda: {
            seeds: [
              seed("proof"),
              resolution
                ? bufferField("submitter")
                : { kind: "account", path: "submitter" },
              bufferField("nonce"),
            ],
          },
        };
      }
      return account;
    }),
  };
}

export function assertCanonicalIdl(value: unknown): asserts value is Idl {
  const before = withoutDocs(deployed);
  const expectedTypes = [
    ...before.types
      .filter((entry) => entry.name !== "TransferRecord")
      .map((entry) => replacements.get(entry.name) ?? entry),
    compositeId,
  ].sort((a, b) => a.name.localeCompare(b.name));
  assert.deepEqual(withoutDocs(value), {
    ...before,
    address: programAddress,
    metadata: idl.metadata,
    errors: idl.errors,
    instructions: [
      ...before.instructions.map(migratedInstruction),
      {
        name: "grow_proof_buffer",
        discriminator: [
          ...createHash("sha256")
            .update("global:grow_proof_buffer")
            .digest()
            .subarray(0, 8),
        ],
        accounts: [
          {
            name: "submitter",
            writable: true,
            signer: true,
            relations: ["proof_buffer"],
          },
          {
            name: "proof_buffer",
            writable: true,
            pda: {
              seeds: [
                seed("proof"),
                { kind: "account", path: "submitter" },
                bufferField("nonce"),
              ],
            },
          },
          {
            name: "system_program",
            address: "11111111111111111111111111111111",
          },
        ],
        args: [],
      },
    ].sort((a, b) => a.name.localeCompare(b.name)),
    types: expectedTypes,
  });
}

test("changes only the canonical transfer and proof interface", () =>
  assertCanonicalIdl(idl));

test("the IDL error table lists only AKT errors", () => {
  assert.deepEqual(
    idl.errors.map((error) => [error.code, error.name]),
    [
      "InvalidProgramData",
      "UnauthorizedInitializer",
      "InvalidAuthority",
      "UnauthorizedGuardian",
      "InvalidMetadataUri",
      "InvalidMetadata",
      "InvalidChainId",
      "InvalidPolicy",
      "InvalidLightClient",
      "WrongChain",
      "ClientFrozen",
      "ClientExpired",
      "InvalidConsensusTimestamp",
      "InvalidProofHeight",
      "InvalidProofLength",
      "InvalidProofOffset",
      "IncompleteProof",
      "BridgeDisabled",
      "CpiNotAllowed",
      "PrecedingInstruction",
      "InvalidRecord",
      "InvalidClock",
      "TransferLimit",
      "DailyLimit",
      "InvalidProofNonce",
      "InvalidSettlementStatus",
      "InvalidVoidAuthorization",
      "InvalidMembershipProof",
      "ProofBufferTooSmall",
    ].map((name, index) => [7000 + index, name]),
  );
});
