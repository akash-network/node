import assert from "node:assert/strict";
import { join } from "node:path";
import { web3 } from "@coral-xyz/anchor";
import { getMint } from "@solana/spl-token";
import { StargateClient } from "@cosmjs/stargate";
import { AkashClient } from "../reporter/akash";
import { parseConfig } from "../reporter/config";
import { planOutcome, reportedObservations } from "../reporter/policy";
import { SolanaObserver, decodeRecord } from "../reporter/solana";
import {
  ROOT,
  PROFILE,
  CHAIN,
  AKASH_RPC,
  SOLANA_RPC,
  json,
  object,
  pda,
  provider,
  save,
} from "./common";
import { comet } from "./testnetify";
import { processes, running } from "./stack";
import { reconcile } from "./reconcile";
import { assertClientActive } from "./client-status";
import {
  clientProgram,
  discoverLightClient,
  readRelayerAccess,
} from "./light-client";

async function main() {
  assert.equal(PROFILE, "testnetify");
  const akash = new AkashClient(AKASH_RPC, CHAIN, 60);
  const nodes = [comet(0), comet(1), comet(2), comet(3)];
  const solana = new web3.Connection(SOLANA_RPC, "finalized");
  try {
    const services = (await processes()).filter(
      (entry) => entry.name !== "testnetify",
    );
    assert.equal(services.length, 10);
    for (const entry of services)
      assert.ok(await running(entry), `${entry.name} stopped`);
    const statuses = await Promise.all(nodes.map((node) => node.status()));
    for (const status of statuses) {
      assert.equal(status.nodeInfo.network, CHAIN);
      assert.equal(status.syncInfo.catchingUp, false);
      assert.ok(
        Date.now() - status.syncInfo.latestBlockTime.getTime() < 60_000,
      );
    }
    const heights = statuses.map((status) => status.syncInfo.latestBlockHeight);
    const height = Math.min(...heights);
    assert.ok(
      Math.max(...heights) - height <= 2,
      "validator lag exceeds two blocks",
    );
    const blocks = await Promise.all(nodes.map((node) => node.block(height)));
    const blockHash = Buffer.from(blocks[0].blockId.hash).toString("hex");
    for (const block of blocks)
      assert.equal(
        Buffer.from(block.blockId.hash).toString("hex"),
        blockHash,
        "validators disagree on committed block",
      );
    const sets = await Promise.all(
      nodes.map((node) => node.validatorsAll(height)),
    );
    for (const set of sets) {
      assert.equal(set.validators.length, 4);
      for (const validator of set.validators)
        assert.equal(validator.votingPower, 10_000n);
    }
    const state = await akash.state();
    assert.equal(state.totals.pendingCount.toBigInt(), 0n);
    assert.equal(state.totals.pendingAmount.toBigInt(), 0n);
    assert.equal(state.totals.voidedAmount.toBigInt(), 0n);
    // The client the bridge pins now, not the one genesis deployed.
    const wallet = await provider();
    const pinned = await discoverLightClient(wallet.connection);
    assert.equal(pinned.chainId, CHAIN, "bridge accepts another Akash chain");
    await assertClientActive(
      wallet,
      clientProgram(wallet, pinned.program),
      CHAIN,
    );
    const relayerAccess = await readRelayerAccess(wallet, pinned);
    assert.notEqual(
      relayerAccess,
      "none",
      "relayer payer may not update the light client",
    );
    const mint = await getMint(solana, pda("mint"), "finalized");
    const reconciliation = await reconcile();
    assert.equal(
      reconciliation.status,
      "healthy",
      JSON.stringify(reconciliation.alerts),
    );
    const burns = BigInt(reconciliation.cumulativeBurnedUakt);
    assert.equal(mint.supply + burns, state.totals.mintedAmount.toBigInt());
    const local = object(await json(join(ROOT, "initialized.json")));
    assert.ok(typeof local.sinkAddress === "string");
    const bank = await StargateClient.create(nodes[0]);
    const locked = BigInt(
      (await bank.getBalance(local.sinkAddress, "uakt")).amount,
    );
    assert.equal(locked, mint.supply + burns);
    const reporters = [];
    for (let i = 0; i < 3; i++) {
      const config = parseConfig(await json(join(ROOT, `reporter${i}.json`)));
      const observer = new SolanaObserver(solana, config);
      for (let id = 1n; id <= state.totals.lastTransferId.toBigInt(); id++) {
        const transfer = await akash.transfer(id, state.height);
        const observation = await observer.observe(
          decodeRecord(transfer.record),
          reportedObservations(transfer),
          transfer.settlement?.votes.find(
            (vote) => vote.reporter === config.reporter,
          )?.observation,
        );
        const plan = planOutcome(
          config.reporter,
          { ...state, transfer },
          observation,
        );
        assert.equal(
          plan.messages.length,
          0,
          `reporter ${i} would send a duplicate or unfinished message`,
        );
        assert.equal(
          plan.alerts.length,
          0,
          `reporter ${i} found a contradictory outcome`,
        );
      }
      reporters.push({
        reporter: config.reporter,
        scanned: state.totals.lastTransferId.toString(),
        failures: 0,
      });
    }
    const result = {
      checkedAt: new Date().toISOString(),
      chainId: CHAIN,
      height,
      blockHash,
      validators: 4,
      runningServices: services.map((entry) => entry.name),
      reporters,
      solanaGenesisHash: await solana.getGenesisHash(),
      lightClientProgram: pinned.program.toBase58(),
      accessManagerProgram: pinned.accessManager.toBase58(),
      relayerAccess,
      solanaSupplyUakt: mint.supply.toString(),
      cumulativeBurnedUakt: burns.toString(),
      lockedUakt: locked.toString(),
      mintedUakt: state.totals.mintedAmount.toString(),
      refundedUakt: state.totals.refundedAmount.toString(),
    };
    await save(join(ROOT, "health-result.json"), result);
    console.log(JSON.stringify(result, null, 2));
  } finally {
    akash.close();
    for (const node of nodes) node.disconnect();
  }
}
void main().catch((error: unknown) => {
  console.error(error);
  process.exitCode = 1;
});
