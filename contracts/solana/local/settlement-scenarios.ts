import assert from "node:assert/strict";
import { join } from "node:path";
import { fromBech32 } from "@cosmjs/encoding";
import { sinkMessage, sinkResponseId } from "../shared/sink";
import { AnchorProvider, Wallet, web3 } from "@coral-xyz/anchor";
import {
  burn,
  createAccount,
  getMint,
  getAssociatedTokenAddressSync,
} from "@solana/spl-token";
import { relayContext } from "./relay";
import { processes, stop } from "./stack";
import {
  ROOT,
  PROFILE,
  CHAIN,
  PROGRAM_ID,
  pda,
  LE64,
  cosmosSigner,
  cosmosWallet,
  fee,
  save,
  sleep,
} from "./common";
import { SolanaObserver, decodeRecord } from "../reporter/solana";
import { planOutcome, reportedObservations } from "../reporter/policy";
import {
  Sink_Status as Status,
  QuerySinkResponse_SinkStatus,
  Hold,
  Outcome,
  Decision,
  Sink_MsgUpdateParams as MsgUpdateParams,
  MsgResolveTransfer,
  type Observation,
  Sink_Params as Params,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { govern, governanceAddress } from "./governance";
import { reconcile } from "./reconcile";
async function main() {
  assert.equal(PROFILE, "testnetify");
  for (const entry of await processes())
    if (
      ["relayer", "reporter0", "reporter1", "reporter2", "reconciler"].includes(
        entry.name,
      )
    )
      await stop(entry);
  const relay = await relayContext();
  const sender = await cosmosSigner("sender");
  const mint = pda("mint");
  const payer = relay.wallet.wallet.payer;
  assert.ok(payer);
  const original = (await relay.akash.state()).params;
  const observer = new SolanaObserver(relay.wallet.connection, {
    genesisHash: await relay.wallet.connection.getGenesisHash(),
    programId: PROGRAM_ID,
    mint,
    // Unpinned: follows the light client the bridge names.
    akashChainId: CHAIN,
    maxAgeSeconds: 60,
  });
  const results: object[] = [];
  let changedParams = false;
  async function record(value: object) {
    results.push(value);
    console.log(JSON.stringify(value));
    await save(join(ROOT, "settlement-scenarios.json"), results);
  }
  async function deposit(
    amount: bigint,
    address = web3.Keypair.generate().publicKey,
  ) {
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
    const receipt = await relay.akash.sink(responseId, tx.height);
    assert.equal(receipt.status, QuerySinkResponse_SinkStatus.transfer_pending);
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
    return id;
  }
  async function relayDeposit(id: bigint) {
    const record = decodeRecord(
      (await relay.akash.snapshot(id)).transfer.record,
    );
    while ((await relay.akash.state()).height <= Number(record.height) + 1)
      await sleep(500);
    const target = (await relay.akash.state()).height - 1;
    await relay.update(target);
    await relay.relay(id, target);
    const status = await relay.program.account.transferStatus.fetch(
      pda("transfer", PROGRAM_ID, LE64(id)),
    );
    while (
      (await relay.wallet.connection.getSlot("finalized")) <
      status.resolvedAtSlot.toNumber()
    )
      await sleep(300);
  }
  async function report(index: number, id: bigint) {
    const signer = await cosmosWallet(`reporter${index}`);
    const address = (await signer.getAccounts())[0].address;
    const snapshot = await relay.akash.snapshot(id);
    const own = snapshot.transfer.settlement?.votes.find(
      (v) => v.reporter === address,
    )?.observation;
    const observed = await observer.observe(
      decodeRecord(snapshot.transfer.record),
      reportedObservations(snapshot.transfer),
      own,
    );
    const plan = planOutcome(address, snapshot, observed);
    const submit = relay.akash.broadcaster(signer, address, {
      amount: "12500",
      gas: "500000",
    });
    for (const msg of plan.messages) await submit(msg);
    return observed;
  }
  async function settleMint(id: bigint) {
    for (let i = 0; i < 3; i++) await report(i, id);
    assert.equal(
      (await relay.akash.snapshot(id)).transfer.settlement?.status,
      Status.STATUS_MINTED,
    );
    const receipt = await relay.akash.sink(
      id.toString(),
      (await relay.akash.state()).height,
    );
    assert.equal(receipt.status, QuerySinkResponse_SinkStatus.completed);
    assert.equal(receipt.transfer?.settlement?.status, Status.STATUS_MINTED);
  }
  async function falseVoid(index: number, id: bigint) {
    const signer = await cosmosSigner(`reporter${index}`);
    const snapshot = await relay.akash.snapshot(id);
    assert.ok(snapshot.transfer.settlement);
    try {
      const tx = await signer.client.signAndBroadcast(
        signer.address,
        [
          {
            typeUrl: "/akash.sink.v1.MsgReportOutcome",
            value: {
              reporter: signer.address,
              id,
              reporterEpoch: snapshot.totals.reporterEpoch,
              round: snapshot.transfer.settlement.round,
              observation: {
                outcome: Outcome.OUTCOME_VOIDED,
                slot: BigInt(
                  await relay.wallet.connection.getSlot("finalized"),
                ),
                transactionSignature: new Uint8Array(64).fill(7),
              },
            },
          },
        ],
        fee,
      );
      assert.equal(tx.code, 0, tx.rawLog);
    } finally {
      signer.client.disconnect();
    }
  }
  async function finalize(id: bigint, expectedError?: RegExp) {
    const signer = await cosmosSigner("reporter0");
    try {
      const send = () =>
        signer.client.signAndBroadcast(
          signer.address,
          [
            {
              typeUrl: "/akash.sink.v1.MsgFinalizeRefund",
              value: { executor: signer.address, id },
            },
          ],
          fee,
        );
      if (expectedError) {
        try {
          const tx = await send();
          assert.notEqual(tx.code, 0);
          assert.match(tx.rawLog ?? "", expectedError);
        } catch (error) {
          assert.match(
            error instanceof Error ? error.message : String(error),
            expectedError,
          );
        }
      } else {
        const tx = await send();
        assert.equal(tx.code, 0, tx.rawLog);
        const receipt = await relay.akash.sink(id.toString(), tx.height);
        assert.equal(receipt.status, QuerySinkResponse_SinkStatus.void);
        assert.equal(
          receipt.transfer?.settlement?.status,
          Status.STATUS_REFUNDED,
        );
      }
    } finally {
      signer.client.disconnect();
    }
  }
  async function resolve(
    id: bigint,
    decision: Decision,
    observation?: Observation,
  ) {
    const before = (await relay.akash.snapshot(id)).transfer.settlement;
    assert.ok(before);
    return govern(
      "Resolve local sink reporter dispute",
      [
        {
          typeUrl: "/akash.sink.v1.MsgResolveTransfer",
          value: MsgResolveTransfer.encode(
            MsgResolveTransfer.fromPartial({
              authority: governanceAddress,
              id,
              decision,
              observation,
              reason: "Reconciled against this local Solana validator",
            }),
          ).finish(),
        },
      ],
      async () =>
        (
          await relay.akash.snapshot(id)
        ).transfer.settlement?.round.toBigInt() ===
        before.round.toBigInt() + 1n,
    );
  }
  async function params(value: Params) {
    await govern(
      "Set local sink refund test limits",
      [
        {
          typeUrl: "/akash.sink.v1.MsgUpdateParams",
          value: MsgUpdateParams.encode({
            authority: governanceAddress,
            params: value,
          }).finish(),
        },
      ],
      async () => {
        const live = (await relay.akash.state()).params;
        return (
          live.maxTransfer.equals(value.maxTransfer) &&
          live.dailyRefundCap.equals(value.dailyRefundCap)
        );
      },
    );
  }
  try {
    assert.equal(
      (await relay.akash.state()).totals.pendingCount.toBigInt(),
      0n,
      "finish prior scenarios first",
    );
    const recipient = web3.Keypair.generate();
    await save(
      join(ROOT, "burn-recipient.json"),
      Array.from(recipient.secretKey),
    );
    const offline = await deposit(5_000_000n, recipient.publicKey);
    await relayDeposit(offline);
    assert.equal(
      (await relay.akash.snapshot(offline)).transfer.settlement?.status,
      Status.STATUS_PENDING,
    );
    let view = await reconcile();
    assert.equal(view.status, "healthy");
    assert.equal(view.awaitingMintReportsUakt, "5000000");
    const burned = await burn(
      relay.wallet.connection,
      payer,
      getAssociatedTokenAddressSync(mint, recipient.publicKey),
      mint,
      recipient,
      2_000_000n,
    );
    await relay.wallet.connection.confirmTransaction(burned, "finalized");
    const previousBurns = BigInt(view.cumulativeBurnedUakt);
    view = await reconcile();
    assert.equal(view.status, "healthy");
    assert.equal(BigInt(view.cumulativeBurnedUakt) - previousBurns, 2_000_000n);
    await report(0, offline);
    await report(1, offline);
    assert.equal(
      (await relay.akash.snapshot(offline)).transfer.settlement?.status,
      Status.STATUS_PENDING,
    );
    await report(2, offline);
    assert.equal((await reconcile()).awaitingMintReportsUakt, "0");
    await record({
      scenario: "reporters-offline-and-spl-burn",
      passed: true,
      id: offline.toString(),
      burnedUakt: "2000000",
      burnTransaction: burned,
    });

    const vetoed = await deposit(1_000_000n);
    for (let i = 0; i < 3; i++) await falseVoid(i, vetoed);
    assert.equal(
      (await relay.akash.snapshot(vetoed)).transfer.settlement?.status,
      Status.STATUS_VOIDED,
    );
    await report(3, vetoed); // The independent fourth reporter sees no Solana status and vetoes.
    assert.equal(
      (await relay.akash.snapshot(vetoed)).transfer.settlement?.hold,
      Hold.HOLD_VETO,
    );
    const vetoedReceipt = await relay.akash.sink(
      vetoed.toString(),
      (await relay.akash.state()).height,
    );
    assert.equal(vetoedReceipt.status, QuerySinkResponse_SinkStatus.void);
    assert.equal(
      vetoedReceipt.transfer?.settlement?.status,
      Status.STATUS_VOIDED,
    );
    assert.equal(vetoedReceipt.transfer.settlement.hold, Hold.HOLD_VETO);
    const balanceBeforeVeto = await sender.client.getBalance(
      sender.address,
      "uakt",
    );
    await finalize(vetoed, /unchallenged confirmed void/);
    assert.deepEqual(
      await sender.client.getBalance(sender.address, "uakt"),
      balanceBeforeVeto,
    );
    await resolve(vetoed, Decision.DECISION_RESTART);
    await relayDeposit(vetoed);
    await settleMint(vetoed);
    await record({
      scenario: "false-void-quorum-veto",
      passed: true,
      id: vetoed.toString(),
      refunded: false,
      recoveredThroughGovernance: true,
    });

    await falseVoid(0, offline);
    assert.equal(
      (await relay.akash.snapshot(offline)).transfer.settlement?.hold,
      Hold.HOLD_CONFLICT,
    );
    await finalize(offline, /unchallenged confirmed void/);
    const actual = await observer.observe(
      decodeRecord((await relay.akash.snapshot(offline)).transfer.record),
    );
    assert.equal(actual.kind, "resolved");
    assert.ok(actual.kind === "resolved");
    await resolve(offline, Decision.DECISION_CONFIRM_MINT, actual.observation);
    assert.equal(
      (await relay.akash.snapshot(offline)).transfer.settlement?.hold,
      Hold.HOLD_NONE,
    );
    await record({
      scenario: "conflicting-reports",
      passed: true,
      id: offline.toString(),
      governancePreservedMint: true,
    });

    const state = await relay.akash.state();
    const spent =
      state.totals.refundDay.toBigInt() === state.time / 86400n
        ? state.totals.refundedToday.toBigInt()
        : 0n;
    await params(
      Params.fromPartial({
        ...original,
        maxTransfer: 2_000_000n,
        dailyRefundCap: spent + 2_000_000n,
      }),
    );
    changedParams = true;
    const tokenAccount = await createAccount(
      relay.wallet.connection,
      payer,
      mint,
      relay.wallet.publicKey,
      web3.Keypair.generate(),
    );
    const refunds: bigint[] = [];
    for (let i = 0; i < 2; i++) {
      const id = await deposit(2_000_000n, tokenAccount);
      refunds.push(id);
      await relayDeposit(id);
      for (let r = 0; r < 3; r++) await report(r, id);
      const until = (await relay.akash.snapshot(id)).transfer.settlement
        ?.challengeEnd;
      assert.ok(until);
      const challenged = await relay.akash.sink(
        id.toString(),
        (await relay.akash.state()).height,
      );
      assert.equal(challenged.status, QuerySinkResponse_SinkStatus.void);
      assert.equal(
        challenged.transfer?.settlement?.status,
        Status.STATUS_VOIDED,
      );
      assert.ok(challenged.transfer.settlement.challengeEnd.equals(until));
      await finalize(id, /challenge window/);
      while ((await relay.akash.state()).time < until.toBigInt())
        await sleep(500);
      const before = BigInt(
        (await sender.client.getBalance(sender.address, "uakt")).amount,
      );
      await finalize(id, i === 1 ? /daily refund cap/ : undefined);
      const after = BigInt(
        (await sender.client.getBalance(sender.address, "uakt")).amount,
      );
      assert.equal(after - before, i === 0 ? 2_000_000n : 0n);
    }
    await params(original);
    changedParams = false;
    await finalize(refunds[1]);
    const paid = await sender.client.getBalance(sender.address, "uakt");
    await finalize(refunds[1]);
    assert.deepEqual(
      await sender.client.getBalance(sender.address, "uakt"),
      paid,
    );
    await record({
      scenario: "challenge-and-refund-cap",
      passed: true,
      ids: refunds.map(String),
      recoveredAfterGovernance: true,
      duplicateRefundPaidNothing: true,
    });

    const unfunded = await deposit(1_000_000n);
    const native = decodeRecord(
      (await relay.akash.snapshot(unfunded)).transfer.record,
    );
    while ((await relay.akash.state()).height <= Number(native.height) + 1)
      await sleep(500);
    const proofHeight = (await relay.akash.state()).height - 1;
    await relay.update(proofHeight);
    const rescue = web3.Keypair.generate();
    await save(join(ROOT, "payer-rescue.json"), Array.from(rescue.secretKey));
    const rescueWallet = new AnchorProvider(
      relay.wallet.connection,
      new Wallet(rescue),
      relay.wallet.opts,
    );
    const balance = await relay.wallet.connection.getBalance(payer.publicKey);
    assert.ok(Number.isSafeInteger(balance));
    await relay.wallet.sendAndConfirm(
      new web3.Transaction().add(
        web3.SystemProgram.transfer({
          fromPubkey: payer.publicKey,
          toPubkey: rescue.publicKey,
          lamports: balance - 5000,
        }),
      ),
    );
    assert.equal(await relay.wallet.connection.getBalance(payer.publicKey), 0);
    const supplyBefore = (await getMint(relay.wallet.connection, mint)).supply;
    try {
      await assert.rejects(
        () => relay.relay(unfunded, proofHeight),
        /insufficient|debit|credit|lamports/i,
      );
      assert.equal(
        await relay.wallet.connection.getAccountInfo(
          pda("transfer", PROGRAM_ID, LE64(unfunded)),
        ),
        null,
      );
      assert.equal(
        (await relay.akash.snapshot(unfunded)).transfer.settlement?.status,
        Status.STATUS_PENDING,
      );
      assert.equal(
        (await getMint(relay.wallet.connection, mint)).supply,
        supplyBefore,
      );
    } finally {
      const held = await relay.wallet.connection.getBalance(rescue.publicKey);
      await rescueWallet.sendAndConfirm(
        new web3.Transaction().add(
          web3.SystemProgram.transfer({
            fromPubkey: rescue.publicKey,
            toPubkey: payer.publicKey,
            lamports: held - 5000,
          }),
        ),
      );
    }
    await relay.relay(unfunded, proofHeight);
    await relay.relay(unfunded, proofHeight);
    assert.equal(
      (await getMint(relay.wallet.connection, mint)).supply - supplyBefore,
      1_000_000n,
    );
    const last = await relay.program.account.transferStatus.fetch(
      pda("transfer", PROGRAM_ID, LE64(unfunded)),
    );
    while (
      (await relay.wallet.connection.getSlot("finalized")) <
      last.resolvedAtSlot.toNumber()
    )
      await sleep(300);
    await settleMint(unfunded);
    await record({
      scenario: "payer-exhaustion-retry",
      passed: true,
      id: unfunded.toString(),
      mintedExactlyOnce: true,
    });
    const final = await reconcile();
    assert.equal(final.status, "healthy");
    await record({ scenario: "final-reconciliation", ...final });
  } finally {
    if (changedParams) await params(original);
    sender.client.disconnect();
    relay.close();
  }
}
void main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
