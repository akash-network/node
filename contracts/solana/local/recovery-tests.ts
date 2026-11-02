import assert from "node:assert/strict";
import { join } from "node:path";
import { readdir } from "node:fs/promises";
import { AnchorProvider, BN, Program, Wallet, web3 } from "@coral-xyz/anchor";
import clientIdl from "../idls/ics07_tendermint.json";
import accessIdl from "../idls/access_manager.json";
import type { AccessManager } from "../.deps/ibc-contracts/ibc-solana/target/types/access_manager";
import type { Ics07Tendermint } from "../.deps/ibc-contracts/ibc-solana/target/types/ics07_tendermint";
import { decodePacket, uploadHeader } from "./header-upload";
import { reclaimLookups } from "./lookup-cleanup";
import { CLIENT_ID, ACCESS_ID, pda, object, sleep } from "./common";
import { optionalJson } from "./durable";
export async function testRecovery(
  wallet: AnchorProvider,
  root: string,
  fixture: (
    target: number,
    trusted: number,
  ) => Promise<ReturnType<typeof decodePacket>>,
) {
  const journalDirectory = join(root, "recovery");
  const packet = await fixture(3, 1);
  const stages = [
    { stage: "lookup-extend", event: "submitted" },
    { stage: "signature", event: "transaction" },
    { stage: "chunk", event: "transaction" },
    { stage: "assembly", event: "transaction" },
    { stage: "cleanup", event: "transaction" },
    { stage: "lookup-deactivate", event: "transaction" },
  ];
  const results: object[] = [];
  for (const fault of stages) {
    let interrupted = false;
    await assert.rejects(
      () =>
        uploadHeader({
          wallet,
          packet,
          target: 3,
          trusted: 1,
          journalDirectory,
          record: async (value) => {
            const row = object(value);
            if (row.event === fault.event && row.stage === fault.stage) {
              interrupted = true;
              throw new Error(`injected interruption at ${fault.stage}`);
            }
          },
        }),
      /injected interruption/,
    );
    assert.equal(interrupted, true);
    results.push({ ...fault, passed: true });
    console.log(JSON.stringify({ recovery: fault.stage, passed: true }));
  }
  const complete = await uploadHeader({
    wallet,
    packet,
    target: 3,
    trusted: 1,
    journalDirectory,
    beforeUpload: async () => {
      throw new Error("expired: no new verification permitted");
    },
    record: async () => {},
  });
  assert.equal(
    complete.assembly.error,
    null,
    "completed job must clean up without reopening expired trust",
  );
  const bad = await fixture(5, 3);
  const offset = bad.bytes.indexOf(Buffer.from(bad.consensus.root));
  assert.ok(offset >= 0);
  bad.bytes[offset] ^= 1;
  await assert.rejects(
    () =>
      uploadHeader({
        wallet,
        packet: bad,
        target: 5,
        trusted: 3,
        journalDirectory,
        record: async (value) => {
          const row = object(value);
          if (row.stage === "lookup-deactivate" && row.event === "transaction")
            throw new Error("rejected cleanup interrupted");
        },
      }),
    /rejected cleanup interrupted/,
  );
  const rejected = await uploadHeader({
    wallet,
    packet: bad,
    target: 5,
    trusted: 3,
    journalDirectory,
    beforeUpload: async () => {
      throw new Error("expired");
    },
    record: async () => {},
  });
  assert.notEqual(rejected.assembly.error, null);
  const client = new Program<Ics07Tendermint>(clientIdl, wallet);
  assert.equal(
    (
      await client.account.clientState.fetch(pda("client", CLIENT_ID))
    ).latestHeight.revisionHeight.toNumber(),
    3,
  );
  results.push({ stage: "rejected-job-cleanup", passed: true });
  // A second authorized operator authenticates this exact header after the
  // first has uploaded chunks, then the first operator loses its update role.
  // Its assembly rejects, but the authenticated target still completes its job.
  const otherKey = web3.Keypair.generate();
  await wallet.sendAndConfirm(
    new web3.Transaction().add(
      web3.SystemProgram.transfer({
        fromPubkey: wallet.publicKey,
        toPubkey: otherKey.publicKey,
        lamports: web3.LAMPORTS_PER_SOL,
      }),
    ),
  );
  const access = new Program<AccessManager>(accessIdl, wallet);
  await access.methods
    .grantRole(new BN(1), otherKey.publicKey)
    .accountsStrict({
      accessManager: pda("access_manager", ACCESS_ID),
      admin: wallet.publicKey,
      instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
    })
    .rpc();
  const other = new AnchorProvider(
    wallet.connection,
    new Wallet(otherKey),
    wallet.opts,
  );
  const competing = await fixture(7, 3);
  let raced = false;
  let chunksUploaded = 0;
  let rejectedAssembly = false;
  let normalized = false;
  const racedResult = await uploadHeader({
    wallet,
    packet: competing,
    target: 7,
    trusted: 3,
    journalDirectory,
    record: async (value) => {
      const row = object(value);
      if (row.event === "transaction" && row.stage === "chunk")
        chunksUploaded++;
      if (
        !raced &&
        chunksUploaded === Math.ceil(competing.bytes.length / 900)
      ) {
        raced = true;
        const winner = await uploadHeader({
          wallet: other,
          packet: competing,
          target: 7,
          trusted: 3,
          journalDirectory: join(root, "other-operator"),
          record: async () => {},
        });
        assert.equal(winner.assembly.error, null);
        await access.methods
          .revokeRole(new BN(1), wallet.publicKey)
          .accountsStrict({
            accessManager: pda("access_manager", ACCESS_ID),
            admin: wallet.publicKey,
            instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
          })
          .rpc();
      }
      if (
        row.event === "transaction" &&
        row.stage === "assembly" &&
        row.error !== null
      )
        rejectedAssembly = true;
      if (row.event === "authenticated-concurrently") normalized = true;
    },
  });
  assert.ok(raced && rejectedAssembly && normalized);
  assert.equal(racedResult.assembly.error, null);
  const racedRetry = await uploadHeader({
    wallet,
    packet: competing,
    target: 7,
    trusted: 3,
    journalDirectory,
    record: async () => {},
  });
  assert.equal(racedRetry.assembly.error, null);
  results.push({ stage: "concurrent-operator-and-restart", passed: true });
  console.log(
    "Interrupted uploads and rejected-job cleanup passed; waiting for actual lookup cooldown.",
  );
  const deadline = Date.now() + 300000;
  for (;;) {
    await reclaimLookups(wallet, journalDirectory);
    await reclaimLookups(other, join(root, "other-operator"));
    const paths = (await readdir(journalDirectory)).filter((name) =>
      /^[0-9a-f]{64}\.json$/.test(name),
    );
    const states = await Promise.all(
      paths.map(async (name) =>
        object(await optionalJson(join(journalDirectory, name))),
      ),
    );
    const otherStates = await Promise.all(
      (await readdir(join(root, "other-operator")))
        .filter((name) => /^[0-9a-f]{64}\.json$/.test(name))
        .map(async (name) =>
          object(await optionalJson(join(root, "other-operator", name))),
        ),
    );
    if (
      states.length === 3 &&
      otherStates.length === 1 &&
      [...states, ...otherStates].every((s) => s.lookupClosed === true)
    )
      break;
    assert.ok(
      Date.now() < deadline,
      "lookup rent not reclaimed after cooldown",
    );
    await sleep(1000);
  }
  results.push({ stage: "lookup-rent-returned", passed: true });
  return { passed: true, results };
}
