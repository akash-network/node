import assert from "node:assert/strict";
import { AnchorProvider, Program } from "@coral-xyz/anchor";
import type { Ics07Tendermint } from "../../../.cache/solana/deps/ibc-contracts/ibc-solana/target/types/ics07_tendermint";
import { pda, LE64 } from "./common";

export function clientStatus(input: {
  frozenHeight: bigint;
  consensusTimestampNs: bigint;
  trustingPeriodSeconds: bigint;
  solanaTimeSeconds: bigint;
}): "active" | "expired" | "frozen" {
  if (input.frozenHeight !== 0n) return "frozen";
  const expires =
    input.consensusTimestampNs + input.trustingPeriodSeconds * 1_000_000_000n;
  return input.solanaTimeSeconds * 1_000_000_000n < expires
    ? "active"
    : "expired";
}
// Addresses derive from client.programId, so the caller chooses the client:
// the one the bridge pins at runtime, the local ID on a fresh network.
export async function assertClientActive(
  wallet: AnchorProvider,
  client: Program<Ics07Tendermint>,
  chainId: string,
) {
  const clientId = client.programId;
  const state = await client.account.clientState.fetch(pda("client", clientId));
  assert.equal(state.chainId, chainId);
  const latest = await client.account.consensusStateStore.fetch(
    pda(
      "consensus_state",
      clientId,
      LE64(BigInt(state.latestHeight.revisionHeight.toString())),
    ),
  );
  const time = await wallet.connection.getBlockTime(
    await wallet.connection.getSlot("confirmed"),
  );
  assert.ok(time !== null, "Solana clock unavailable");
  const status = clientStatus({
    frozenHeight: BigInt(state.frozenHeight.revisionHeight.toString()),
    consensusTimestampNs: BigInt(latest.consensusState.timestamp.toString()),
    trustingPeriodSeconds: BigInt(state.trustingPeriod.toString()),
    solanaTimeSeconds: BigInt(time),
  });
  assert.equal(
    status,
    "active",
    `light client is ${status}; uploads stopped, pending escrow preserved; reviewed checkpoint recovery required`,
  );
}
