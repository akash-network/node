import assert from "node:assert/strict";
import test from "node:test";
import idl from "../target/idl/akt_solana.json";
import deployed from "./fixtures/deployed-interface.json";

type Named = { name: string };
type Interface = Record<
  "instructions" | "accounts" | "events" | "types",
  Named[]
> & { address: string };

// Docs are not part of the interface.
function withoutDocs(value: unknown): Interface {
  return JSON.parse(
    JSON.stringify(value, (key, field) => (key === "docs" ? undefined : field)),
  );
}

// fixtures/deployed-interface.json is the interface of the zero-supply
// genesis build, without docs or errors. That build dropped the genesis
// treasury, so networks launched with the 1,000,000 AKT genesis mint must be
// redeployed rather than upgraded in place. From here on, existing accounts,
// clients and event decoders keep working only if every instruction, account,
// event and type it lists stays byte-for-byte the same: names,
// discriminators, arguments, account lists, PDA seeds and field layouts.
// Additions are allowed.
test("keeps the deployed program interface", () => {
  const current = withoutDocs(idl);
  const before = withoutDocs(deployed);
  assert.equal(current.address, before.address);
  for (const section of [
    "instructions",
    "accounts",
    "events",
    "types",
  ] as const) {
    const entries = new Map(
      current[section].map((entry) => [entry.name, entry]),
    );
    for (const entry of before[section])
      assert.deepEqual(
        entries.get(entry.name),
        entry,
        `${section}: ${entry.name} changed`,
      );
  }
  assert.deepEqual(
    current.instructions.map((instruction) => instruction.name),
    before.instructions.map((instruction) => instruction.name),
  );
  assert.deepEqual(
    current.events.map((event) => event.name),
    [
      "BridgeConfigured",
      "BridgeEnabledSet",
      "GuardianPaused",
      "LaunchInitialized",
      "LightClientRepinned",
      "MintPolicySet",
      "PauseGuardianSet",
      "TokenMetadataSet",
      "TransferResolved",
    ],
  );
});

// Clients and explorers decode a failed instruction's custom error code with
// this table, so it must be AKT's own. Anchor once replaced it with the ICS07
// client's errors; see programs/akt-solana/ics07-tendermint-interface. AKT's
// codes start at 7000 so they never collide with ICS07 codes from 6000, which
// a failed membership CPI returns through mint_transfer and void_transfer.
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
    ].map((name, index) => [7000 + index, name]),
  );
});
