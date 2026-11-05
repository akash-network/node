import {
  Hold,
  MsgRequestVoid,
  Sink_Status as Status,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { web3 } from "@coral-xyz/anchor";
import { toBech32 } from "@cosmjs/encoding";
import {
  getAccount,
  getAssociatedTokenAddressSync,
  getMint,
} from "@solana/spl-token";
import { VoteOption } from "cosmjs-types/cosmos/gov/v1/gov";
import {
  MsgSubmitProposal,
  MsgSubmitProposalResponse,
  MsgVote,
} from "cosmjs-types/cosmos/gov/v1/tx";
import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { join } from "node:path";
import { promisify } from "node:util";
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
  PROFILE,
  PROGRAM_ID,
  programData,
  REPO,
  ROOT,
  save,
  sleep,
} from "./common";
import { reconcile } from "./reconcile";
import { relayContext } from "./relay";
import { processes, stop } from "./stack";
import { comet, startValidator, validatorName } from "./testnetify";

const exec = promisify(execFile);
async function main() {
  assert.equal(
    PROFILE,
    "testnetify",
    "fault tests require the isolated testnetify profile",
  );
  const node = comet();
  const relay = await relayContext();
  const sender = await cosmosSigner("sender");
  const results: object[] = [];
  const errors: unknown[] = [];
  const local = object(await json(join(ROOT, "initialized.json")));
  assert.ok(typeof local.sinkAddress === "string");
  const sink = local.sinkAddress;
  const mint = pda("mint");
  const admin = {
    admin: {
      program: PROGRAM_ID,
      programData: programData(PROGRAM_ID),
      authority: relay.wallet.publicKey,
    },
    config: pda("config"),
    bridge: pda("bridge"),
  };
  const enabled = (value: boolean) =>
    relay.program.methods.setBridgeEnabled(value).accountsStrict(admin).rpc();
  const height = async () => (await node.status()).syncInfo.latestBlockHeight;
  async function waitHeight(target: number) {
    const deadline = Date.now() + 60_000;
    while ((await height()) < target) {
      assert.ok(Date.now() < deadline, `chain did not reach height ${target}`);
      await sleep(500);
    }
  }
  async function stopService(name: string) {
    const entry = (await processes()).find((entry) => entry.name === name);
    assert.ok(entry, `unknown service ${name}`);
    await stop(entry);
  }
  async function deposit(amount: bigint) {
    const recipient = web3.Keypair.generate();
    await save(
      join(ROOT, `scenario-${recipient.publicKey.toBase58()}-keypair.json`),
      Array.from(recipient.secretKey),
    );
    const tx = await sender.client.signAndBroadcast(
      sender.address,
      [
        sinkMessage({
          sender: sender.address,
          recipient: recipient.publicKey.toBase58(),
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
    assert.ok(record.recipient.equals(recipient.publicKey));
    assert.equal(sinkIdParts(id).owner, sender.address);
    return { id, recipient, tx };
  }
  async function settlement(id: SinkId, status: Status) {
    const deadline = Date.now() + 150_000;
    while (true) {
      const snapshot = await relay.akash.snapshot(id);
      assert.ok(snapshot.transfer.settlement);
      assert.equal(snapshot.transfer.settlement.hold, Hold.HOLD_NONE);
      if (snapshot.transfer.status === status) {
        const state = await relay.akash.state();
        const receipt = await relay.akash.transfer(id, state.height);
        assert.equal(receipt?.status, status);
        assert.deepEqual(receipt.data, snapshot.transfer.data);
        return snapshot;
      }
      assert.ok(
        Date.now() < deadline,
        `transfer ${id} did not reach ${Status[status]}`,
      );
      await sleep(1000);
    }
  }
  async function record(value: object) {
    results.push(value);
    console.log(JSON.stringify(value));
    await save(join(ROOT, "scenarios-progress.json"), results);
  }
  try {
    assert.equal((await node.validatorsAll()).validators.length, 4);
    assert.equal(
      (await relay.akash.state()).totals.pendingCount,
      0n,
      "finish existing transfers before fault injection",
    );
    await stopService("relayer");

    await stopService(validatorName(3));
    const beforeOutage = await height();
    await waitHeight(beforeOutage + 4);
    const target = (await height()) - 1;
    const response = await fetch(`${AKASH_RPC}/commit?height=${target}`, {
      signal: AbortSignal.timeout(15_000),
    });
    const commit = object(
      object(object(object(await response.json()).result).signed_header).commit,
    );
    assert.ok(Array.isArray(commit.signatures));
    assert.equal(commit.signatures.length, 4);
    assert.equal(
      commit.signatures.filter((s: unknown) => object(s).block_id_flag === 1)
        .length,
      1,
    );
    await startValidator(3);
    await assert.rejects(
      () => relay.update(target, true),
      /UpdateClientFailed/,
    );
    await relay.update(target);
    await record({
      scenario: "one-validator-offline",
      passed: true,
      height: target,
      commitEntries: 4,
      signatures: 3,
      alteredHeaderRejected: true,
    });
    await waitHeight((await height()) + 3);

    await stopService(validatorName(2));
    await stopService(validatorName(3));
    await sleep(3000); // Let any block already committed before shutdown finish.
    const stalled = await height();
    await sleep(5000);
    assert.equal(
      await height(),
      stalled,
      "half of the voting power advanced consensus",
    );
    await startValidator(2);
    await startValidator(3);
    await waitHeight(stalled + 3);
    await record({
      scenario: "two-validators-offline",
      passed: true,
      stalledHeight: stalled,
      resumedHeight: await height(),
    });

    const supplyBefore = (await getMint(relay.wallet.connection, mint)).supply;
    await enabled(false);
    const paused = await deposit(2_000_000n);
    await waitHeight(paused.tx.height + 3);
    await relay.tick();
    assert.equal(
      await relay.program.account.transferStatus.fetchNullable(
        statusAddress(PROGRAM_ID, paused.id),
      ),
      null,
    );
    assert.equal(
      (await relay.akash.snapshot(paused.id)).transfer.status,
      Status.STATUS_TRANSFER_PENDING,
    );
    assert.equal(
      (await getMint(relay.wallet.connection, mint)).supply,
      supplyBefore,
    );
    await enabled(true);
    await relay.tick();
    await settlement(paused.id, Status.STATUS_MINTED);
    assert.equal(
      (
        await getAccount(
          relay.wallet.connection,
          getAssociatedTokenAddressSync(mint, paused.recipient.publicKey),
          "finalized",
        )
      ).amount,
      2_000_000n,
    );
    await relay.tick();
    assert.equal(
      (await getMint(relay.wallet.connection, mint, "finalized")).supply,
      supplyBefore + 2_000_000n,
    );
    await record({
      scenario: "pause-retry-and-replay",
      passed: true,
      id: paused.id,
      mintedUakt: "2000000",
      deposit: paused.tx.transactionHash,
    });

    await enabled(false);
    const governed = await deposit(5_000_000n);
    sender.client.registry.register(
      "/akash.sink.v1.MsgRequestVoid",
      MsgRequestVoid,
    );
    const expectedRound = (await relay.akash.snapshot(governed.id)).transfer
      .settlement?.round;
    assert.ok(expectedRound);
    const forbidden = await sender.client.signAndBroadcast(
      sender.address,
      [
        {
          typeUrl: "/akash.sink.v1.MsgRequestVoid",
          value: {
            authority: sender.address,
            id: sinkIdParts(governed.id),
            expectedRound,
          },
        },
      ],
      fee,
    );
    assert.notEqual(
      forbidden.code,
      0,
      "ordinary sender authorized a governance void",
    );
    assert.equal(
      (await relay.akash.snapshot(governed.id)).transfer.settlement
        ?.voidAuthorization,
      undefined,
    );
    const balanceAfterDeposit = BigInt(
      (await sender.client.getBalance(sender.address, "uakt")).amount,
    );
    const proposer = await cosmosSigner("validator0");
    let proposalId: bigint;
    try {
      const authority = toBech32(
        "akash",
        createHash("sha256").update("gov").digest().subarray(0, 20),
      );
      const tx = await proposer.client.signAndBroadcast(
        proposer.address,
        [
          {
            typeUrl: "/cosmos.gov.v1.MsgSubmitProposal",
            value: MsgSubmitProposal.fromPartial({
              proposer: proposer.address,
              title: "Local sink void rehearsal",
              summary:
                "Prove the governance order on Solana before refunding the local deposit.",
              initialDeposit: [{ denom: "uakt", amount: "100000000" }],
              messages: [
                {
                  typeUrl: "/akash.sink.v1.MsgRequestVoid",
                  value: MsgRequestVoid.encode(
                    MsgRequestVoid.fromPartial({
                      authority,
                      expectedRound,
                      id: sinkIdParts(governed.id),
                    }),
                  ).finish(),
                },
              ],
            }),
          },
        ],
        { amount: [{ denom: "uakt", amount: "50000" }], gas: "2000000" },
      );
      assert.equal(tx.code, 0, tx.rawLog);
      const response = tx.msgResponses.find(
        (m) => m.typeUrl === "/cosmos.gov.v1.MsgSubmitProposalResponse",
      );
      assert.ok(response);
      proposalId = MsgSubmitProposalResponse.decode(response.value).proposalId;
    } finally {
      proposer.client.disconnect();
    }
    // Each validator uses its own funded operator key; no governance bypass.
    for (let i = 0; i < 4; i++) {
      const voter = await cosmosSigner(`validator${i}`);
      try {
        const tx = await voter.client.signAndBroadcast(
          voter.address,
          [
            {
              typeUrl: "/cosmos.gov.v1.MsgVote",
              value: MsgVote.fromPartial({
                proposalId,
                voter: voter.address,
                option: VoteOption.VOTE_OPTION_YES,
              }),
            },
          ],
          fee,
        );
        assert.equal(tx.code, 0, tx.rawLog);
      } finally {
        voter.client.disconnect();
      }
    }
    const deadline = Date.now() + 45_000;
    while (
      !(await relay.akash.snapshot(governed.id)).transfer.settlement
        ?.voidAuthorization
    ) {
      assert.ok(
        Date.now() < deadline,
        `governance proposal ${proposalId} did not create a void order`,
      );
      await sleep(500);
    }
    const ordered = await relay.akash.transfer(
      governed.id,
      (await relay.akash.state()).height,
    );
    assert.ok(ordered?.settlement?.voidAuthorization);
    assert.equal(ordered.status, Status.STATUS_VOIDED);
    await waitHeight((await height()) + 3);
    // A proven governance void is allowed while minting is paused.
    await relay.tick();
    await settlement(governed.id, Status.STATUS_REFUNDED);
    const status = await relay.program.account.transferStatus.fetch(
      statusAddress(PROGRAM_ID, governed.id),
    );
    assert.ok(status.outcome.voided?.reason.governance);
    assert.equal(
      BigInt((await sender.client.getBalance(sender.address, "uakt")).amount) -
        balanceAfterDeposit,
      5_000_000n,
    );
    assert.equal(
      (await getMint(relay.wallet.connection, mint, "finalized")).supply,
      supplyBefore + 2_000_000n,
    );
    await record({
      scenario: "governance-void-and-refund",
      passed: true,
      id: governed.id,
      proposalId: proposalId.toString(),
      deposit: governed.tx.transactionHash,
      refundConfirmedByHeight: (await relay.akash.state()).height,
      refundedUakt: "5000000",
      ordinarySenderRejected: true,
    });

    const final = await relay.akash.state();
    const supply = (await getMint(relay.wallet.connection, mint, "finalized"))
      .supply;
    assert.equal(coinAmount(final.totals.pendingAmount), 0n);
    assert.equal(coinAmount(final.totals.voidedAmount), 0n);
    assert.equal(final.totals.voidedCount, 0n);
    assert.equal(coinAmount(final.totals.refundPendingAmount), 0n);
    const reconciliation = await reconcile();
    assert.equal(
      reconciliation.status,
      "healthy",
      JSON.stringify(reconciliation.alerts),
    );
    const burned = BigInt(reconciliation.cumulativeBurnedUakt);
    assert.equal(supply + burned, coinAmount(final.totals.mintedAmount));
    assert.equal(
      BigInt((await sender.client.getBalance(sink, "uakt")).amount),
      supply + burned,
    );
    await save(join(ROOT, "scenarios-result.json"), {
      completedAt: new Date().toISOString(),
      chainId: CHAIN,
      validators: 4,
      results,
      solanaSupplyUakt: supply.toString(),
      cumulativeBurnedUakt: burned.toString(),
      lockedUakt: (supply + burned).toString(),
    });
  } catch (error) {
    errors.push(error);
  } finally {
    for (const restore of [
      () => startValidator(2),
      () => startValidator(3),
      () => enabled(true),
      () => relay.close(),
      () => node.disconnect(),
      () => sender.client.disconnect(),
      async () => {
        const result = await exec(
          process.execPath,
          ["--import", "tsx", join(REPO, "local/stack.ts"), "up"],
          { timeout: 120_000, maxBuffer: 2 * 1024 * 1024 },
        );
        console.log(result.stdout.trim());
      },
    ]) {
      try {
        await restore();
      } catch (error) {
        errors.push(error);
      }
    }
  }
  if (errors.length === 1) throw errors[0];
  if (errors.length > 1)
    throw new AggregateError(errors, "Testnet scenarios or cleanup failed");
}
void main().catch((error: unknown) => {
  console.error(error);
  process.exitCode = 1;
});
