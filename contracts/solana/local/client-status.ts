import assert from "node:assert/strict";
import { AnchorProvider, Program } from "@coral-xyz/anchor";
import type { Ics07Tendermint } from "../.deps/ibc-contracts/ibc-solana/target/types/ics07_tendermint";
import { pda, LE64 } from "./common";
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
  assert.ok(
    state.frozenHeight.revisionHeight.isZero(),
    "light client is frozen",
  );
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
  const expires =
    BigInt(latest.consensusState.timestamp.toString()) +
    BigInt(state.trustingPeriod.toString()) * 1_000_000_000n;
  assert.ok(
    BigInt(time) * 1_000_000_000n < expires,
    "light client expired; uploads stopped, pending escrow preserved; reviewed checkpoint recovery required",
  );
}
