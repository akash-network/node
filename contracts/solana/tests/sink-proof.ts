import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import type { TestContext } from "node:test";
import { BN, Program, web3 } from "@coral-xyz/anchor";
import {
  ASSOCIATED_TOKEN_PROGRAM_ID,
  TOKEN_PROGRAM_ID,
  getAccount,
  getAssociatedTokenAddressSync,
  getMint,
} from "@solana/spl-token";
import idl from "../target/idl/akt_solana.json";
import clientIdl from "../idls/ics07_tendermint.json";
import accessIdl from "../idls/access_manager.json";
import type { AktSolana } from "../target/types/akt_solana";
import type { Ics07Tendermint } from "../.deps/ibc-contracts/ibc-solana/target/types/ics07_tendermint";
import type { AccessManager } from "../.deps/ibc-contracts/ibc-solana/target/types/access_manager";
import { testProvider } from "./helpers";
import { testReporterReturn } from "./reporter";

// Runs on its own fresh validator after the shared genesis tests. The input
// comes from a signed MsgSink executed and committed by the actual Akash app.
export async function runSinkProofTests(t: TestContext, path: string) {
  const fixture: unknown = JSON.parse(readFileSync(path, "utf8"));
  assert.ok(
    typeof fixture === "object" &&
      fixture !== null &&
      "chain_id" in fixture &&
      typeof fixture.chain_id === "string" &&
      "header_height" in fixture &&
      typeof fixture.header_height === "number" &&
      "query_height" in fixture &&
      typeof fixture.query_height === "number" &&
      "root" in fixture &&
      typeof fixture.root === "string" &&
      "record" in fixture &&
      typeof fixture.record === "string" &&
      "proof" in fixture &&
      typeof fixture.proof === "string",
  );
  function hex(value: string) {
    assert.match(value, /^(?:[0-9a-f]{2})+$/);
    return Buffer.from(value, "hex");
  }
  const root = hex(fixture.root);
  const raw = hex(fixture.record);
  const proof = hex(fixture.proof);
  assert.equal(root.length, 32);
  assert.equal(raw.length, 77);
  assert.equal(raw[0], 1);
  assert.ok(Number.isSafeInteger(fixture.header_height));
  assert.equal(fixture.header_height, fixture.query_height + 1);
  const height = new BN(fixture.header_height);
  const le64 = (n: BN) => n.toArrayLike(Buffer, "le", 8);
  const record = {
    version: raw[0],
    id: new BN(raw.subarray(1, 9), "le"),
    sender: Array.from(raw.subarray(9, 29)),
    recipient: new web3.PublicKey(raw.subarray(29, 61)),
    amount: new BN(raw.subarray(61, 69), "le"),
    height: new BN(raw.subarray(69, 77), "le"),
  };
  const provider = testProvider();
  const program = new Program<AktSolana>(idl, provider);
  const client = new Program<Ics07Tendermint>(clientIdl, provider);
  const access = new Program<AccessManager>(accessIdl, provider);
  const pda = (seed: string, owner = program.programId) =>
    web3.PublicKey.findProgramAddressSync([Buffer.from(seed)], owner)[0];
  const programData = (owner: web3.PublicKey) =>
    web3.PublicKey.findProgramAddressSync(
      [owner.toBuffer()],
      new web3.PublicKey("BPFLoaderUpgradeab1e11111111111111111111111"),
    )[0];
  const accessManager = pda("access_manager", access.programId);
  await access.methods
    .initialize(provider.publicKey)
    .accountsStrict({
      accessManager,
      payer: provider.publicKey,
      systemProgram: web3.SystemProgram.programId,
      instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
      programData: programData(access.programId),
      authority: provider.publicKey,
    })
    .rpc();
  await access.methods
    .grantRole(new BN(1), provider.publicKey)
    .accountsStrict({
      accessManager,
      admin: provider.publicKey,
      instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
    })
    .rpc();
  const clientState = pda("client", client.programId);
  const consensus = web3.PublicKey.findProgramAddressSync(
    [Buffer.from("consensus_state"), le64(height)],
    client.programId,
  )[0];
  // Deliberately trusted test checkpoint. This tests app proofs and minting;
  // it does not authenticate an Akash header or test validator signatures.
  await client.methods
    .initialize(
      {
        chainId: fixture.chain_id,
        trustLevelNumerator: new BN(1),
        trustLevelDenominator: new BN(3),
        trustingPeriod: new BN(3600),
        unbondingPeriod: new BN(7200),
        maxClockDrift: new BN(5),
        frozenHeight: { revisionNumber: new BN(0), revisionHeight: new BN(0) },
        latestHeight: { revisionNumber: new BN(1), revisionHeight: height },
      },
      {
        timestamp: new BN(Math.floor(Date.now() / 1000)).mul(
          new BN(1_000_000_000),
        ),
        root: Array.from(root),
        nextValidatorsHash: Array<number>(32).fill(7),
      },
      access.programId,
    )
    .accountsStrict({
      clientStateAccount: clientState,
      consensusStateStore: consensus,
      appState: pda("app_state", client.programId),
      payer: provider.publicKey,
      systemProgram: web3.SystemProgram.programId,
      programData: programData(client.programId),
      authority: provider.publicKey,
    })
    .rpc();
  const admin = {
    program: program.programId,
    programData: programData(program.programId),
    authority: provider.publicKey,
  };
  const config = pda("config"),
    bridge = pda("bridge"),
    mint = pda("mint");
  await program.methods
    .configureBridge(fixture.chain_id, {
      perTransferMax: new BN(1_000_000_000),
      dailyMintCap: new BN(10_000_000_000),
      vaults: [],
    })
    .accountsStrict({
      admin,
      config,
      bridge,
      lightClientProgram: client.programId,
      clientState,
      latestConsensus: consensus,
      systemProgram: web3.SystemProgram.programId,
    })
    .rpc();
  await program.methods
    .setBridgeEnabled(true)
    .accountsStrict({ admin, config, bridge })
    .rpc();

  const nonce = new BN(1);
  const proofBuffer = web3.PublicKey.findProgramAddressSync(
    [Buffer.from("proof"), provider.publicKey.toBuffer(), le64(nonce)],
    program.programId,
  )[0];
  await program.methods
    .createProofBuffer(nonce, proof.length)
    .accountsStrict({
      submitter: provider.publicKey,
      proofBuffer,
      systemProgram: web3.SystemProgram.programId,
    })
    .rpc();
  for (let offset = 0; offset < proof.length; offset += 500) {
    await program.methods
      .appendProof(offset, proof.subarray(offset, offset + 500))
      .accountsStrict({ submitter: provider.publicKey, proofBuffer })
      .rpc();
  }
  const status = web3.PublicKey.findProgramAddressSync(
    [Buffer.from("transfer"), le64(record.id)],
    program.programId,
  )[0];
  const recipientTokenAccount = getAssociatedTokenAddressSync(
    mint,
    record.recipient,
  );
  const accounts = {
    payer: provider.publicKey,
    config,
    bridge,
    proofBuffer,
    status,
    lightClientProgram: client.programId,
    clientState,
    latestConsensus: consensus,
    proofConsensus: consensus,
    mint,
    mintAuthority: pda("mint_authority"),
    recipient: record.recipient,
    recipientTokenAccount,
    tokenProgram: TOKEN_PROGRAM_ID,
    associatedTokenProgram: ASSOCIATED_TOKEN_PROGRAM_ID,
    systemProgram: web3.SystemProgram.programId,
  };
  const mintRecord = (value: typeof record) =>
    program.methods
      .mintTransfer(value, height)
      .accountsStrict(accounts)
      .preInstructions([
        web3.ComputeBudgetProgram.setComputeUnitLimit({ units: 1_400_000 }),
        web3.ComputeBudgetProgram.requestHeapFrame({ bytes: 256 * 1024 }),
      ])
      .rpc();
  const supplyBefore = (await getMint(provider.connection, mint)).supply;
  let mintedSignature: string | undefined;
  await t.test(
    "rejects an amount altered after the Akash commitment",
    async () => {
      await assert.rejects(() =>
        mintRecord({ ...record, amount: record.amount.addn(1) }),
      );
      assert.equal(await provider.connection.getAccountInfo(status), null);
      assert.equal(
        (await getMint(provider.connection, mint)).supply,
        supplyBefore,
      );
    },
  );
  await t.test("mints against the real x/sink store proof", async () => {
    const signature = await mintRecord(record);
    mintedSignature = signature;
    const token = await getAccount(provider.connection, recipientTokenAccount);
    assert.equal(token.amount, BigInt(record.amount.toString()));
    assert.equal(
      (await getMint(provider.connection, mint)).supply,
      supplyBefore + token.amount,
    );
    const tx = await provider.connection.getTransaction(signature, {
      commitment: "confirmed",
      maxSupportedTransactionVersion: 0,
    });
    assert.ok(tx?.meta && tx.meta.err === null);
    t.diagnostic(
      `${proof.length}-byte Akash app proof; ${tx.meta.computeUnitsConsumed} Solana compute units`,
    );
  });
  await t.test("cannot mint the committed Akash deposit twice", async () => {
    await assert.rejects(() => mintRecord(record));
    assert.equal(
      (await getAccount(provider.connection, recipientTokenAccount)).amount,
      BigInt(record.amount.toString()),
    );
  });
  await t.test(
    "reporters observe finalized mint and sign native Akash reports",
    async () => {
      assert.ok(mintedSignature);
      const finalized = await provider.connection.confirmTransaction(
        mintedSignature,
        "finalized",
      );
      assert.equal(finalized.value.err, null);
      await testReporterReturn(t, provider.connection, fixture);
    },
  );
}
