import assert from "node:assert/strict";
import { join } from "node:path";
import { createHash } from "node:crypto";
import { Program, utils, web3 } from "@coral-xyz/anchor";
import { TOKEN_PROGRAM_ID, unpackMint } from "@solana/spl-token";
import {
  QueryBalanceRequest,
  QueryBalanceResponse,
} from "cosmjs-types/cosmos/bank/v1beta1/query";
import idl from "../target/idl/akt_solana.json";
import type { AktSolana } from "../target/types/akt_solana";
import { AkashClient } from "../reporter/akash";
import { decodeRecord } from "../reporter/solana";
import { Sink_Status as Status } from "@akashnetwork/chain-sdk/private-types/akash.v1";
import {
  ROOT,
  CHAIN,
  AKASH_RPC,
  PROGRAM_ID,
  pda,
  LE64,
  provider,
  object,
  sleep,
} from "./common";
import { atomicJson, optionalJson, withUploadLock } from "./durable";
import {
  inSnapshot,
  reconcileSettings,
  transferAlerts,
} from "./reconcile-policy";

function amount(value: unknown) {
  assert.ok(
    typeof value === "string" && /^(0|[1-9][0-9]*)$/.test(value),
    "invalid amount",
  );
  return BigInt(value);
}
// Burn accounting comes from successful finalized SPL instructions, including
// inner CPI calls. It is independent of the supply difference being checked.
async function indexBurns(
  connection: web3.Connection,
  mint: web3.PublicKey,
  slot: number,
) {
  const path = join(ROOT, "burn-index.json");
  const genesis = await connection.getGenesisHash();
  const saved = await optionalJson(path);
  const prior = saved === undefined ? undefined : object(saved);
  if (prior) {
    assert.equal(prior.genesis, genesis);
    assert.equal(prior.mint, mint.toBase58());
    assert.ok(
      typeof prior.slot === "number" &&
        Number.isSafeInteger(prior.slot) &&
        prior.slot <= slot,
      "invalid burn checkpoint slot",
    );
    assert.ok(
      typeof prior.cursor === "string" &&
        typeof prior.genesisTransaction === "string",
    );
  }
  let burned = prior ? amount(prior.burned) : 0n;
  let genesisTransaction = prior?.genesisTransaction;
  let before: string | undefined;
  const assertHistoryCoverage = async () => {
    if (prior) {
      assert.ok(typeof prior.slot === "number");
      assert.ok(
        (await connection.getFirstAvailableBlock()) <= prior.slot + 1,
        "burn checkpoint predates retained RPC history; refusing a gap",
      );
    }
  };
  await assertHistoryCoverage();
  let reachedCursor = false;
  const signatures: web3.ConfirmedSignatureInfo[] = [];
  for (let page = 0; ; page++) {
    assert.ok(
      page < 100,
      "burn history exceeds local 10000-transaction scan bound",
    );
    const rows = await connection.getSignaturesForAddress(
      mint,
      { before, limit: 100 },
      "finalized",
    );
    if (!rows.length) break;
    for (const row of rows) {
      if (prior && typeof prior.slot === "number" && row.slot <= prior.slot) {
        reachedCursor = true;
        break;
      }
      if (row.slot <= slot) signatures.push(row);
    }
    if (reachedCursor) break;
    before = rows.at(-1)?.signature;
  }
  // A quiet mint's last signature may be pruned before the index checkpoint.
  // Coverage is anchored to the finalized slot already scanned, not that old
  // signature. No newer row is also a valid scan, while the slot range exists.
  await assertHistoryCoverage();
  const burns: { signature: string; slot: number; amount: string }[] = [];
  const initialize = createHash("sha256")
    .update("global:initialize")
    .digest()
    .subarray(0, 8);
  for (const row of signatures) {
    if (row.err) continue;
    // Holders' wallets send version 1 transactions (SIMD-0385).
    const tx = await connection.getParsedTransaction(row.signature, {
      commitment: "finalized",
      maxSupportedTransactionVersion: 1,
    });
    assert.ok(
      tx?.meta && tx.slot === row.slot && tx.meta.err === null,
      "finalized transaction history unavailable",
    );
    const instructions = [
      ...tx.transaction.message.instructions,
      ...(tx.meta.innerInstructions ?? []).flatMap((i) => i.instructions),
    ];
    let burnedHere = 0n;
    // Genesis mints nothing; its initialize transaction, which creates the
    // mint, anchors the start of the burn history.
    const initializes = tx.transaction.message.instructions.some(
      (ix) =>
        ix.programId.equals(PROGRAM_ID) &&
        "data" in ix &&
        Buffer.from(utils.bytes.bs58.decode(ix.data))
          .subarray(0, 8)
          .equals(initialize) &&
        ix.accounts.some((a) => a.equals(mint)),
    );
    for (const ix of instructions) {
      if (!ix.programId.equals(TOKEN_PROGRAM_ID) || !("parsed" in ix)) continue;
      const parsed = object(ix.parsed);
      const info = object(parsed.info);
      if (info.mint !== mint.toBase58()) continue;
      if (parsed.type === "burn") burnedHere += amount(info.amount);
      else if (parsed.type === "burnChecked")
        burnedHere += amount(object(info.tokenAmount).amount);
    }
    if (initializes) genesisTransaction = row.signature;
    if (burnedHere > 0n) {
      burned += burnedHere;
      burns.push({
        signature: row.signature,
        slot: row.slot,
        amount: burnedHere.toString(),
      });
    }
  }
  assert.ok(
    typeof genesisTransaction === "string",
    "mint genesis transaction missing; refusing an incomplete burn baseline",
  );
  const cursor = signatures[0]?.signature ?? prior?.cursor;
  assert.ok(typeof cursor === "string");
  await assertHistoryCoverage();
  const next = {
    version: 1,
    genesis,
    mint: mint.toBase58(),
    cursor,
    slot,
    burned: burned.toString(),
    genesisTransaction,
  };
  // Write idempotent evidence before advancing the cursor. A crash can repeat a
  // write, but cannot permanently omit an event from the audit trail.
  for (const burn of burns)
    await atomicJson(join(ROOT, "burn-events", `${burn.signature}.json`), burn);
  await atomicJson(path, next);
  return next;
}

export async function reconcile() {
  return withUploadLock(
    join(ROOT, "reconciliation.lock"),
    reconcileSnapshot,
    10000,
  );
}
async function reconcileSnapshot() {
  const { reportLagSeconds } = reconcileSettings(process.env);
  const wallet = await provider();
  const program = new Program<AktSolana>(idl, wallet);
  const akash = new AkashClient(AKASH_RPC, CHAIN, 60);
  try {
    const state = await akash.state();
    const records = [];
    for (let id = 1n; id <= state.totals.lastTransferId.toBigInt(); id++) {
      const transfer = await akash.transfer(id, state.height);
      records.push({ transfer, record: decodeRecord(transfer.record) });
    }
    const mint = pda("mint");
    const addresses = [
      mint,
      ...records.map(({ record }) =>
        pda("transfer", PROGRAM_ID, LE64(record.id)),
      ),
    ];
    // The first page pins the finalized slot for supply. Later pages may come
    // from a newer slot; statuses resolved after the pinned slot are excluded.
    const snapshot = await wallet.connection.getMultipleAccountsInfoAndContext(
      addresses.slice(0, 100),
      { commitment: "finalized" },
    );
    const infos = [...snapshot.value];
    for (let i = 100; i < addresses.length; i += 100) {
      const page = await wallet.connection.getMultipleAccountsInfoAndContext(
        addresses.slice(i, i + 100),
        { commitment: "finalized", minContextSlot: snapshot.context.slot },
      );
      assert.ok(page.context.slot >= snapshot.context.slot);
      infos.push(...page.value);
    }
    // Deposits created after the Akash snapshot could already have minted. Retry
    // rather than report a false supply gap from mismatched chain snapshots.
    assert.equal(
      (await akash.state()).totals.lastTransferId.toBigInt(),
      state.totals.lastTransferId.toBigInt(),
      "deposits advanced during reconciliation; retry snapshot",
    );
    const mintInfo = unpackMint(mint, infos[0], TOKEN_PROGRAM_ID);
    assert.equal(mintInfo.decimals, 6);
    assert.equal(mintInfo.freezeAuthority, null);
    assert.ok(mintInfo.mintAuthority?.equals(pda("mint_authority")));
    let observedMinted = 0n;
    let awaitingReports = 0n;
    let pendingUnminted = 0n;
    const alerts: string[] = [];
    for (const [i, { transfer, record }] of records.entries()) {
      const info = infos[i + 1];
      const settlement = transfer.settlement;
      assert.ok(settlement);
      let solana: "minted" | "voided" | undefined;
      let resolvedAtSlot: bigint | undefined;
      if (info) {
        assert.ok(info.owner.equals(PROGRAM_ID) && !info.executable);
        const status = program.coder.accounts.decode<
          Awaited<ReturnType<typeof program.account.transferStatus.fetch>>
        >("transferStatus", info.data);
        assert.equal(status.id.toString(), record.id.toString());
        resolvedAtSlot = BigInt(status.resolvedAtSlot.toString());
        if (!inSnapshot(resolvedAtSlot, snapshot.context.slot))
          resolvedAtSlot = undefined;
        else if (status.outcome.minted) {
          assert.equal(
            BigInt(status.outcome.minted.amount.toString()),
            record.amount,
          );
          assert.ok(status.outcome.minted.recipient.equals(record.recipient));
          solana = "minted";
          observedMinted += record.amount;
          if (settlement.status === Status.STATUS_PENDING)
            awaitingReports += record.amount;
        } else solana = "voided";
      }
      if (solana !== "minted" && settlement.status === Status.STATUS_PENDING)
        pendingUnminted += record.amount;
      alerts.push(
        ...transferAlerts({
          id: record.id,
          akash: settlement.status,
          solana,
          resolvedAtSlot,
          slot: snapshot.context.slot,
          reportLagSeconds,
        }),
      );
    }
    const initialized = object(
      await optionalJson(join(ROOT, "initialized.json")),
    );
    assert.ok(typeof initialized.sinkAddress === "string");
    const balanceReply = await akash.comet.abciQuery({
      path: "/cosmos.bank.v1beta1.Query/Balance",
      data: QueryBalanceRequest.encode({
        address: initialized.sinkAddress,
        denom: "uakt",
      }).finish(),
      height: state.height,
    });
    assert.equal(balanceReply.code, 0);
    assert.equal(balanceReply.height, state.height);
    const balance = BigInt(
      QueryBalanceResponse.decode(balanceReply.value).balance?.amount ?? "0",
    );
    const accounted =
      state.totals.pendingAmount.toBigInt() +
      state.totals.mintedAmount.toBigInt() +
      state.totals.voidedAmount.toBigInt();
    if (balance !== accounted)
      alerts.push("Akash bank escrow differs from sink totals");
    const burnIndex = await indexBurns(
      wallet.connection,
      mint,
      snapshot.context.slot,
    );
    // Every SPL AKT was minted for a proven record whose escrow stays locked
    // on Akash: supply plus burns equals the escrow behind Minted records.
    const backing = observedMinted;
    const supplyAndBurns = mintInfo.supply + BigInt(burnIndex.burned);
    if (backing !== supplyAndBurns)
      alerts.push(
        `backing minus supply and burns: ${backing - supplyAndBurns} uakt`,
      );
    const report = {
      checkedAt: new Date().toISOString(),
      chainId: CHAIN,
      genesis: burnIndex.genesis,
      status: alerts.length ? "mismatch" : "healthy",
      akashHeight: state.height,
      solanaSlot: snapshot.context.slot,
      lockedUakt: balance.toString(),
      observedMintedUakt: observedMinted.toString(),
      solanaSupplyUakt: mintInfo.supply.toString(),
      cumulativeBurnedUakt: burnIndex.burned,
      awaitingMintReportsUakt: awaitingReports.toString(),
      pendingUnmintedUakt: pendingUnminted.toString(),
      voidedEscrowUakt: state.totals.voidedAmount.toString(),
      alerts,
    };
    await atomicJson(join(ROOT, "reconciliation.json"), report);
    return report;
  } finally {
    akash.close();
  }
}
async function main() {
  const watch = process.argv.includes("--watch");
  let stopped = false;
  process.once("SIGTERM", () => {
    stopped = true;
  });
  process.once("SIGINT", () => {
    stopped = true;
  });
  do {
    try {
      const result = await reconcile();
      console.log(JSON.stringify(result));
      if (!watch)
        assert.equal(result.status, "healthy", result.alerts.join("; "));
    } catch (error) {
      if (!watch) throw error;
      const result = {
        status: "unavailable",
        checkedAt: new Date().toISOString(),
        error: error instanceof Error ? error.message : String(error),
      };
      await atomicJson(join(ROOT, "reconciliation.json"), result);
      console.error(JSON.stringify(result));
    }
    if (watch && !stopped) await sleep(5000);
  } while (watch && !stopped);
}
if (require.main === module)
  void main().catch((error) => {
    console.error(error);
    process.exitCode = 1;
  });
