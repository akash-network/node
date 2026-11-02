import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { writeFileSync } from "node:fs";
import type { TestContext } from "node:test";
import { web3 } from "@coral-xyz/anchor";
import { DirectSecp256k1Wallet } from "@cosmjs/proto-signing";
import { SigningStargateClient } from "@cosmjs/stargate";
import { TxRaw } from "cosmjs-types/cosmos/tx/v1beta1/tx";
import { reporterRegistry } from "../reporter/akash";
import { planOutcome } from "../reporter/policy";
import {
  Outcome,
  QueryStateResponse,
  QueryTransferResponse,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { SolanaObserver, decodeRecord } from "../reporter/solana";

export async function testReporterReturn(
  t: TestContext,
  connection: web3.Connection,
  fixture: unknown,
) {
  assert.ok(
    typeof fixture === "object" &&
      fixture !== null &&
      "state_response" in fixture &&
      typeof fixture.state_response === "string" &&
      "transfer_response" in fixture &&
      typeof fixture.transfer_response === "string" &&
      "reporters" in fixture &&
      Array.isArray(fixture.reporters),
  );
  const state = QueryStateResponse.decode(
    Buffer.from(fixture.state_response, "hex"),
  );
  const transfer = QueryTransferResponse.decode(
    Buffer.from(fixture.transfer_response, "hex"),
  ).transfer;
  assert.ok(state.params && state.totals && transfer);
  const programId = new web3.PublicKey(
    "FfFAPUKqjf68ggj1HvpyzMHFDRnT2cFAdZYYQb7mK9i5",
  );
  const mint = web3.PublicKey.findProgramAddressSync(
    [Buffer.from("mint")],
    programId,
  )[0];
  const observer = new SolanaObserver(connection, {
    programId,
    mint,
    genesisHash: await connection.getGenesisHash(),
    lightClientProgram: new web3.PublicKey(
      "HqPcGpVHxNNFfVatjhG78dFVMwjyZixoKPdZSt3d3TdD",
    ),
    akashChainId: "akash-1",
    maxAgeSeconds: 60,
  });
  const record = decodeRecord(transfer.record);
  let observed = await observer.observe(record);
  const deadline = Date.now() + 30_000;
  while (
    observed.kind === "absent" &&
    Date.now() < deadline &&
    !t.signal.aborted
  ) {
    await new Promise((resolve) => setTimeout(resolve, 500));
    observed = await observer.observe(record);
  }
  assert.ok(
    observed.kind === "resolved",
    "mint did not reach finalized commitment",
  );
  assert.equal(observed.observation.outcome, Outcome.OUTCOME_MINTED);
  const transactions: string[] = [];
  // Reproduce the SDK's deterministic test keys. These seeds are public test
  // data and must never be used on any funded network.
  const order =
    0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141n;
  for (let i = 0; i < 3; i++) {
    const signer: unknown = fixture.reporters[i];
    assert.ok(
      typeof signer === "object" &&
        signer !== null &&
        "address" in signer &&
        typeof signer.address === "string" &&
        "account_number" in signer &&
        typeof signer.account_number === "string" &&
        "sequence" in signer &&
        typeof signer.sequence === "number",
    );
    const hash = createHash("sha256")
      .update(`local reporter ${i}`)
      .digest("hex");
    const key = ((BigInt(`0x${hash}`) % (order - 1n)) + 1n)
      .toString(16)
      .padStart(64, "0");
    const wallet = await DirectSecp256k1Wallet.fromKey(
      Buffer.from(key, "hex"),
      "akash",
    );
    assert.equal((await wallet.getAccounts())[0]?.address, signer.address);
    const plan = planOutcome(
      signer.address,
      {
        params: state.params,
        totals: state.totals,
        transfer,
        time: 1_750_000_002n,
      },
      observed,
    );
    assert.equal(plan.messages.length, 1);
    assert.equal(plan.messages[0].typeUrl, "/akash.sink.v1.MsgReportOutcome");
    const client = await SigningStargateClient.offline(wallet, {
      registry: reporterRegistry(),
    });
    const tx = await client.sign(
      signer.address,
      plan.messages,
      { amount: [{ denom: "uakt", amount: "10" }], gas: "500000" },
      "local finalized observation",
      {
        chainId: "akash-1",
        accountNumber: BigInt(signer.account_number),
        sequence: signer.sequence,
      },
    );
    transactions.push(
      Buffer.from(TxRaw.encode(tx).finish()).toString("base64"),
    );
    client.disconnect();
  }
  const path = process.env.SINK_REPORTER_FIXTURE;
  assert.ok(path, "test:sink must provide SINK_REPORTER_FIXTURE");
  writeFileSync(
    path,
    JSON.stringify(
      {
        transactions,
        slot: Number(observed.observation.slot),
        signature: Buffer.from(
          observed.observation.transactionSignature,
        ).toString("base64"),
      },
      null,
      2,
    ),
  );
  t.diagnostic(
    `finalized Solana slot ${observed.observation.slot}; three SIGN_MODE_DIRECT reporter transactions exported`,
  );
}
