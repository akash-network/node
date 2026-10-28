import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { appendFile } from "node:fs/promises";
import { join } from "node:path";
import { AnchorProvider, BN, Program, web3 } from "@coral-xyz/anchor";
import clientIdl from "../idls/ics07_tendermint.json";
import type { Ics07Tendermint } from "../.deps/ibc-contracts/ibc-solana/target/types/ics07_tendermint";
import { CLIENT_ID, ACCESS_ID, pda, LE64 } from "./common";
import { assertClientActive } from "./client-status";
import { decodePacket, uploadHeader } from "./header-upload";
export async function testExpiry(
  wallet: AnchorProvider,
  packet: ReturnType<typeof decodePacket>,
  root: string,
) {
  const client = new Program<Ics07Tendermint>(clientIdl, wallet);
  const balance = await wallet.connection.getBalance(wallet.publicKey);
  await assert.rejects(
    () =>
      uploadHeader({
        wallet,
        packet,
        target: 3,
        trusted: 1,
        journalDirectory: join(root, "expired"),
        beforeUpload: () => assertClientActive(wallet, client, "akash-scale-1"),
        record: async () => {},
      }),
    /light client expired/,
  );
  assert.equal(
    await wallet.connection.getBalance(wallet.publicKey),
    balance,
    "expiry preflight spent fees",
  );
  // Bypass only the off-chain preflight to prove the actual on-chain verifier
  // independently refuses this authentic header after its trust period.
  const onchain = await uploadHeader({
    wallet,
    packet,
    target: 3,
    trusted: 1,
    record: (value) =>
      appendFile(
        join(root, "expiry-transactions.jsonl"),
        JSON.stringify(value) + "\n",
      ),
  });
  assert.notEqual(onchain.assembly.error, null);
  assert.equal(
    (
      await client.account.clientState.fetch(pda("client", CLIENT_ID))
    ).latestHeight.revisionHeight.toNumber(),
    1,
  );
  assert.equal(
    await client.account.consensusStateStore.fetchNullable(
      pda("consensus_state", CLIENT_ID, LE64(3n)),
    ),
    null,
  );
  return {
    passed: true,
    trustingPeriodSeconds: 5,
    checkpointAgeAtInitializationSeconds: 60,
    guardSpentLamports: 0,
    onchainRejection: onchain.assembly.error,
    trustedHeight: 1,
  };
}
export async function testMisbehaviour(
  wallet: AnchorProvider,
  first: ReturnType<typeof decodePacket>,
  second: ReturnType<typeof decodePacket>,
) {
  const client = new Program<Ics07Tendermint>(clientIdl, wallet);
  const appState = pda("app_state", CLIENT_ID),
    clientState = pda("client", CLIENT_ID),
    accessManager = pda("access_manager", ACCESS_ID);
  const session = createHash("sha256").update("misbehaviour test").digest();
  const signatureBatch = web3.PublicKey.findProgramAddressSync(
    [Buffer.from("sig_batch"), wallet.publicKey.toBuffer(), session],
    CLIENT_ID,
  )[0];
  const common = {
    appState,
    accessManager,
    submitter: wallet.publicKey,
    instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
  };
  await client.methods
    .initializeSignatureBatch(Array.from(session))
    .accountsStrict({
      ...common,
      signatureBatch,
      systemProgram: web3.SystemProgram.programId,
    })
    .rpc();
  for (const sig of [...first.signatures, ...second.signatures])
    await client.methods
      .preVerifySignatureBatch(sig)
      .accountsStrict({ ...common, signatureBatch })
      .preInstructions([
        web3.Ed25519Program.createInstructionWithPublicKey({
          publicKey: Uint8Array.from(sig.pubkey),
          message: sig.msg,
          signature: Uint8Array.from(sig.signature),
        }),
      ])
      .rpc();
  async function submit(right: ReturnType<typeof decodePacket>) {
    // The pinned upstream BorshMisbehaviour is string client_id, Header, Header.
    const name = Buffer.from("07-tendermint-0");
    const size = Buffer.alloc(4);
    size.writeUInt32LE(name.length);
    const bytes = Buffer.concat([size, name, first.bytes, right.bytes]);
    const chunks = Array.from(
      { length: Math.ceil(bytes.length / 600) },
      (_, i) =>
        web3.PublicKey.findProgramAddressSync(
          [
            Buffer.from("misbehaviour_chunk"),
            wallet.publicKey.toBuffer(),
            Buffer.from([i]),
          ],
          CLIENT_ID,
        )[0],
    );
    try {
      for (const [i, chunk] of chunks.entries())
        await client.methods
          .uploadMisbehaviourChunk({
            chunkIndex: i,
            chunkData: bytes.subarray(i * 600, (i + 1) * 600),
          })
          .accountsStrict({
            ...common,
            clientState,
            chunk,
            systemProgram: web3.SystemProgram.programId,
          })
          .rpc();
      return await client.methods
        .assembleAndSubmitMisbehaviour(chunks.length, new BN(1), new BN(1))
        .accountsStrict({
          ...common,
          clientState,
          trustedConsensusState1: pda("consensus_state", CLIENT_ID, LE64(1n)),
          trustedConsensusState2: pda("consensus_state", CLIENT_ID, LE64(1n)),
        })
        .remainingAccounts(
          [...chunks, signatureBatch].map((pubkey) => ({
            pubkey,
            isSigner: false,
            isWritable: true,
          })),
        )
        .preInstructions([
          web3.ComputeBudgetProgram.setComputeUnitLimit({ units: 1_400_000 }),
          web3.ComputeBudgetProgram.requestHeapFrame({ bytes: 256 * 1024 }),
        ])
        .rpc();
    } finally {
      await client.methods
        .cleanupIncompleteMisbehaviour()
        .accountsStrict({ submitter: wallet.publicKey })
        .remainingAccounts(
          chunks.map((pubkey) => ({
            pubkey,
            isSigner: false,
            isWritable: true,
          })),
        )
        .rpc();
    }
  }
  try {
    await assert.rejects(() => submit(first), /MisbehaviourCheckFailed/);
    assert.ok(
      (
        await client.account.clientState.fetch(clientState)
      ).frozenHeight.revisionHeight.isZero(),
    );
    const signature = await submit(second);
    assert.ok(
      !(
        await client.account.clientState.fetch(clientState)
      ).frozenHeight.revisionHeight.isZero(),
    );
    await assert.rejects(
      () => assertClientActive(wallet, client, "akash-scale-1"),
      /frozen/,
    );
    const chunk = web3.PublicKey.findProgramAddressSync(
      [
        Buffer.from("header_chunk"),
        wallet.publicKey.toBuffer(),
        LE64(4n),
        Buffer.from([0]),
      ],
      CLIENT_ID,
    )[0];
    await assert.rejects(
      () =>
        client.methods
          .uploadHeaderChunk({
            targetHeight: new BN(4),
            chunkIndex: 0,
            chunkData: Buffer.from([1]),
          })
          .accountsStrict({
            ...common,
            clientState,
            chunk,
            systemProgram: web3.SystemProgram.programId,
          })
          .rpc(),
      /ClientFrozen/,
    );
    const tx = await wallet.connection.getTransaction(signature, {
      commitment: "confirmed",
      maxSupportedTransactionVersion: 0,
    });
    return {
      passed: true,
      validators: 4,
      signaturesPerHeader: 3,
      identicalHeadersRejected: true,
      conflictingHeadersFreeze: true,
      furtherUpdatesRejected: true,
      signature,
      computeUnits: tx?.meta?.computeUnitsConsumed,
    };
  } finally {
    await client.methods
      .cleanupIncompleteUpload()
      .accountsStrict({ submitter: wallet.publicKey })
      .remainingAccounts([
        { pubkey: signatureBatch, isSigner: false, isWritable: true },
      ])
      .rpc();
  }
}
