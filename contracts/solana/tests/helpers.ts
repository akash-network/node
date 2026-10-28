import assert from "node:assert/strict";
import {
  AnchorError,
  AnchorProvider,
  EventParser,
  type Program,
  web3,
} from "@coral-xyz/anchor";
import idl from "../target/idl/akt_solana.json";
import type { AktSolana } from "../target/types/akt_solana";

const AKT_PROGRAM = new web3.PublicKey(idl.address);
const AKT_ERROR_CODES = new Map(
  idl.errors.map((error) => [error.name, error.code]),
);

export function testProvider() {
  const env = AnchorProvider.env();
  assert.ok(
    ["127.0.0.1", "localhost", "[::1]"].includes(
      new URL(env.connection.rpcEndpoint).hostname,
    ),
    "tests require a local validator",
  );
  return new AnchorProvider(
    new web3.Connection(env.connection.rpcEndpoint, "confirmed"),
    env.wallet,
    {
      commitment: "confirmed",
      preflightCommitment: "confirmed",
    },
  );
}

export async function rejectsWithAnchorCode(
  operation: () => Promise<unknown>,
  expectedCode: string,
) {
  await assert.rejects(operation, (error: unknown) => {
    assert.ok(
      error instanceof AnchorError,
      `expected an Anchor error, received: ${String(error)}`,
    );
    assert.equal(error.error.errorCode.code, expectedCode);
    // AKT errors must fail with the number its IDL publishes.
    if (error.program.equals(AKT_PROGRAM) && AKT_ERROR_CODES.has(expectedCode))
      assert.equal(
        error.error.errorCode.number,
        AKT_ERROR_CODES.get(expectedCode),
      );
    return true;
  });
}

export async function rejectsWithProgramLogs(
  operation: () => Promise<unknown>,
  expected: RegExp,
) {
  await assert.rejects(operation, (error: unknown) => {
    assert.ok(error instanceof Error);
    const logs =
      error instanceof web3.SendTransactionError || error instanceof AnchorError
        ? (error.logs ?? [])
        : [];
    assert.match([error.message, ...logs].join("\n"), expected);
    return true;
  });
}

// Returns the data of the single event `name` (camelCase, as Anchor decodes
// it) that the program emitted in a confirmed transaction.
export async function emittedEvent(
  program: Program<AktSolana>,
  signature: string,
  name: string,
) {
  const transaction = await program.provider.connection.getTransaction(
    signature,
    { commitment: "confirmed", maxSupportedTransactionVersion: 0 },
  );
  const logs = transaction?.meta?.logMessages;
  assert.ok(logs, `no logs for ${signature}`);
  const events = [
    ...new EventParser(program.programId, program.coder).parseLogs(logs),
  ].filter((event) => event.name === name);
  assert.equal(events.length, 1, `expected one ${name} event`);
  return events[0].data;
}
