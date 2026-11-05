import assert from "node:assert/strict";
import { unlink } from "node:fs/promises";
import { AnchorProvider, utils, web3 } from "@coral-xyz/anchor";
import { atomicJson, optionalJson } from "./durable";
import { object, sleep } from "./common";

export type TransactionResult = {
  stage: string;
  signature: string;
  bytes: number;
  accounts: number;
  fee: number;
  compute: number;
  error: unknown;
  logs: string[];
  // Fee plus rent the payer locked, minus rent returned to it.
  payerLamports?: number;
};
type Intent = {
  stage: string;
  genesis: string;
  payer: string;
  raw: string;
  signature: string;
  lastValidBlockHeight: number;
  accounts: number;
};
function decodeIntent(value: unknown): Intent {
  const v = object(value);
  assert.ok(
    typeof v.stage === "string" &&
      typeof v.genesis === "string" &&
      typeof v.payer === "string" &&
      typeof v.raw === "string" &&
      typeof v.signature === "string" &&
      typeof v.lastValidBlockHeight === "number" &&
      Number.isSafeInteger(v.lastValidBlockHeight) &&
      v.lastValidBlockHeight > 0 &&
      typeof v.accounts === "number" &&
      Number.isInteger(v.accounts) &&
      v.accounts > 0 &&
      v.accounts <= 64,
  );
  return {
    stage: v.stage,
    genesis: v.genesis,
    payer: v.payer,
    raw: v.raw,
    signature: v.signature,
    lastValidBlockHeight: v.lastValidBlockHeight,
    accounts: v.accounts,
  };
}
async function settle(
  wallet: AnchorProvider,
  intent: Intent,
  submitted?: (signature: string) => Promise<void>,
): Promise<TransactionResult> {
  assert.equal(intent.genesis, await wallet.connection.getGenesisHash());
  assert.equal(intent.payer, wallet.publicKey.toBase58());
  const raw = Buffer.from(intent.raw, "base64");
  assert.ok(raw.length <= 1232 && raw.toString("base64") === intent.raw);
  const decoded = web3.VersionedTransaction.deserialize(raw);
  assert.equal(
    utils.bytes.bs58.encode(decoded.signatures[0]),
    intent.signature,
  );
  assert.ok(decoded.message.staticAccountKeys[0].equals(wallet.publicKey));
  const deadline = Date.now() + 90_000;
  let resendAt = 0;
  for (;;) {
    const status = (
      await wallet.connection.getSignatureStatuses([intent.signature], {
        searchTransactionHistory: true,
      })
    ).value[0];
    const confirmed =
      status?.confirmationStatus === "confirmed" ||
      status?.confirmationStatus === "finalized";
    // Appending addresses and recording a terminal assembly require a finalized
    // outcome so a fork cannot leave the durable journal ahead of chain state.
    if (
      confirmed &&
      (!(
        intent.stage.startsWith("lookup-") ||
        intent.stage === "assembly" ||
        intent.stage === "cleanup" ||
        intent.stage.startsWith("proof-") ||
        intent.stage === "resolve"
      ) ||
        status?.confirmationStatus === "finalized")
    )
      break;
    if (
      !status &&
      (await wallet.connection.getBlockHeight("finalized")) >
        intent.lastValidBlockHeight
    )
      throw new Error(
        `${intent.stage}: expired transaction history unresolved (${intent.signature}); durable intent retained; restore signature history and transaction metadata`,
      );
    assert.ok(
      Date.now() < deadline,
      `${intent.stage}: confirmation unresolved (${intent.signature}); durable intent retained`,
    );
    if (!confirmed && Date.now() >= resendAt) {
      await wallet.connection.sendRawTransaction(raw, {
        skipPreflight: true,
        maxRetries: 0,
      });
      await submitted?.(intent.signature);
      resendAt = Date.now() + 2000;
    }
    await sleep(150);
  }
  let result = await wallet.connection.getTransaction(intent.signature, {
    commitment: "confirmed",
    maxSupportedTransactionVersion: 0,
  });
  for (let i = 0; !result && i < 20; i++) {
    await sleep(100);
    result = await wallet.connection.getTransaction(intent.signature, {
      commitment: "confirmed",
      maxSupportedTransactionVersion: 0,
    });
  }
  assert.ok(result?.meta, `missing transaction metadata: ${intent.signature}`);
  return {
    stage: intent.stage,
    signature: intent.signature,
    bytes: raw.length,
    accounts: intent.accounts,
    fee: result.meta.fee,
    compute: result.meta.computeUnitsConsumed ?? 0,
    error: result.meta.err,
    logs: result.meta.logMessages ?? [],
    // The fee payer is always the first account.
    payerLamports: result.meta.preBalances[0] - result.meta.postBalances[0],
  };
}
export async function resumeTransaction(
  wallet: AnchorProvider,
  path: string,
  observe: (result: TransactionResult) => Promise<void>,
) {
  const pending = await optionalJson(path);
  if (pending === undefined) return;
  const intent = decodeIntent(pending);
  const result = await settle(wallet, intent);
  await observe(result); // Persist terminal effects before dropping intent.
  await unlink(path);
}
export async function sendTransaction({
  wallet,
  stage,
  instructions,
  lookup,
  pendingPath,
  observe,
  submitted,
  preflight = false,
}: {
  wallet: AnchorProvider;
  stage: string;
  instructions: web3.TransactionInstruction[];
  lookup?: web3.AddressLookupTableAccount;
  pendingPath?: string;
  observe: (result: TransactionResult) => Promise<void>;
  submitted?: (signature: string) => Promise<void>;
  preflight?: boolean;
}) {
  if (pendingPath)
    assert.equal(
      await optionalJson(pendingPath),
      undefined,
      "resolve previous transaction before submitting another",
    );
  const hash = await wallet.connection.getLatestBlockhash();
  const message = new web3.TransactionMessage({
    payerKey: wallet.publicKey,
    recentBlockhash: hash.blockhash,
    instructions,
  }).compileToV0Message(lookup ? [lookup] : []);
  const accounts =
    message.staticAccountKeys.length +
    message.addressTableLookups.reduce(
      (sum, table) =>
        sum + table.readonlyIndexes.length + table.writableIndexes.length,
      0,
    );
  assert.ok(accounts <= 64);
  const tx = await wallet.wallet.signTransaction(
    new web3.VersionedTransaction(message),
  );
  const raw = tx.serialize();
  assert.ok(raw.length <= 1232, `${stage}: ${raw.length} bytes exceeds 1232`);
  const intent = {
    stage,
    genesis: await wallet.connection.getGenesisHash(),
    payer: wallet.publicKey.toBase58(),
    raw: Buffer.from(raw).toString("base64"),
    signature: utils.bytes.bs58.encode(tx.signatures[0]),
    lastValidBlockHeight: hash.lastValidBlockHeight,
    accounts,
  };
  if (preflight) {
    const simulation = await wallet.connection.simulateTransaction(tx, {
      sigVerify: true,
      commitment: "confirmed",
    });
    assert.equal(simulation.value.err, null, `${stage}: simulation failed`);
  }
  if (pendingPath) await atomicJson(pendingPath, intent);
  const result = await settle(wallet, intent, submitted);
  await observe(result);
  if (pendingPath) await unlink(pendingPath);
  return result;
}
