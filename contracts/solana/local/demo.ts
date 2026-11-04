import {
  DestType,
  Hold,
  MsgSink,
  Sink_Status as Status,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { Program, web3 } from "@coral-xyz/anchor";
import {
  createAccount,
  getAccount,
  getAssociatedTokenAddressSync,
  getMint,
} from "@solana/spl-token";
import assert from "node:assert/strict";
import { join } from "node:path";
import idl from "../../../.cache/solana/target/idl/akt.json";
import type { Akt } from "../../../.cache/solana/target/types/akt";
import { AkashClient } from "../reporter/akash";
import { decodeRecord } from "../reporter/solana";
import { sinkIdParts, type SinkId } from "../shared/id";
import { coinAmount, statusAddress } from "../shared/proof";
import { sinkMessage, sinkResponseId } from "../shared/sink";
import {
  AKASH_RPC,
  CHAIN,
  cosmosSigner,
  fee,
  json,
  object,
  pda,
  PROGRAM_ID,
  provider,
  ROOT,
  save,
  sleep,
} from "./common";
import { reconcile } from "./reconcile";

async function main() {
  const local = object(await json(join(ROOT, "initialized.json")));
  assert.ok(typeof local.sinkAddress === "string");
  const sink = local.sinkAddress;
  const wallet = await provider();
  const program = new Program<Akt>(idl, wallet);
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
    const beforeInvalid = (await akash.state()).totals;
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
              destType: DestType.DEST_TYPE_SOLANA,
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
    assert.deepEqual((await akash.state()).totals, beforeInvalid);
    console.log(
      "Direct bank deposit and malformed MsgSink destination rejected without creating a transfer.",
    );

    async function deposit(address: web3.PublicKey, amount: bigint) {
      const tx = await sender.client.signAndBroadcast(
        sender.address,
        [
          sinkMessage({
            sender: sender.address,
            recipient: address.toBase58(),
            amountUakt: amount.toString(),
          }),
        ],
        fee,
        "",
      );
      assert.equal(tx.code, 0, `deposit failed: ${tx.rawLog}`);
      const responseId = sinkResponseId(tx.msgResponses);
      const id = responseId;
      const receipt = await akash.transfer(responseId, tx.height);
      assert.ok(receipt?.settlement);
      assert.equal(receipt.status, Status.STATUS_TRANSFER_PENDING);
      const record = decodeRecord(receipt);
      assert.equal(record.id, id);
      assert.equal(record.amount, amount);
      assert.equal(record.height, BigInt(tx.height));
      assert.ok(record.recipient.equals(address));
      assert.equal(sinkIdParts(id).owner, sender.address);
      console.log(
        JSON.stringify({
          event: "deposit",
          id: id,
          amount: amount.toString(),
          akashTransaction: tx.transactionHash,
        }),
      );
      return { id, tx };
    }
    async function wait(id: SinkId, expected: Status) {
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
        const step = `${Status[snapshot.transfer.status]} (${settlement.votes.length} reports)`;
        if (step !== last) {
          console.log(`Transfer ${id}: ${step}`);
          last = step;
        }
        if (snapshot.transfer.status === expected) {
          const state = await akash.state();
          const receipt = await akash.transfer(id, state.height);
          assert.equal(receipt?.status, expected);
          assert.deepEqual(receipt.data, snapshot.transfer.data);
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
      id: good.id,
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
      statusAddress(PROGRAM_ID, bad.id),
    );
    assert.ok(status.outcome.voided?.reason.invalidRecipient);
    assert.equal(
      (await getAccount(wallet.connection, tokenAccount)).amount,
      0n,
    );
    results.push({
      id: bad.id,
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
      coinAmount(after.totals.mintedAmount) -
        coinAmount(stateBefore.totals.mintedAmount),
      10_000_000n,
    );
    assert.equal(
      coinAmount(after.totals.refundedAmount) -
        coinAmount(stateBefore.totals.refundedAmount),
      3_000_000n,
    );
    assert.equal(mintAfter.supply - mintBefore.supply, 10_000_000n);
    assert.equal(coinAmount(after.totals.pendingAmount), 0n);
    assert.equal(coinAmount(after.totals.refundPendingAmount), 0n);
    assert.equal(
      BigInt((await sender.client.getBalance(sink, "uakt")).amount),
      coinAmount(after.totals.mintedAmount),
    );
    const reconciliation = await reconcile();
    assert.equal(
      reconciliation.status,
      "healthy",
      JSON.stringify(reconciliation.alerts),
    );
    assert.equal(
      mintAfter.supply + BigInt(reconciliation.cumulativeBurnedUakt),
      coinAmount(after.totals.mintedAmount),
    );
    const report = {
      completedAt: new Date().toISOString(),
      chainId: CHAIN,
      mint: mint.toBase58(),
      results,
      solanaSupplyUakt: mintAfter.supply.toString(),
      cumulativeBurnedUakt: reconciliation.cumulativeBurnedUakt,
      lockedUakt: coinAmount(after.totals.mintedAmount).toString(),
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
