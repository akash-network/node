import {
  Decision,
  Hold,
  MsgConfirmMint,
  MsgConfirmVoid,
  MsgRestartTransfer,
  Sink_MsgUpdateParams as MsgUpdateParams,
  Outcome,
  Sink_Params as Params,
  Sink_Status as Status,
  type Observation,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import type { DeliverTxResponse } from "@cosmjs/stargate";
import { AnchorProvider, Wallet, web3 } from "@coral-xyz/anchor";
import {
  burn,
  createAccount,
  getAssociatedTokenAddressSync,
  getMint,
} from "@solana/spl-token";
import assert from "node:assert/strict";
import { join } from "node:path";
import type { AkashClient } from "../reporter/akash";
import { planOutcome, reportedObservations } from "../reporter/policy";
import { SolanaObserver, decodeRecord } from "../reporter/solana";
import { sinkIdParts, type SinkId } from "../shared/id";
import { coinAmount, statusAddress } from "../shared/proof";
import { sinkMessage, sinkResponseId } from "../shared/sink";
import {
  CHAIN,
  PROFILE,
  PROGRAM_ID,
  ROOT,
  cosmosSigner,
  cosmosWallet,
  fee,
  pda,
  save,
  sleep,
} from "./common";
import { govern, governanceAddress } from "./governance";
import { reconcile } from "./reconcile";
import { relayContext } from "./relay";
import { processes, stop } from "./stack";
export async function expectRefundError(
  send: () => Promise<Pick<DeliverTxResponse, "code" | "rawLog">>,
  expectedError: RegExp,
) {
  const tx = await send().catch((error: unknown) => ({
    code: 1,
    rawLog: error instanceof Error ? error.message : String(error),
  }));
  assert.notEqual(tx.code, 0);
  assert.match(tx.rawLog ?? "", expectedError);
}

export async function refundCapScenario({
  prepare,
  state,
  setCap,
  finalize,
  balance,
  sleep,
}: {
  prepare: () => Promise<{ id: SinkId; challengeEnd: bigint }>;
  state: () => Promise<
    Pick<
      Awaited<ReturnType<AkashClient["state"]>>,
      "epoch" | "time" | "refundUsage"
    >
  >;
  setCap: (amount: bigint) => Promise<void>;
  finalize: (id: SinkId, expectedError?: RegExp) => Promise<void>;
  balance: () => Promise<bigint>;
  sleep: (ms: number) => Promise<void>;
}) {
  const candidates = [await prepare(), await prepare()];
  for (const { challengeEnd } of candidates) {
    while ((await state()).time < challengeEnd) await sleep(500);
  }
  const { epoch, refundUsage } = await state();
  assert.ok(epoch > 0n, "refund quota epoch has not started");
  const spent = coinAmount(refundUsage.amount);
  const cap = spent + 2_000_000n;
  async function consumption() {
    const current = await state();
    assert.equal(
      current.epoch,
      epoch,
      "refund cap coverage inconclusive: quota epoch changed during cap assertions",
    );
    return coinAmount(current.refundUsage.amount);
  }
  await setCap(cap);
  assert.equal(await consumption(), spent);
  for (const [i, { id }] of candidates.entries()) {
    const beforeBalance = await balance();
    assert.equal(await consumption(), i === 0 ? spent : cap);
    try {
      await finalize(id, i === 1 ? /epoch refund cap reached/ : undefined);
      assert.equal(
        (await balance()) - beforeBalance,
        i === 0 ? 2_000_000n : 0n,
      );
    } finally {
      await consumption();
    }
    assert.equal(await consumption(), cap);
  }
  return { refunds: candidates.map(({ id }) => id), epoch };
}

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
    const receipt = await relay.akash.transfer(responseId, tx.height);
    assert.ok(receipt?.settlement);
    assert.equal(receipt.status, Status.STATUS_TRANSFER_PENDING);
    const record = decodeRecord(receipt);
    assert.equal(record.id, id);
    assert.equal(record.amount, amount);
    assert.equal(record.height, BigInt(tx.height));
    assert.ok(record.recipient.equals(address));
    assert.equal(sinkIdParts(id).owner, sender.address);
    return id;
  }
  async function relayDeposit(id: SinkId) {
    const record = decodeRecord((await relay.akash.snapshot(id)).transfer);
    while ((await relay.akash.state()).height <= Number(record.height) + 1)
      await sleep(500);
    const target = (await relay.akash.state()).height - 1;
    await relay.update(target);
    await relay.relay(id, target);
    const status = await relay.program.account.transferStatus.fetch(
      statusAddress(PROGRAM_ID, id),
    );
    while (
      (await relay.wallet.connection.getSlot("finalized")) <
      status.resolvedAtSlot.toNumber()
    )
      await sleep(300);
  }
  async function report(index: number, id: SinkId) {
    const signer = await cosmosWallet(`reporter${index}`);
    const address = (await signer.getAccounts())[0].address;
    const snapshot = await relay.akash.snapshot(id);
    const own = snapshot.transfer.settlement?.votes.find(
      (v) => v.reporter === address,
    )?.observation;
    const observed = await observer.observe(
      decodeRecord(snapshot.transfer),
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
  async function settleMint(id: SinkId) {
    for (let i = 0; i < 3; i++) await report(i, id);
    assert.equal(
      (await relay.akash.snapshot(id)).transfer.status,
      Status.STATUS_MINTED,
    );
    const receipt = await relay.akash.transfer(
      id,
      (await relay.akash.state()).height,
    );
    assert.equal(receipt?.status, Status.STATUS_MINTED);
  }
  async function falseVoid(index: number, id: SinkId) {
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
              id: sinkIdParts(id),
              policyVersion: snapshot.policy.version,
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
  async function finalize(id: SinkId, expectedError?: RegExp) {
    const signer = await cosmosSigner("reporter0");
    try {
      const send = () =>
        signer.client.signAndBroadcast(
          signer.address,
          [
            {
              typeUrl: "/akash.sink.v1.MsgFinalizeRefund",
              value: { executor: signer.address, id: sinkIdParts(id) },
            },
          ],
          fee,
        );
      if (expectedError) {
        await expectRefundError(send, expectedError);
      } else {
        const tx = await send();
        assert.equal(tx.code, 0, tx.rawLog);
        const receipt = await relay.akash.transfer(id, tx.height);
        assert.equal(receipt?.status, Status.STATUS_REFUNDED);
      }
    } finally {
      signer.client.disconnect();
    }
  }
  async function resolve(
    id: SinkId,
    decision: Decision,
    observation?: Observation,
  ) {
    const before = (await relay.akash.snapshot(id)).transfer.settlement;
    assert.ok(before);
    const common = {
      authority: governanceAddress,
      id: sinkIdParts(id),
      expectedRound: before.round,
      reason: "Reconciled against this local Solana validator",
    };
    const message =
      decision === Decision.DECISION_RESTART
        ? {
            typeUrl: "/akash.sink.v1.MsgRestartTransfer",
            value: MsgRestartTransfer.encode(common).finish(),
          }
        : decision === Decision.DECISION_CONFIRM_MINT
          ? {
              typeUrl: "/akash.sink.v1.MsgConfirmMint",
              value: MsgConfirmMint.encode({
                ...common,
                evidence: observation && {
                  slot: observation.slot,
                  transactionSignature: observation.transactionSignature,
                },
              }).finish(),
            }
          : {
              typeUrl: "/akash.sink.v1.MsgConfirmVoid",
              value: MsgConfirmVoid.encode({
                ...common,
                evidence: observation && {
                  slot: observation.slot,
                  transactionSignature: observation.transactionSignature,
                },
              }).finish(),
            };
    return govern(
      "Resolve local sink reporter dispute",
      [message],
      async () =>
        (await relay.akash.snapshot(id)).transfer.settlement?.round ===
        before.round + 1n,
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
          coinAmount(live.maxAmountPerTransfer) ===
            coinAmount(value.maxAmountPerTransfer) &&
          coinAmount(live.maxEpochRefund) === coinAmount(value.maxEpochRefund)
        );
      },
    );
  }
  try {
    assert.equal(
      (await relay.akash.state()).totals.pendingCount,
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
      (await relay.akash.snapshot(offline)).transfer.status,
      Status.STATUS_TRANSFER_PENDING,
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
      (await relay.akash.snapshot(offline)).transfer.status,
      Status.STATUS_TRANSFER_PENDING,
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
      (await relay.akash.snapshot(vetoed)).transfer.status,
      Status.STATUS_REFUND_PENDING,
    );
    await report(3, vetoed); // The independent fourth reporter sees no Solana status and vetoes.
    assert.equal(
      (await relay.akash.snapshot(vetoed)).transfer.settlement?.hold,
      Hold.HOLD_VETO,
    );
    const vetoedReceipt = await relay.akash.transfer(
      vetoed,
      (await relay.akash.state()).height,
    );
    assert.equal(vetoedReceipt?.status, Status.STATUS_REFUND_PENDING);
    assert.equal(vetoedReceipt.settlement?.hold, Hold.HOLD_VETO);
    const balanceBeforeVeto = await sender.client.getBalance(
      sender.address,
      "uakt",
    );
    await finalize(vetoed, /settlement requires governance resolution/);
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
      id: vetoed,
      refunded: false,
      recoveredThroughGovernance: true,
    });

    await falseVoid(0, offline);
    assert.equal(
      (await relay.akash.snapshot(offline)).transfer.settlement?.hold,
      Hold.HOLD_CONFLICT,
    );
    await finalize(
      offline,
      /refund needs current authorization without a hold: invalid settlement/,
    );
    const actual = await observer.observe(
      decodeRecord((await relay.akash.snapshot(offline)).transfer),
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

    changedParams = true;
    await params(
      Params.fromPartial({
        ...original,
        maxAmountPerTransfer: { denom: "uakt", amount: "2000000" },
      }),
    );
    const tokenAccount = await createAccount(
      relay.wallet.connection,
      payer,
      mint,
      relay.wallet.publicKey,
      web3.Keypair.generate(),
    );
    const { refunds, epoch } = await refundCapScenario({
      state: () => relay.akash.state(),
      setCap: async (amount) => {
        await params(
          Params.fromPartial({
            ...original,
            maxAmountPerTransfer: { denom: "uakt", amount: "2000000" },
            maxEpochRefund: { denom: "uakt", amount: amount.toString() },
          }),
        );
      },
      prepare: async () => {
        const id = await deposit(2_000_000n, tokenAccount);
        await relayDeposit(id);
        for (let r = 0; r < 3; r++) await report(r, id);
        const until = (await relay.akash.snapshot(id)).transfer.settlement
          ?.challengeEnd;
        assert.ok(until);
        const challenged = await relay.akash.transfer(
          id,
          (await relay.akash.state()).height,
        );
        assert.equal(challenged?.status, Status.STATUS_REFUND_PENDING);
        assert.ok(challenged.settlement?.challengeEnd === until);
        await finalize(id, /challenge window has not elapsed/);
        return { id, challengeEnd: until };
      },
      finalize,
      balance: async () =>
        BigInt((await sender.client.getBalance(sender.address, "uakt")).amount),
      sleep,
    });
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
      epoch: epoch.toString(),
      ids: refunds.map(String),
      recoveredAfterGovernance: true,
      duplicateRefundPaidNothing: true,
    });

    const unfunded = await deposit(1_000_000n);
    const native = decodeRecord(
      (await relay.akash.snapshot(unfunded)).transfer,
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
        /insufficient|debit|credit|lamports|proof-create: simulation failed[\s\S]*\bAccountNotFound\b/i,
      );
      assert.equal(
        await relay.wallet.connection.getAccountInfo(
          statusAddress(PROGRAM_ID, unfunded),
        ),
        null,
      );
      assert.equal(
        (await relay.akash.snapshot(unfunded)).transfer.status,
        Status.STATUS_TRANSFER_PENDING,
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
      statusAddress(PROGRAM_ID, unfunded),
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
if (require.main === module)
  void main().catch((error) => {
    console.error(error);
    process.exitCode = 1;
  });
