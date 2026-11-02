import assert from "node:assert/strict";
import { join } from "node:path";
import { fromBech32 } from "@cosmjs/encoding";
import { Program, web3 } from "@coral-xyz/anchor";
import {
  createAccount,
  getAccount,
  getAssociatedTokenAddressSync,
  getMint,
} from "@solana/spl-token";
import idl from "../target/idl/akt_solana.json";
import type { AktSolana } from "../target/types/akt_solana";
import { sinkMessage, sinkResponseId } from "../shared/sink";
import { AkashClient } from "../reporter/akash";
import {
  Sink_Status as Status,
  Hold,
  MsgSink,
  QuerySinkResponse_SinkStatus,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { decodeRecord } from "../reporter/solana";
import { reconcile } from "./reconcile";
import {
  ROOT,
  CHAIN,
  AKASH_RPC,
  LE64,
  PROGRAM_ID,
  provider,
  cosmosSigner,
  fee,
  pda,
  json,
  object,
  save,
  sleep,
} from "./common";

async function main() {
  const local = object(await json(join(ROOT, "initialized.json")));
  assert.ok(typeof local.sinkAddress === "string");
  const sink = local.sinkAddress;
  const wallet = await provider();
  const program = new Program<AktSolana>(idl, wallet);
  const akash = new AkashClient(AKASH_RPC, CHAIN, 60);
  const sender = await cosmosSigner("sender");
  const mint = pda("mint");
  const mintBefore = await getMint(wallet.connection, mint);
  const stateBefore = await akash.state();
  const recipient = web3.Keypair.generate();
  await save(
    join(ROOT, `demo-recipient-${recipient.publicKey.toBase58()}-keypair.json`),
    Array.from(recipient.secretKey),
  );
  const results: object[] = [];
  try {
    const beforeInvalid = (
      await akash.state()
    ).totals.lastTransferId.toBigInt();
    await assert.rejects(async () => {
      const tx = await sender.client.sendTokens(
        sender.address,
        sink,
        [{ denom: "uakt", amount: "1000000" }],
        fee,
        JSON.stringify({
          v: 1,
          solana_address: recipient.publicKey.toBase58(),
        }),
      );
      if (tx.code !== 0) throw new Error(tx.rawLog);
      // x/bank blocks the sink address; only MsgSink can deposit.
    }, /sink|deposit|not allowed to receive funds/i);
    await assert.rejects(async () => {
      const tx = await sender.client.signAndBroadcast(
        sender.address,
        [
          {
            typeUrl: "/akash.sink.v1.MsgSink",
            value: MsgSink.fromPartial({
              sender: sender.address,
              destType: 1,
              dest: "bad-address",
              amount: { denom: "uakt", amount: "1000000" },
            }),
          },
        ],
        fee,
        "",
      );
      if (tx.code !== 0) throw new Error(tx.rawLog);
    }, /destination|address|base58/i);
    assert.equal(
      (await akash.state()).totals.lastTransferId.toBigInt(),
      beforeInvalid,
    );
    console.log(
      "Direct bank deposit and malformed MsgSink destination rejected without creating a transfer.",
    );

    async function deposit(address: web3.PublicKey, amount: bigint) {
      const tx = await sender.client.signAndBroadcast(
        sender.address,
        [
          sinkMessage({
            sender: sender.address,
            dest: address.toBase58(),
            amountUakt: amount.toString(),
          }),
        ],
        fee,
        "",
      );
      assert.equal(tx.code, 0, `deposit failed: ${tx.rawLog}`);
      const responseId = sinkResponseId(tx.msgResponses);
      const id = BigInt(responseId);
      const receipt = await akash.sink(responseId, tx.height);
      assert.equal(
        receipt.status,
        QuerySinkResponse_SinkStatus.transfer_pending,
      );
      assert.ok(receipt.transfer?.settlement);
      assert.equal(receipt.transfer.settlement.status, Status.STATUS_PENDING);
      const record = decodeRecord(receipt.transfer.record);
      assert.equal(record.id, id);
      assert.equal(record.amount, amount);
      assert.equal(record.height, BigInt(tx.height));
      assert.ok(record.recipient.equals(address));
      assert.deepEqual(
        Buffer.from(receipt.transfer.record).subarray(9, 29),
        Buffer.from(fromBech32(sender.address, 90).data),
      );
      console.log(
        JSON.stringify({
          event: "deposit",
          id: id.toString(),
          amount: amount.toString(),
          akashTransaction: tx.transactionHash,
        }),
      );
      return { id, tx };
    }
    async function wait(id: bigint, expected: Status) {
      // A deposit can arrive just after a keep-alive chose its proof height.
      // Allow two finalized upload cycles plus reporter voting and challenge.
      const end = Date.now() + 300_000;
      let last = "";
      while (Date.now() < end) {
        const snapshot = await akash.snapshot(id);
        const settlement = snapshot.transfer.settlement;
        assert.ok(settlement);
        assert.equal(
          settlement.hold,
          Hold.HOLD_NONE,
          "unexpected settlement dispute",
        );
        const step = `${Status[settlement.status]} (${settlement.votes.length} reports)`;
        if (step !== last) {
          console.log(`Transfer ${id}: ${step}`);
          last = step;
        }
        if (settlement.status === expected) {
          const state = await akash.state();
          const receipt = await akash.sink(id.toString(), state.height);
          assert.equal(
            receipt.status,
            expected === Status.STATUS_MINTED
              ? QuerySinkResponse_SinkStatus.completed
              : QuerySinkResponse_SinkStatus.void,
          );
          assert.equal(receipt.transfer?.settlement?.status, expected);
          assert.deepEqual(receipt.transfer.record, snapshot.transfer.record);
          return snapshot;
        }
        await sleep(1000);
      }
      throw new Error(
        `transfer ${id} did not reach ${Status[expected]}; inspect relayer/reporter logs`,
      );
    }

    const good = await deposit(recipient.publicKey, 10_000_000n);
    await wait(good.id, Status.STATUS_MINTED);
    const token = await getAccount(
      wallet.connection,
      getAssociatedTokenAddressSync(mint, recipient.publicKey),
      "finalized",
    );
    assert.equal(token.amount, 10_000_000n);
    results.push({
      id: good.id.toString(),
      outcome: "minted",
      amount: "10000000",
      recipient: recipient.publicKey.toBase58(),
      akashTransaction: good.tx.transactionHash,
    });
    console.log(
      "10 AKT minted to the fresh recipient; reporter quorum confirmed it on Akash.",
    );

    // An on-curve SPL account passes Akash's format check but is not a wallet.
    const payer = wallet.wallet.payer;
    assert.ok(payer, "local wallet needs its development payer key");
    const tokenAccount = await createAccount(
      wallet.connection,
      payer,
      mint,
      wallet.publicKey,
      web3.Keypair.generate(),
    );
    const bad = await deposit(tokenAccount, 3_000_000n);
    const senderAfterDeposit = BigInt(
      (await sender.client.getBalance(sender.address, "uakt")).amount,
    );
    await wait(bad.id, Status.STATUS_REFUNDED);
    const senderAfterRefund = BigInt(
      (await sender.client.getBalance(sender.address, "uakt")).amount,
    );
    assert.equal(senderAfterRefund - senderAfterDeposit, 3_000_000n);
    const status = await program.account.transferStatus.fetch(
      pda("transfer", PROGRAM_ID, LE64(bad.id)),
    );
    assert.ok(status.outcome.voided?.reason.invalidRecipient);
    assert.equal(
      (await getAccount(wallet.connection, tokenAccount)).amount,
      0n,
    );
    results.push({
      id: bad.id.toString(),
      outcome: "refunded",
      amount: "3000000",
      invalidRecipient: tokenAccount.toBase58(),
      akashTransaction: bad.tx.transactionHash,
    });
    console.log(
      "3 AKT permanently voided on Solana and refunded to the original Akash sender after the challenge.",
    );

    const after = await akash.state();
    const mintAfter = await getMint(wallet.connection, mint, "finalized");
    assert.equal(
      after.totals.mintedAmount.toBigInt() -
        stateBefore.totals.mintedAmount.toBigInt(),
      10_000_000n,
    );
    assert.equal(
      after.totals.refundedAmount.toBigInt() -
        stateBefore.totals.refundedAmount.toBigInt(),
      3_000_000n,
    );
    assert.equal(mintAfter.supply - mintBefore.supply, 10_000_000n);
    assert.equal(after.totals.pendingAmount.toBigInt(), 0n);
    assert.equal(after.totals.voidedAmount.toBigInt(), 0n);
    assert.equal(
      BigInt((await sender.client.getBalance(sink, "uakt")).amount),
      after.totals.mintedAmount.toBigInt(),
    );
    const reconciliation = await reconcile();
    assert.equal(
      reconciliation.status,
      "healthy",
      JSON.stringify(reconciliation.alerts),
    );
    assert.equal(
      mintAfter.supply + BigInt(reconciliation.cumulativeBurnedUakt),
      after.totals.mintedAmount.toBigInt(),
    );
    const report = {
      completedAt: new Date().toISOString(),
      chainId: CHAIN,
      mint: mint.toBase58(),
      results,
      solanaSupplyUakt: mintAfter.supply.toString(),
      cumulativeBurnedUakt: reconciliation.cumulativeBurnedUakt,
      lockedUakt: after.totals.mintedAmount.toString(),
    };
    await save(join(ROOT, "demo-result.json"), report);
    console.log(JSON.stringify(report, null, 2));
  } finally {
    akash.close();
    sender.client.disconnect();
  }
}
void main().catch((error: unknown) => {
  console.error(error);
  process.exitCode = 1;
});
