import assert from "node:assert/strict";
import { AnchorProvider, BN, utils, web3 } from "@coral-xyz/anchor";
import clientIdl from "../idls/ics07_tendermint.json";
import { LE64, pda } from "./common";
import {
  LOCAL_LIGHT_CLIENT,
  accessProgram,
  clientProgram,
  relayerAccess,
  type LightClientIds,
} from "./light-client";
import { sendTransaction } from "./transactions";

// ConsensusStateStore: discriminator, height u64, then timestamp u64 first.
const STORE_SIZE = 88;
const STORE_DISCRIMINATOR = Buffer.from(
  clientIdl.accounts.find((a) => a.name === "ConsensusStateStore")!
    .discriminator,
);

export type ConsensusPruneResult = {
  latestHeight: number;
  scanned: number; // consensus-state accounts found
  expired: number; // expired states other than the latest
  pruned: number;
  failed: number;
  remaining: number; // expired states left for later ticks
  reclaimedLamports: number;
  feeLamports: number;
  signatures: string[];
  skipped?: string;
};

// Closes expired consensus states other than the latest, oldest first. Each
// call scans once and sends at most ceil(maxPerTick / perTransaction)
// transactions. It stops at the first failed transaction; a later call
// rescans. The program enforces role, expiry and latest-height rules; this
// selection only avoids sending transactions that would fail.
export async function pruneExpiredConsensusStates(
  wallet: AnchorProvider,
  {
    maxPerTick = 20,
    perTransaction = 8,
    lightClient = LOCAL_LIGHT_CLIENT,
  }: {
    maxPerTick?: number;
    perTransaction?: number;
    // The client the bridge pins; fresh local networks use the default.
    lightClient?: LightClientIds;
  } = {},
): Promise<ConsensusPruneResult> {
  assert.ok(Number.isInteger(maxPerTick) && maxPerTick > 0);
  assert.ok(
    Number.isInteger(perTransaction) &&
      perTransaction > 0 &&
      perTransaction <= 10,
  );
  const clientId = lightClient.program;
  const accessId = lightClient.accessManager;
  const client = clientProgram(wallet, clientId);
  const access = accessProgram(wallet, accessId);
  const clientState = pda("client", clientId);
  const state = await client.account.clientState.fetch(
    clientState,
    "confirmed",
  );
  const latest = BigInt(state.latestHeight.revisionHeight.toString());
  const trusting = BigInt(state.trustingPeriod.toString()) * 1_000_000_000n;
  const result: ConsensusPruneResult = {
    latestHeight: Number(latest),
    scanned: 0,
    expired: 0,
    pruned: 0,
    failed: 0,
    remaining: 0,
    reclaimedLamports: 0,
    feeLamports: 0,
    signatures: [],
  };
  const roles = (
    await access.account.accessManager.fetch(
      pda("access_manager", accessId),
      "confirmed",
    )
  ).roles;
  // Open relaying does not extend to pruning; it needs a real member.
  if (relayerAccess(roles, wallet.publicKey, clientId) !== "member")
    return { ...result, skipped: "payer lacks RELAYER_ROLE" };
  const clock = await wallet.connection.getAccountInfo(
    web3.SYSVAR_CLOCK_PUBKEY,
    "confirmed",
  );
  assert.ok(clock && clock.data.length === 40, "Solana clock unavailable");
  const now = clock.data.readBigInt64LE(32) * 1_000_000_000n;
  const accounts = await wallet.connection.getProgramAccounts(clientId, {
    commitment: "confirmed",
    dataSlice: { offset: 8, length: 16 },
    filters: [
      { dataSize: STORE_SIZE },
      {
        memcmp: {
          offset: 0,
          bytes: utils.bytes.bs58.encode(STORE_DISCRIMINATOR),
        },
      },
    ],
  });
  result.scanned = accounts.length;
  const candidates = accounts
    .map(({ pubkey, account }) => ({
      pubkey,
      lamports: account.lamports,
      height: account.data.readBigUInt64LE(0),
      timestamp: account.data.readBigUInt64LE(8),
    }))
    .filter(
      (entry) =>
        entry.height !== latest &&
        entry.timestamp + trusting <= now &&
        entry.pubkey.equals(
          pda("consensus_state", clientId, LE64(entry.height)),
        ),
    )
    .sort((a, b) => (a.height < b.height ? -1 : 1));
  result.expired = candidates.length;
  const selected = candidates.slice(0, maxPerTick);
  for (let i = 0; i < selected.length; i += perTransaction) {
    const batch = selected.slice(i, i + perTransaction);
    const instructions = await Promise.all(
      batch.map((entry) =>
        client.methods
          .pruneConsensusState(new BN(entry.height.toString()))
          .accountsStrict({
            clientState,
            appState: pda("app_state", clientId),
            accessManager: pda("access_manager", accessId),
            consensusStateStore: entry.pubkey,
            submitter: wallet.publicKey,
            instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
          })
          .instruction(),
      ),
    );
    let failed = false;
    try {
      const sent = await sendTransaction({
        wallet,
        stage: "consensus-prune",
        instructions,
        observe: async () => {},
      });
      result.signatures.push(sent.signature);
      result.feeLamports += sent.fee;
      if (sent.error === null) {
        result.pruned += batch.length;
        result.reclaimedLamports += batch.reduce(
          (sum, entry) => sum + entry.lamports,
          0,
        );
      } else failed = true;
    } catch (error) {
      // Expired or unconfirmed transactions are left to the next scan.
      if (!(error instanceof Error && /^consensus-prune: /.test(error.message)))
        throw error;
      failed = true;
    }
    if (failed) {
      result.failed += batch.length;
      break;
    }
  }
  result.remaining = result.expired - result.pruned;
  return result;
}
