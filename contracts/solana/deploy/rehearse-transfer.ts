import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { appendFile, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { parseArgs } from "node:util";
import { AnchorProvider, BN, Program, Wallet, web3 } from "@coral-xyz/anchor";
import {
  ASSOCIATED_TOKEN_PROGRAM_ID,
  TOKEN_PROGRAM_ID,
  getAccount,
  getAssociatedTokenAddressSync,
  getMint,
} from "@solana/spl-token";
import { z } from "zod";
import aktIdl from "../../../.cache/solana/target/idl/akt.json";
import type { Akt } from "../../../.cache/solana/target/types/akt";
import { LE64, REPO, TARGET, object, pda } from "../local/common";
import { decodePacket, uploadHeader } from "../local/header-upload";
import { accessProgram, clientProgram } from "../local/light-client";
import { MAINNET_GENESIS } from "../shared/networks";
import {
  anchorId,
  decodeRecordData,
  idFromTransferKey,
  proofAddress,
  proofNonce,
  statusAddress,
} from "../shared/proof";
import { rejectsWithProgramLogs } from "../tests/helpers";

const json = async (path: string): Promise<unknown> =>
  JSON.parse(await readFile(path, "utf8"));
const save = (path: string, value: unknown) =>
  writeFile(path, JSON.stringify(value, null, 2) + "\n", { flag: "wx" });
const hex = z.string().regex(/^(?:[0-9a-f]{2})+$/);
const publicKey = z.string().transform((value) => new web3.PublicKey(value));
const fixtureSchema = z.object({
  chain_id: z.string(),
  header_height: z.number().int().positive(),
  query_height: z.number().int().positive(),
  root: hex.length(64),
  key: hex,
  record: hex,
  proof: hex,
});
const keyBytes = z.array(z.number().int().min(0).max(255)).length(64);
const keypair = async (path: string) =>
  web3.Keypair.fromSecretKey(Uint8Array.from(keyBytes.parse(await json(path))));
const CHAIN = "akash-scale-1";

async function main() {
  const {
    positionals: [command],
    values,
  } = parseArgs({
    allowPositionals: true,
    options: { run: { type: "string" }, url: { type: "string" } },
  });
  assert.ok(values.run && values.url, "Pass --run and --url");
  const run = values.run;
  assert.ok(
    ["127.0.0.1", "localhost", "[::1]"].includes(new URL(values.url).hostname),
  );
  const connection = new web3.Connection(values.url, "finalized");
  assert.notEqual(await connection.getGenesisHash(), MAINNET_GENESIS);
  const fixture = fixtureSchema.parse(
    await json(join(run, "akash-proof.json")),
  );
  assert.equal(fixture.header_height, fixture.query_height + 1);
  const manifest = z
    .object({
      programs: z.object({
        akt: publicKey,
        ics07_tendermint: publicKey,
        access_manager: publicKey,
      }),
    })
    .parse(await json(join(run, "manifest.json")));
  const ids = manifest.programs;
  const options = {
    commitment: "finalized",
    preflightCommitment: "finalized",
  } as const;
  const ceremony = new AnchorProvider(
    connection,
    new Wallet(await keypair(join(run, "keys/ceremony.json"))),
    options,
  );
  const outsider = new AnchorProvider(
    connection,
    new Wallet(await keypair(join(run, "keys/outsider.json"))),
    options,
  );
  const program = new Program<Akt>(
    { ...aktIdl, address: ids.akt.toBase58() },
    outsider,
  );
  const client = clientProgram(outsider, ids.ics07_tendermint);
  const access = accessProgram(outsider, ids.access_manager);
  const consensusAt = (height: number) =>
    pda("consensus_state", ids.ics07_tendermint, LE64(BigInt(height)));
  const mint = pda("mint", ids.akt);
  const id = idFromTransferKey(Buffer.from(fixture.key, "hex"));
  const record = decodeRecordData(Buffer.from(fixture.record, "hex"));
  const status = statusAddress(ids.akt, id);
  const proof = Buffer.from(fixture.proof, "hex");
  const key = Buffer.from(fixture.key, "hex");
  const proofBuffer = proofAddress(ids.akt, outsider.publicKey, key);
  const recipientTokenAccount = getAssociatedTokenAddressSync(
    mint,
    record.recipient,
  );
  const compute = () => [
    web3.ComputeBudgetProgram.setComputeUnitLimit({ units: 1_400_000 }),
    web3.ComputeBudgetProgram.requestHeapFrame({ bytes: 256 * 1024 }),
  ];
  const mintRecord = (amount = record.amount) =>
    program.methods
      .mintTransfer(
        anchorId(id),
        record.recipient,
        new BN(amount.toString()),
        new BN(fixture.header_height),
      )
      .accountsStrict({
        payer: outsider.publicKey,
        config: pda("config", ids.akt),
        bridge: pda("bridge", ids.akt),
        proofBuffer,
        status,
        lightClientProgram: ids.ics07_tendermint,
        clientState: pda("client", ids.ics07_tendermint),
        latestConsensus: consensusAt(fixture.header_height),
        proofConsensus: consensusAt(fixture.header_height),
        mint,
        mintAuthority: pda("mint_authority", ids.akt),
        recipient: record.recipient,
        recipientTokenAccount,
        tokenProgram: TOKEN_PROGRAM_ID,
        associatedTokenProgram: ASSOCIATED_TOKEN_PROGRAM_ID,
        systemProgram: web3.SystemProgram.programId,
      })
      .preInstructions(compute())
      .rpc();

  if (command === "checkpoint") {
    const clock = await connection.getAccountInfo(web3.SYSVAR_CLOCK_PUBKEY);
    assert.ok(clock && clock.data.length === 40);
    const seconds = clock.data.readBigInt64LE(32);
    const template = object(
      await json(join(REPO, "tools/header-encoder/fixtures/template.json")),
    );
    const signed = object(template.signed_header);
    const header = object(signed.header);
    header.height = String(fixture.header_height);
    header.time = new Date(Number(seconds - 2n) * 1000).toISOString();
    header.app_hash = fixture.root.toUpperCase();
    signed.header = header;
    signed.commit = { ...object(signed.commit), height: header.height };
    template.signed_header = signed;
    template.trusted_height = 1;
    const packetValue: unknown = JSON.parse(
      execFileSync(
        join(TARGET, "debug/akt-header-encoder"),
        ["--fixture", "4", "3", "equal"],
        { input: JSON.stringify(template), encoding: "utf8" },
      ),
    );
    const packet = decodePacket(packetValue, CHAIN);
    assert.equal(
      Buffer.from(packet.consensus.root).toString("hex"),
      fixture.root,
    );
    await save(join(run, "header-packet.json"), packetValue);
    const checkpointTime = seconds - 60n;
    const validatorsHash = Buffer.from(
      packet.consensus.nextValidatorsHash,
    ).toString("hex");
    await save(join(run, "checkpoint.json"), {
      chainId: CHAIN,
      height: "1",
      time: new Date(Number(checkpointTime) * 1000).toISOString(),
      timestamp: String(checkpointTime * 1_000_000_000n),
      root: "00".repeat(32),
      nextValidatorsHash: validatorsHash,
      validatorsHash,
      blockHash: "00".repeat(32),
    });
    await save(join(run, "proof-scope.json"), {
      proof:
        "Signed MsgSink committed by the real Akash app in an isolated test",
      appChainId: fixture.chain_id,
      header: "Synthetic local validator set signs the real app proof root",
      headerChainId: CHAIN,
      authenticMainnetCheckpoint: false,
    });
    return;
  }

  if (command === "relay") {
    await ceremony.sendAndConfirm(
      new web3.Transaction().add(
        web3.SystemProgram.transfer({
          fromPubkey: ceremony.publicKey,
          toPubkey: outsider.publicKey,
          lamports: 2 * web3.LAMPORTS_PER_SOL,
        }),
      ),
    );
    const roles = (
      await access.account.accessManager.fetch(
        pda("access_manager", ids.access_manager),
      )
    ).roles;
    const relayers = roles.find((role) => role.roleId.eqn(1))?.members ?? [];
    assert.ok(relayers.some((member) => member.equals(ids.ics07_tendermint)));
    assert.ok(!relayers.some((member) => member.equals(outsider.publicKey)));
    // Match the relayer's confirmed intermediate reads. Header assembly and
    // the state assertions below still require finalized results.
    const relayWallet = new AnchorProvider(
      new web3.Connection(values.url, "confirmed"),
      outsider.wallet,
      { commitment: "confirmed", preflightCommitment: "confirmed" },
    );
    const result = await uploadHeader({
      wallet: relayWallet,
      packet: decodePacket(await json(join(run, "header-packet.json")), CHAIN),
      target: fixture.header_height,
      trusted: 1,
      lightClient: {
        program: ids.ics07_tendermint,
        accessManager: ids.access_manager,
      },
      record: (value) =>
        appendFile(
          join(run, "header-transactions.jsonl"),
          JSON.stringify(value) + "\n",
        ),
    });
    assert.equal(result.assembly.error, null, "outsider header update failed");
    const state = await client.account.clientState.fetch(
      pda("client", ids.ics07_tendermint),
    );
    assert.equal(
      state.latestHeight.revisionHeight.toNumber(),
      fixture.header_height,
    );
    const consensus = await client.account.consensusStateStore.fetch(
      consensusAt(fixture.header_height),
    );
    assert.equal(
      Buffer.from(consensus.consensusState.root).toString("hex"),
      fixture.root,
    );
    await rejectsWithProgramLogs(
      () =>
        client.methods
          .pruneConsensusState(new BN(1))
          .accountsStrict({
            clientState: pda("client", ids.ics07_tendermint),
            appState: pda("app_state", ids.ics07_tendermint),
            accessManager: pda("access_manager", ids.access_manager),
            consensusStateStore: consensusAt(1),
            submitter: outsider.publicKey,
            instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
          })
          .rpc(),
      /Unauthorized/,
    );
    await program.methods
      .createProofBuffer(Array.from(proofNonce(key)), proof.length)
      .accountsStrict({
        submitter: outsider.publicKey,
        proofBuffer,
        systemProgram: web3.SystemProgram.programId,
      })
      .rpc();
    if (proof.length + 80 > 10_240)
      await program.methods
        .growProofBuffer()
        .accountsStrict({
          submitter: outsider.publicKey,
          proofBuffer,
          systemProgram: web3.SystemProgram.programId,
        })
        .rpc();
    for (let offset = 0; offset < proof.length; offset += 500) {
      await program.methods
        .appendProof(offset, proof.subarray(offset, offset + 500))
        .accountsStrict({ submitter: outsider.publicKey, proofBuffer })
        .preInstructions(compute())
        .rpc();
    }
    await save(join(run, "open-relay-result.json"), {
      outsider: outsider.publicKey.toBase58(),
      explicitMember: false,
      updatedHeight: fixture.header_height,
      pruningRejected: true,
      result,
    });
    return;
  }

  if (command === "paused") {
    await rejectsWithProgramLogs(() => mintRecord(), /BridgeDisabled/);
    assert.equal((await getMint(connection, mint, "finalized")).supply, 0n);
    assert.equal(await connection.getAccountInfo(status), null);
    await save(join(run, "paused-mint-result.json"), {
      rejected: "BridgeDisabled",
      supply: "0",
      statusCreated: false,
    });
    return;
  }

  assert.equal(command, "mint", "expected checkpoint, relay, paused or mint");
  await rejectsWithProgramLogs(
    () => mintRecord(record.amount + 1n),
    /InvalidMembershipProof/,
  );
  assert.equal(await connection.getAccountInfo(status), null);
  assert.equal((await getMint(connection, mint, "finalized")).supply, 0n);
  const signature = await mintRecord();
  assert.equal(
    (await getAccount(connection, recipientTokenAccount, "finalized")).amount,
    record.amount,
  );
  assert.equal(
    (await getMint(connection, mint, "finalized")).supply,
    record.amount,
  );
  await rejectsWithProgramLogs(() => mintRecord(), /already in use/);
  assert.equal(
    (await getMint(connection, mint, "finalized")).supply,
    record.amount,
  );
  const transfer = await program.account.transferStatus.fetch(status);
  assert.ok(transfer.outcome.minted);
  assert.equal(
    transfer.outcome.minted.amount.toString(),
    record.amount.toString(),
  );
  assert.ok(transfer.outcome.minted.recipient.equals(record.recipient));
  const transaction = await connection.getTransaction(signature, {
    commitment: "finalized",
    maxSupportedTransactionVersion: 0,
  });
  assert.ok(transaction?.meta && transaction.meta.err === null);
  await save(join(run, "mint-result.json"), {
    signature,
    recipient: record.recipient.toBase58(),
    amount: record.amount.toString(),
    proofBytes: proof.length,
    computeUnits: transaction.meta.computeUnitsConsumed,
    alteredAmountRejected: true,
    replayRejected: true,
  });
}

void main().catch((error: unknown) => {
  process.stderr.write(
    `${error instanceof Error ? error.stack : String(error)}\n`,
  );
  process.exitCode = 1;
});
