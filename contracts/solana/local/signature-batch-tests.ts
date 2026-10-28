import assert from "node:assert/strict";
import { createHash, createPrivateKey, sign } from "node:crypto";
import { AnchorProvider, BN, Program, Wallet, web3 } from "@coral-xyz/anchor";
import clientIdl from "../idls/ics07_tendermint.json";
import accessIdl from "../idls/access_manager.json";
import type { Ics07Tendermint } from "../.deps/ibc-contracts/ibc-solana/target/types/ics07_tendermint";
import type { AccessManager } from "../.deps/ibc-contracts/ibc-solana/target/types/access_manager";
import { CLIENT_ID, ACCESS_ID, pda } from "./common";

// Actual SBF writer/cleanup checks, on the benchmark's isolated validator only.
export async function testSignatureBatch(wallet: AnchorProvider) {
  const client = new Program<Ics07Tendermint>(clientIdl, wallet);
  const access = new Program<AccessManager>(accessIdl, wallet);
  const appState = pda("app_state", CLIENT_ID);
  const accessManager = pda("access_manager", ACCESS_ID);
  const session = createHash("sha256").update("batch-runtime-tests").digest();
  const batch = web3.PublicKey.findProgramAddressSync(
    [Buffer.from("sig_batch"), wallet.publicKey.toBuffer(), session],
    CLIENT_ID,
  )[0];
  const accounts = {
    signatureBatch: batch,
    appState,
    accessManager,
    submitter: wallet.publicKey,
    instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
  };
  await client.methods
    .initializeSignatureBatch(Array.from(session))
    .accountsStrict({
      ...accounts,
      systemProgram: web3.SystemProgram.programId,
    })
    .rpc();
  const key = web3.Keypair.generate();
  const privateKey = createPrivateKey({
    key: Buffer.concat([
      Buffer.from("302e020100300506032b657004220420", "hex"),
      Buffer.from(key.secretKey.subarray(0, 32)),
    ]),
    format: "der",
    type: "pkcs8",
  });
  function signed(index: number) {
    const msg = Buffer.from(`receipt test ${index}`);
    const signature = sign(null, msg, privateKey);
    const pubkey = key.publicKey.toBuffer();
    return {
      signatureHash: Array.from(
        createHash("sha256")
          .update(pubkey)
          .update(msg)
          .update(signature)
          .digest(),
      ),
      pubkey: Array.from(pubkey),
      msg,
      signature: Array.from(signature),
    };
  }
  const native = (s: ReturnType<typeof signed>) =>
    web3.Ed25519Program.createInstructionWithPublicKey({
      publicKey: Uint8Array.from(s.pubkey),
      message: s.msg,
      signature: Uint8Array.from(s.signature),
    });
  const append = (s: ReturnType<typeof signed>) =>
    client.methods
      .preVerifySignatureBatch(s)
      .accountsStrict(accounts)
      .instruction();
  async function send(instructions: web3.TransactionInstruction[]) {
    return wallet.sendAndConfirm(new web3.Transaction().add(...instructions));
  }
  async function rejects(
    instructions: web3.TransactionInstruction[],
    pattern: RegExp,
  ) {
    const before = await wallet.connection.getAccountInfo(batch);
    await assert.rejects(() => send(instructions), pattern);
    assert.deepEqual(
      (await wallet.connection.getAccountInfo(batch))?.data,
      before?.data,
    );
  }
  const first = signed(0);
  await send([native(first), await append(first)]);
  await send([native(first), await append(first)]);
  assert.equal(
    (await client.account.signatureBatch.fetch(batch)).hashes.length,
    1,
  );
  await rejects([await append(first)], /InvalidAccountData/);
  const changed = { ...first, signatureHash: [...first.signatureHash] };
  changed.signatureHash[0] ^= 1;
  await rejects([native(first), await append(changed)], /InvalidAccountData/);
  await rejects([native(first), await append(signed(1))], /InvalidAccountData/);
  const bad = { ...first, signature: [...first.signature] };
  bad.signature[0] ^= 1;
  await rejects(
    [native(bad), await append(bad)],
    /Instruction 0: custom program error: 0x2/,
  );
  // Both native instructions verify, but ix0 references ix1's bytes. Our writer
  // deliberately accepts only inline offsets, as does the existing receipt path.
  const external = native(first);
  for (const offset of [4, 8, 14]) external.data.writeUInt16LE(1, offset);
  await rejects(
    [external, native(first), await append(first)],
    /InvalidAccountData/,
  );
  const stranger = web3.Keypair.generate();
  await send([
    web3.SystemProgram.transfer({
      fromPubkey: wallet.publicKey,
      toPubkey: stranger.publicKey,
      lamports: 100_000_000,
    }),
  ]);
  const foreign = new Program<Ics07Tendermint>(
    clientIdl,
    new AnchorProvider(wallet.connection, new Wallet(stranger), wallet.opts),
  );
  await assert.rejects(
    () =>
      foreign.methods
        .preVerifySignatureBatch(first)
        .accountsStrict({ ...accounts, submitter: stranger.publicKey })
        .preInstructions([native(first)])
        .rpc(),
    /ConstraintSeeds|ConstraintHasOne/,
  );
  await foreign.methods
    .cleanupIncompleteUpload()
    .accountsStrict({ submitter: stranger.publicKey })
    .remainingAccounts([{ pubkey: batch, isSigner: false, isWritable: true }])
    .rpc();
  assert.equal(
    (await client.account.signatureBatch.fetch(batch)).hashes.length,
    1,
  );
  for (let i = 1; i < 128; i++) {
    const value = signed(i);
    await send([native(value), await append(value)]);
  }
  await send([native(first), await append(first)]);
  const overflow = signed(128);
  await rejects(
    [native(overflow), await append(overflow)],
    /InvalidAccountData/,
  );
  assert.equal(
    (await client.account.signatureBatch.fetch(batch)).hashes.length,
    128,
  );
  const roleAccounts = {
    accessManager,
    admin: wallet.publicKey,
    instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
  };
  await access.methods
    .revokeRole(new BN(1), wallet.publicKey)
    .accountsStrict(roleAccounts)
    .rpc();
  try {
    await rejects([native(first), await append(first)], /Unauthorized/);
    await client.methods
      .cleanupIncompleteUpload()
      .accountsStrict({ submitter: wallet.publicKey })
      .remainingAccounts([{ pubkey: batch, isSigner: false, isWritable: true }])
      .rpc();
    assert.equal(await wallet.connection.getAccountInfo(batch), null);
    await client.methods
      .cleanupIncompleteUpload()
      .accountsStrict({ submitter: wallet.publicKey })
      .remainingAccounts([{ pubkey: batch, isSigner: false, isWritable: true }])
      .rpc();
  } finally {
    await access.methods
      .grantRole(new BN(1), wallet.publicKey)
      .accountsStrict(roleAccounts)
      .rpc();
  }
  return {
    passed: true,
    capacity: 128,
    cases: [
      "retry",
      "missing-precompile",
      "digest-mismatch",
      "message-mismatch",
      "invalid-signature",
      "external-offsets",
      "foreign-writer",
      "foreign-cleanup",
      "capacity",
      "revoked-writer",
      "cleanup-after-revocation",
      "repeated-cleanup",
    ],
  };
}
