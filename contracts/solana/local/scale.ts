import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { openSync, closeSync } from "node:fs";
import { appendFile, mkdir, mkdtemp, readFile } from "node:fs/promises";
import { join } from "node:path";
import { createConnection } from "node:net";
import { createHash } from "node:crypto";
import { AnchorProvider, BN, Program, Wallet, web3 } from "@coral-xyz/anchor";
import clientIdl from "../idls/ics07_tendermint.json";
import accessIdl from "../idls/access_manager.json";
import type { Ics07Tendermint } from "../.deps/ibc-contracts/ibc-solana/target/types/ics07_tendermint";
import type { AccessManager } from "../.deps/ibc-contracts/ibc-solana/target/types/access_manager";
import {
  REPO,
  CLIENT_ID,
  ACCESS_ID,
  pda,
  LE64,
  programData,
  object,
  save,
  sleep,
} from "./common";
import { decodePacket, uploadHeader } from "./header-upload";
import { testSignatureBatch } from "./signature-batch-tests";
import { testRecovery } from "./recovery-tests";
import { testMisbehaviour, testExpiry } from "./client-fault-tests";
import { assertClientActive } from "./client-status";
import { pruneExpiredConsensusStates } from "./consensus-prune";
import { transferKey } from "../shared/sink";

const PATCHES = [
  "patches/ics07-signature-batches.patch",
  "patches/ics07-monotonic-latest-height.patch",
  "patches/ics07-consensus-pruning.patch",
  "patches/ics07-open-relaying.patch",
];
type Packet = ReturnType<typeof decodePacket>;

// The prune run's checkpoint expires 180 s after initialization; each later
// state expires 300 s after its header time.
const PRUNE_TRUSTING_SECONDS = 300;
const PRUNE_CHECKPOINT_AGE_SECONDS = 120;

async function chainNanos(wallet: AnchorProvider) {
  const clock = await wallet.connection.getAccountInfo(
    web3.SYSVAR_CLOCK_PUBKEY,
    "confirmed",
  );
  assert.ok(clock && clock.data.length === 40, "Solana clock unavailable");
  return clock.data.readBigInt64LE(32) * 1_000_000_000n;
}

// prune_consensus_state closes only expired, non-latest states for a relayer.
async function testPrune(
  wallet: AnchorProvider,
  record: (value: unknown) => Promise<void>,
  packet: (target: number, trusted: number) => Promise<Packet>,
) {
  const client = new Program<Ics07Tendermint>(clientIdl, wallet);
  const consensusAt = (height: number) =>
    pda("consensus_state", CLIENT_ID, LE64(BigInt(height)));
  const expiry = async (height: number) =>
    BigInt(
      (
        await client.account.consensusStateStore.fetch(consensusAt(height))
      ).consensusState.timestamp.toString(),
    ) +
    BigInt(PRUNE_TRUSTING_SECONDS) * 1_000_000_000n;
  const expired = async (height: number) =>
    (await chainNanos(wallet)) >= (await expiry(height));
  async function waitExpired(height: number) {
    const deadline = Date.now() + (PRUNE_TRUSTING_SECONDS + 120) * 1000;
    while (!(await expired(height))) {
      assert.ok(Date.now() < deadline, `state ${height} did not expire`);
      await sleep(1000);
    }
  }
  async function update(target: number, trusted: number) {
    const result = await uploadHeader({
      wallet,
      packet: await packet(target, trusted),
      target,
      trusted,
      record,
    });
    assert.equal(result.assembly.error, null, `update to ${target} failed`);
  }
  const prune = (height: number, signer: AnchorProvider = wallet) =>
    new Program<Ics07Tendermint>(clientIdl, signer).methods
      .pruneConsensusState(new BN(height))
      .accountsStrict({
        clientState: pda("client", CLIENT_ID),
        appState: pda("app_state", CLIENT_ID),
        accessManager: pda("access_manager", ACCESS_ID),
        consensusStateStore: consensusAt(height),
        submitter: signer.publicKey,
        instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
      })
      .rpc();
  async function pruned(height: number) {
    const rent = (await wallet.connection.getAccountInfo(consensusAt(height)))
      ?.lamports;
    assert.equal(
      rent,
      await wallet.connection.getMinimumBalanceForRentExemption(88),
    );
    const before = await wallet.connection.getBalance(wallet.publicKey);
    const signature = await prune(height);
    const tx = await wallet.connection.getTransaction(signature, {
      commitment: "confirmed",
      maxSupportedTransactionVersion: 0,
    });
    assert.ok(tx?.meta);
    assert.equal(
      (await wallet.connection.getBalance(wallet.publicKey)) - before,
      rent - tx.meta.fee,
      "pruned rent did not return to the relayer",
    );
    assert.equal(
      await wallet.connection.getAccountInfo(consensusAt(height)),
      null,
    );
    await record({ event: "pruned", height, rent, signature });
    return rent;
  }
  const results: Record<string, unknown> = {};

  await update(3, 1);
  assert.ok(!(await expired(1)), "checkpoint expired before the first check");
  await assert.rejects(() => prune(1), /ConsensusStateNotExpired/);
  results.rejectsUnexpired = true;

  await update(4, 3);
  await update(5, 4);
  await waitExpired(1);
  await assertClientActive(wallet, client, "akash-scale-1");
  const outsiderKey = web3.Keypair.generate();
  await wallet.sendAndConfirm(
    new web3.Transaction().add(
      web3.SystemProgram.transfer({
        fromPubkey: wallet.publicKey,
        toPubkey: outsiderKey.publicKey,
        lamports: web3.LAMPORTS_PER_SOL / 10,
      }),
    ),
  );
  const outsider = new AnchorProvider(
    wallet.connection,
    new Wallet(outsiderKey),
    wallet.opts,
  );
  const held = await wallet.connection.getAccountInfo(consensusAt(1));
  await assert.rejects(() => prune(1, outsider), /Error Code: Unauthorized\b/);
  assert.deepEqual(
    await wallet.connection.getAccountInfo(consensusAt(1)),
    held,
  );
  results.rejectsWithoutRole = true;
  results.prunedWhileActiveLamports = await pruned(1);
  await assertClientActive(wallet, client, "akash-scale-1");
  await assert.rejects(
    () =>
      client.methods
        .verifyMembership({
          height: new BN(1),
          proof: Buffer.from([0]),
          path: [Buffer.from("sink"), Buffer.from(transferKey(1))],
          value: Buffer.from([1]),
        })
        .accountsStrict({
          clientState: pda("client", CLIENT_ID),
          consensusStateAtHeight: consensusAt(1),
        })
        .rpc(),
    /AccountNotInitialized/,
  );
  results.proofAtPrunedHeightRejected = true;
  await assert.rejects(() => prune(1), /AccountNotInitialized/);
  // Pruning an old root leaves verification from recent states intact.
  await update(7, 5);
  results.updateAfterPrune = 7;

  await waitExpired(7);
  await assert.rejects(() => prune(7), /CannotPruneLatestConsensusState/);
  assert.ok(await client.account.consensusStateStore.fetch(consensusAt(7)));
  results.rejectsLatest = true;

  // The relayer helper: role check, bounded batches, never the latest.
  const outsiderRun = await pruneExpiredConsensusStates(outsider);
  assert.equal(outsiderRun.skipped, "payer lacks RELAYER_ROLE");
  assert.equal(outsiderRun.signatures.length, 0);
  const before = await wallet.connection.getBalance(wallet.publicKey);
  const first = await pruneExpiredConsensusStates(wallet, {
    maxPerTick: 2,
    perTransaction: 2,
  });
  assert.deepEqual(
    [first.scanned, first.expired, first.pruned, first.remaining],
    [4, 3, 2, 1],
  );
  assert.equal(first.signatures.length, 1, "two prunes share a transaction");
  const second = await pruneExpiredConsensusStates(wallet);
  assert.deepEqual(
    [second.expired, second.pruned, second.remaining, second.failed],
    [1, 1, 0, 0],
  );
  const idle = await pruneExpiredConsensusStates(wallet);
  assert.deepEqual(
    [idle.scanned, idle.expired, idle.signatures.length],
    [1, 0, 0],
  );
  const rent = await wallet.connection.getMinimumBalanceForRentExemption(88);
  assert.equal(first.reclaimedLamports + second.reclaimedLamports, 3 * rent);
  assert.equal(
    (await wallet.connection.getBalance(wallet.publicKey)) - before,
    3 * rent - first.feeLamports - second.feeLamports,
  );
  for (const height of [3, 4, 5])
    assert.equal(
      await wallet.connection.getAccountInfo(consensusAt(height)),
      null,
    );
  assert.ok(await client.account.consensusStateStore.fetch(consensusAt(7)));
  const compute = [];
  for (const signature of [...first.signatures, ...second.signatures])
    compute.push(
      (
        await wallet.connection.getTransaction(signature, {
          commitment: "confirmed",
          maxSupportedTransactionVersion: 0,
        })
      )?.meta?.computeUnitsConsumed,
    );
  results.helper = {
    outsiderSkipped: true,
    first: { ...first, computeUnits: compute[0] },
    second: { ...second, computeUnits: compute[1] },
    idle,
  };
  return {
    passed: true,
    trustingPeriodSeconds: PRUNE_TRUSTING_SECONDS,
    ...results,
  };
}

// Granting RELAYER_ROLE to the ICS07 program ID opens header and misbehaviour
// relaying to any signer. Pruning still needs a real member.
async function testOpenRelaying(
  wallet: AnchorProvider,
  record: (value: unknown) => Promise<void>,
  packet: (target: number, trusted: number) => Promise<Packet>,
  misbehaviour: (signer: AnchorProvider) => Promise<unknown>,
) {
  const client = new Program<Ics07Tendermint>(clientIdl, wallet),
    access = new Program<AccessManager>(accessIdl, wallet);
  const accessManager = pda("access_manager", ACCESS_ID),
    appState = pda("app_state", CLIENT_ID),
    clientState = pda("client", CLIENT_ID),
    consensusAt = (height: number) =>
      pda("consensus_state", CLIENT_ID, LE64(BigInt(height)));
  const roleAccounts = {
    accessManager,
    admin: wallet.publicKey,
    instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
  };
  const relayers = async () =>
    (await access.account.accessManager.fetch(accessManager)).roles
      .find((role) => role.roleId.eqn(1))
      ?.members.map((member: web3.PublicKey) => member.toBase58()) ?? [];
  async function setOpen(open: boolean) {
    await (
      open
        ? access.methods.grantRole(new BN(1), CLIENT_ID)
        : access.methods.revokeRole(new BN(1), CLIENT_ID)
    )
      .accountsStrict(roleAccounts)
      .rpc();
    assert.equal(
      (await relayers()).includes(CLIENT_ID.toBase58()),
      open,
      "marker state",
    );
  }
  const outsiderKey = web3.Keypair.generate();
  await wallet.sendAndConfirm(
    new web3.Transaction().add(
      web3.SystemProgram.transfer({
        fromPubkey: wallet.publicKey,
        toPubkey: outsiderKey.publicKey,
        lamports: web3.LAMPORTS_PER_SOL,
      }),
    ),
  );
  const outsider = new AnchorProvider(
    wallet.connection,
    new Wallet(outsiderKey),
    wallet.opts,
  );
  const asOutsider = new Program<Ics07Tendermint>(clientIdl, outsider);
  const common = {
    appState,
    accessManager,
    submitter: outsider.publicKey,
    instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
  };
  const ed25519 = (sig: Packet["signatures"][number]) =>
    web3.Ed25519Program.createInstructionWithPublicKey({
      publicKey: Uint8Array.from(sig.pubkey),
      message: sig.msg,
      signature: Uint8Array.from(sig.signature),
    });
  const batchAt = (session: Buffer) =>
    web3.PublicKey.findProgramAddressSync(
      [Buffer.from("sig_batch"), outsider.publicKey.toBuffer(), session],
      CLIENT_ID,
    )[0];
  const receiptAt = (sig: Packet["signatures"][number]) =>
    web3.PublicKey.findProgramAddressSync(
      [Buffer.from("sig_verify"), Buffer.from(sig.signatureHash)],
      CLIENT_ID,
    )[0];
  const sample = (await packet(9, 1)).signatures;
  // The open phase creates `session` and a receipt for sample[2]; rejection
  // checks use other addresses so an existing account cannot mask the result.
  const session = createHash("sha256").update("open relaying").digest(),
    unused = createHash("sha256").update("open relaying unused").digest();
  const gated = {
    uploadHeaderChunk: () =>
      asOutsider.methods
        .uploadHeaderChunk({
          targetHeight: new BN(9),
          chunkIndex: 0,
          chunkData: Buffer.from([1]),
        })
        .accountsStrict({
          ...common,
          clientState,
          chunk: web3.PublicKey.findProgramAddressSync(
            [
              Buffer.from("header_chunk"),
              outsider.publicKey.toBuffer(),
              LE64(9n),
              Buffer.from([0]),
            ],
            CLIENT_ID,
          )[0],
          systemProgram: web3.SystemProgram.programId,
        })
        .rpc(),
    initializeSignatureBatch: (batchSession = unused) =>
      asOutsider.methods
        .initializeSignatureBatch(Array.from(batchSession))
        .accountsStrict({
          ...common,
          signatureBatch: batchAt(batchSession),
          systemProgram: web3.SystemProgram.programId,
        })
        .rpc(),
    preVerifySignatureBatch: () =>
      asOutsider.methods
        .preVerifySignatureBatch(sample[0])
        .accountsStrict({ ...common, signatureBatch: batchAt(session) })
        .preInstructions([ed25519(sample[0])])
        .rpc(),
    preVerifySignature: (sig = sample[1]) =>
      asOutsider.methods
        .preVerifySignature(sig)
        .accountsStrict({
          ...common,
          signatureVerification: receiptAt(sig),
          systemProgram: web3.SystemProgram.programId,
        })
        .preInstructions([ed25519(sig)])
        .rpc(),
    assembleAndUpdateClient: () =>
      asOutsider.methods
        .assembleAndUpdateClient(new BN(9), 0, new BN(1))
        .accountsStrict({
          ...common,
          clientState,
          trustedConsensusState: consensusAt(1),
          newConsensusStateStore: consensusAt(9),
          systemProgram: web3.SystemProgram.programId,
        })
        .rpc(),
    uploadMisbehaviourChunk: () =>
      asOutsider.methods
        .uploadMisbehaviourChunk({ chunkIndex: 0, chunkData: Buffer.from([1]) })
        .accountsStrict({
          ...common,
          clientState,
          chunk: web3.PublicKey.findProgramAddressSync(
            [
              Buffer.from("misbehaviour_chunk"),
              outsider.publicKey.toBuffer(),
              Buffer.from([0]),
            ],
            CLIENT_ID,
          )[0],
          systemProgram: web3.SystemProgram.programId,
        })
        .rpc(),
    assembleAndSubmitMisbehaviour: () =>
      asOutsider.methods
        .assembleAndSubmitMisbehaviour(0, new BN(1), new BN(1))
        .accountsStrict({
          ...common,
          clientState,
          trustedConsensusState1: consensusAt(1),
          trustedConsensusState2: consensusAt(1),
        })
        .rpc(),
  };
  async function allRejected(names: (keyof typeof gated)[]) {
    const balance = await outsider.connection.getBalance(outsider.publicKey);
    for (const name of names)
      await assert.rejects(
        () => gated[name](),
        /Error Code: Unauthorized\b/,
        `${name} accepted a non-member`,
      );
    // Rejected transactions still pay their fee and nothing else.
    assert.ok(
      balance - (await outsider.connection.getBalance(outsider.publicKey)) <=
        names.length * 10_000,
    );
    return names;
  }
  const prune = (signer: AnchorProvider) =>
    new Program<Ics07Tendermint>(clientIdl, signer).methods
      .pruneConsensusState(new BN(1))
      .accountsStrict({
        clientState,
        appState,
        accessManager,
        consensusStateStore: consensusAt(1),
        submitter: signer.publicKey,
        instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
      })
      .rpc();
  const results: Record<string, unknown> = {};

  assert.deepEqual(await relayers(), [wallet.publicKey.toBase58()]);
  results.closedRejected = await allRejected([
    "uploadHeaderChunk",
    "initializeSignatureBatch",
    "preVerifySignature",
    "assembleAndUpdateClient",
    "uploadMisbehaviourChunk",
    "assembleAndSubmitMisbehaviour",
  ]);

  await setOpen(true);
  await gated.initializeSignatureBatch(session);
  await gated.preVerifySignatureBatch();
  await gated.preVerifySignature(sample[2]);
  assert.ok(
    (await client.account.signatureVerification.fetch(receiptAt(sample[2])))
      .isValid,
  );
  const update = await uploadHeader({
    wallet: outsider,
    packet: await packet(4, 1),
    target: 4,
    trusted: 1,
    record,
  });
  assert.equal(update.assembly.error, null, "open update rejected");
  assert.equal(
    (
      await client.account.clientState.fetch(clientState)
    ).latestHeight.revisionHeight.toNumber(),
    4,
  );
  // uploadHeader reconciles the payer: the outsider paid the new state's rent.
  assert.equal(
    update.costs.permanentRent,
    await wallet.connection.getMinimumBalanceForRentExemption(88),
  );
  results.openUpdate = {
    target: 4,
    compute: update.assembly.compute,
    costs: update.costs,
  };

  // The marker never counts for pruning: a non-member is refused, and the
  // program ID cannot act as the submitter even with its keypair, which is
  // public for upstream's localnet ID.
  const held = await wallet.connection.getAccountInfo(consensusAt(1));
  await assert.rejects(() => prune(outsider), /Error Code: Unauthorized\b/);
  const programKey = web3.Keypair.fromSecretKey(
    Uint8Array.from(
      JSON.parse(
        await readFile(
          join(
            REPO,
            ".deps/ibc-contracts/solana-keypairs/localnet/ics07_tendermint-keypair.json",
          ),
          "utf8",
        ),
      ),
    ),
  );
  assert.ok(programKey.publicKey.equals(CLIENT_ID));
  const loader = new web3.PublicKey(
    "BPFLoaderUpgradeab1e11111111111111111111111",
  );
  const programSigner: Record<string, string> = {};
  for (const [name, extra] of [
    ["demoted", []],
    ["writable", [loader]],
  ] as const) {
    const instruction = await client.methods
      .pruneConsensusState(new BN(1))
      .accountsStrict({
        clientState,
        appState,
        accessManager,
        consensusStateStore: consensusAt(1),
        submitter: CLIENT_ID,
        instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
      })
      .instruction();
    // An invoked program's account is demoted to read-only unless the
    // upgradeable loader is also in the transaction.
    instruction.keys.push(
      ...extra.map((pubkey) => ({
        pubkey,
        isSigner: false,
        isWritable: false,
      })),
    );
    const error = await wallet
      .sendAndConfirm(new web3.Transaction().add(instruction), [programKey])
      .then(
        () => null,
        (err: unknown) => String(err),
      );
    assert.notEqual(error, null, `program ID pruned a state (${name})`);
    programSigner[name] = String(error).slice(0, 600);
  }
  // A writable program-ID submitter passes require_role while the marker is
  // granted; only the explicit check in prune_consensus_state refuses it.
  assert.match(programSigner.writable, /Unauthorized|0x1770/);
  await assert.rejects(() => prune(wallet), /ConsensusStateNotExpired/);
  assert.deepEqual(
    await wallet.connection.getAccountInfo(consensusAt(1)),
    held,
  );
  results.pruneWhileOpen = {
    outsiderRejected: true,
    programIdSignerRejected: programSigner,
    memberPassesRoleCheck: true,
  };

  await setOpen(false);
  results.revokedRejected = await allRejected([
    "uploadHeaderChunk",
    "initializeSignatureBatch",
    "preVerifySignatureBatch",
    "preVerifySignature",
    "assembleAndUpdateClient",
    "uploadMisbehaviourChunk",
    "assembleAndSubmitMisbehaviour",
  ]);
  // Cleanup is not gated, so a closed relayer still recovers its rent.
  const balance = await outsider.connection.getBalance(outsider.publicKey);
  const cleanup = await asOutsider.methods
    .cleanupIncompleteUpload()
    .accountsStrict({ submitter: outsider.publicKey })
    .remainingAccounts(
      [batchAt(session), receiptAt(sample[2])].map((pubkey) => ({
        pubkey,
        isSigner: false,
        isWritable: true,
      })),
    )
    .rpc();
  assert.equal(await wallet.connection.getAccountInfo(batchAt(session)), null);
  assert.equal(
    await wallet.connection.getAccountInfo(receiptAt(sample[2])),
    null,
  );
  results.closedCleanup = {
    signature: cleanup,
    returnedLamports:
      (await outsider.connection.getBalance(outsider.publicKey)) - balance,
  };
  const member = await uploadHeader({
    wallet,
    packet: await packet(5, 4),
    target: 5,
    trusted: 4,
    record,
  });
  assert.equal(member.assembly.error, null, "member update after revoke");
  results.memberUpdateAfterRevoke = 5;

  await setOpen(true);
  results.openMisbehaviour = await misbehaviour(outsider);
  assert.ok(
    !(
      await client.account.clientState.fetch(clientState)
    ).frozenHeight.revisionHeight.isZero(),
  );
  return { passed: true, marker: CLIENT_ID.toBase58(), ...results };
}

// A 100-signature commit split into two 64-hash batches authenticates; a
// packet above the verifier's 256-signature limit never reaches the chain.
async function testBatchSplit(
  wallet: AnchorProvider,
  record: (value: unknown) => Promise<void>,
  packet: Packet,
) {
  const client = new Program<Ics07Tendermint>(clientIdl, wallet);
  const stages: string[] = [];
  const split = await uploadHeader({
    wallet,
    packet,
    target: 3,
    trusted: 1,
    signaturesPerBatch: 64,
    record: async (value) => {
      const row = object(value);
      if (row.event === "transaction" && typeof row.stage === "string")
        stages.push(row.stage);
      await record(value);
    },
  });
  assert.equal(split.assembly.error, null, "split commit rejected");
  assert.equal(packet.signatures.length, 100);
  assert.equal(
    stages.filter((stage) => stage === "signature-batch-create").length,
    2,
  );
  assert.equal(stages.filter((stage) => stage === "signature").length, 100);
  const state = await client.account.clientState.fetch(
    pda("client", CLIENT_ID),
  );
  assert.equal(state.latestHeight.revisionHeight.toNumber(), 3);
  const balance = await wallet.connection.getBalance(wallet.publicKey);
  const oversized = {
    ...packet,
    signatures: Array.from(
      { length: 257 },
      (_, i) => packet.signatures[i % packet.signatures.length],
    ),
  };
  const rejected = await uploadHeader({
    wallet,
    packet: oversized,
    target: 5,
    trusted: 3,
    record,
  });
  assert.equal(rejected.assembly.stage, "unsupported");
  assert.match(JSON.stringify(rejected.assembly.error), /at most 256/);
  assert.equal(rejected.transactions, 0);
  assert.equal(await wallet.connection.getBalance(wallet.publicKey), balance);
  return {
    passed: true,
    signatures: 100,
    batches: 2,
    assemblyCompute: split.assembly.compute,
    accounts: split.assembly.accounts,
    oversizedRejected: rejected.assembly.error,
  };
}

// An authentic header older than the latest fills in its consensus state but
// must not lower latest_height (ibc-go behavior).
async function testLatestHeight(
  wallet: AnchorProvider,
  record: (value: unknown) => Promise<void>,
  packet: (target: number, trusted: number) => Promise<Packet>,
) {
  const client = new Program<Ics07Tendermint>(clientIdl, wallet);
  const latest = async () =>
    (await client.account.clientState.fetch(pda("client", CLIENT_ID)))
      .latestHeight;
  // Sign the older header first so header times increase with height.
  const older = await packet(3, 1);
  await sleep(1100);
  const newer = await packet(5, 1);
  const upload = (value: Packet, target: number) =>
    uploadHeader({ wallet, packet: value, target, trusted: 1, record });
  assert.equal((await upload(newer, 5)).assembly.error, null);
  assert.equal((await latest()).revisionHeight.toNumber(), 5);
  const filled = await upload(older, 3);
  assert.equal(filled.assembly.error, null, "older header rejected");
  const after = await latest();
  assert.equal(after.revisionNumber.toNumber(), 1);
  assert.equal(
    after.revisionHeight.toNumber(),
    5,
    "older header lowered latest_height",
  );
  assert.deepEqual(
    (
      await client.account.consensusStateStore.fetch(
        pda("consensus_state", CLIENT_ID, LE64(3n)),
      )
    ).consensusState,
    older.consensus,
  );
  await assertClientActive(wallet, client, "akash-scale-1");
  return {
    passed: true,
    latestAfterNewer: 5,
    olderHeaderStored: 3,
    latestAfterOlder: after.revisionHeight.toNumber(),
    olderAssembly: filled.assembly.signature,
  };
}

async function fixture({
  target,
  trusted,
  signers,
  distribution,
  validators = 100,
  rootTag = "local scale fixture",
}: {
  target: number;
  trusted: number;
  signers: number;
  distribution: "equal" | "weighted";
  validators?: number;
  rootTag?: string;
}) {
  const input = object(
    JSON.parse(
      await readFile(
        join(REPO, "tools/header-encoder/fixtures/template.json"),
        "utf8",
      ),
    ),
  );
  const signed = object(input.signed_header),
    header = object(signed.header);
  header.height = String(target);
  header.time = new Date(Date.now() - 2000).toISOString();
  header.app_hash = createHash("sha256")
    .update(`${rootTag} ${target}`)
    .digest("hex")
    .toUpperCase();
  signed.header = header;
  signed.commit = { ...object(signed.commit), height: header.height };
  input.signed_header = signed;
  input.trusted_height = trusted;
  const raw = await new Promise<string>((resolve, reject) => {
    const child = spawn(
      join(REPO, "tools/header-encoder/target/debug/akt-header-encoder"),
      ["--fixture", String(validators), String(signers), distribution],
      { stdio: ["pipe", "pipe", "pipe"] },
    );
    let stdout = "",
      stderr = "";
    child.stdout.setEncoding("utf8").on("data", (data: string) => {
      stdout += data;
    });
    child.stderr.setEncoding("utf8").on("data", (data: string) => {
      stderr += data;
    });
    child.on("error", reject);
    child.on("close", (code) =>
      code === 0 ? resolve(stdout) : reject(new Error(stderr)),
    );
    child.stdin.end(JSON.stringify(input));
  });
  const value: unknown = JSON.parse(raw);
  return { value, packet: decodePacket(value) };
}

async function main() {
  const mode = process.argv[2] ?? "equal";
  const distribution = mode === "weighted" ? "weighted" : "equal";
  assert.ok(
    [
      "equal",
      "weighted",
      "receipts",
      "recovery",
      "misbehaviour",
      "expiry",
      "latest",
      "prune",
      "split",
      "open",
      "wide",
    ].includes(mode),
    "expected equal, weighted, receipts, recovery, misbehaviour, expiry, latest, prune, split, open or wide",
  );
  // `wide N` measures equal-power sets larger than 100; above 128 signatures
  // the uploader splits them across two batches.
  const wide = mode === "wide" ? Number(process.argv[3] ?? 150) : 0;
  assert.ok(
    mode !== "wide" || (Number.isInteger(wide) && wide > 100 && wide <= 256),
    "wide needs a validator count from 101 to 256",
  );
  const small = [
    "recovery",
    "misbehaviour",
    "expiry",
    "latest",
    "prune",
    "open",
  ].includes(mode);
  for (const port of [19999, 20000, 19901, 19900]) {
    const occupied = await new Promise<boolean>((resolve, reject) => {
      const socket = createConnection({ host: "127.0.0.1", port });
      socket.once("connect", () => {
        socket.destroy();
        resolve(true);
      });
      socket.once("error", (err: NodeJS.ErrnoException) => {
        socket.destroy();
        if (err.code === "ECONNREFUSED") resolve(false);
        else reject(err);
      });
      socket.setTimeout(1000, () => {
        socket.destroy();
        reject(new Error(`port ${port} probe timed out`));
      });
    });
    assert.ok(!occupied, `benchmark port ${port} already occupied`);
  }
  await mkdir(join(REPO, ".cache/scale"), { recursive: true, mode: 0o700 });
  const root = await mkdtemp(join(REPO, ".cache/scale/run-"));
  const validators = small ? 4 : wide || 100;
  const wideQuorum = Math.floor((2 * wide) / 3) + 1;
  console.log(`Isolated ${mode} run with ${validators} validators: ${root}`);
  const key = web3.Keypair.generate();
  await save(join(root, "solana-keypair.json"), Array.from(key.secretKey));
  const artifacts = join(REPO, ".deps/ibc-contracts/ibc-solana/target/deploy");
  const args = [
    "--ledger",
    join(root, "ledger"),
    "--bind-address",
    "127.0.0.1",
    "--rpc-port",
    "19999",
    "--gossip-port",
    "19901",
    "--dynamic-port-range",
    "19902-19950",
    "--faucet-port",
    "19900",
    "--transaction-account-lock-limit",
    "64",
    "--mint",
    web3.Keypair.generate().publicKey.toBase58(),
    "--upgradeable-program",
    CLIENT_ID.toBase58(),
    join(artifacts, "ics07_tendermint.so"),
    key.publicKey.toBase58(),
    "--upgradeable-program",
    ACCESS_ID.toBase58(),
    join(artifacts, "access_manager.so"),
    key.publicKey.toBase58(),
    "--quiet",
  ];
  const log = openSync(join(root, "validator.log"), "a", 0o600);
  const child = spawn("solana-test-validator", args, {
    stdio: ["ignore", log, log],
  });
  closeSync(log);
  const stopped = new Promise<void>((resolve, reject) => {
    child.once("exit", () => resolve());
    child.once("error", reject);
  });
  // Signal handlers only stop the process started by this invocation. Each run
  // gets a fresh directory; interrupted ledgers are retained for inspection.
  const interrupt = () => {
    child.kill("SIGTERM");
  };
  process.once("SIGINT", interrupt);
  process.once("SIGTERM", interrupt);
  try {
    const connection = new web3.Connection(
      "http://127.0.0.1:19999",
      "confirmed",
    );
    const wallet = new AnchorProvider(connection, new Wallet(key), {
      commitment: "confirmed",
      preflightCommitment: "confirmed",
    });
    const deadline = Date.now() + 60_000;
    for (;;) {
      assert.equal(
        child.exitCode,
        null,
        `benchmark validator exited; inspect ${root}/validator.log`,
      );
      assert.ok(Date.now() < deadline, "benchmark validator startup timed out");
      try {
        if ((await connection.getSlot()) > 1) break;
      } catch {
        /* RPC starts after genesis */
      }
      await sleep(250);
    }
    const funding = await connection.requestAirdrop(
      key.publicKey,
      100 * web3.LAMPORTS_PER_SOL,
    );
    await connection.confirmTransaction(funding, "confirmed");
    assert.ok(
      Number.isSafeInteger(await connection.getBalance(key.publicKey)),
      "payer balance exceeds exact JS integer range",
    );
    const manifest = {
      kind: "synthetic signed headers on real Solana runtime",
      mode,
      validators,
      distribution,
      votingPower:
        distribution === "equal"
          ? `${validators} validators at 10000 each`
          : "30 validators at 100000, 70 at 1000 (synthetic)",
      genesis: await connection.getGenesisHash(),
      runtime: await connection.getVersion(),
      accountLimit: 64,
      computeLimit: 1_400_000,
      clientProgram: CLIENT_ID.toBase58(),
      upstreamCommit: "dc78ade828b766b218fb6bcb5417fdaf8e553021",
      patchSha256: createHash("sha256")
        .update(await readFile(join(REPO, PATCHES[0])))
        .digest("hex"),
      patches: Object.fromEntries(
        await Promise.all(
          PATCHES.map(async (path) => [
            path,
            createHash("sha256")
              .update(await readFile(join(REPO, path)))
              .digest("hex"),
          ]),
        ),
      ),
      programSha256: {
        client: createHash("sha256")
          .update(await readFile(join(artifacts, "ics07_tendermint.so")))
          .digest("hex"),
        accessManager: createHash("sha256")
          .update(await readFile(join(artifacts, "access_manager.so")))
          .digest("hex"),
      },
      validatorArgs: args,
    };
    await save(join(root, "manifest.json"), manifest);
    const client = new Program<Ics07Tendermint>(clientIdl, wallet),
      access = new Program<AccessManager>(accessIdl, wallet);
    const accessManager = pda("access_manager", ACCESS_ID),
      clientState = pda("client", CLIENT_ID);
    await access.methods
      .initialize(key.publicKey)
      .accountsStrict({
        accessManager,
        payer: key.publicKey,
        systemProgram: web3.SystemProgram.programId,
        instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
        programData: programData(ACCESS_ID),
        authority: key.publicKey,
      })
      .rpc();
    await access.methods
      .grantRole(new BN(1), key.publicKey)
      .accountsStrict({
        accessManager,
        admin: key.publicKey,
        instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
      })
      .rpc();
    const initial = await fixture({
      target: 3,
      trusted: 1,
      signers: small ? 3 : wide ? wideQuorum : 67,
      distribution,
      validators,
    });
    await client.methods
      .initialize(
        {
          chainId: "akash-scale-1",
          trustLevelNumerator: new BN(1),
          trustLevelDenominator: new BN(3),
          trustingPeriod: new BN(
            mode === "expiry"
              ? 5
              : mode === "prune"
                ? PRUNE_TRUSTING_SECONDS
                : 86400,
          ),
          unbondingPeriod: new BN(1814400),
          maxClockDrift: new BN(10),
          frozenHeight: {
            revisionNumber: new BN(0),
            revisionHeight: new BN(0),
          },
          latestHeight: {
            revisionNumber: new BN(1),
            revisionHeight: new BN(1),
          },
        },
        {
          ...initial.packet.consensus,
          timestamp: new BN(
            (
              BigInt(
                Date.now() -
                  (mode === "prune" ? PRUNE_CHECKPOINT_AGE_SECONDS : 60) * 1000,
              ) * 1_000_000n
            ).toString(),
          ),
        },
        ACCESS_ID,
      )
      .accountsStrict({
        clientStateAccount: clientState,
        consensusStateStore: pda("consensus_state", CLIENT_ID, LE64(1n)),
        appState: pda("app_state", CLIENT_ID),
        payer: key.publicKey,
        systemProgram: web3.SystemProgram.programId,
        programData: programData(CLIENT_ID),
        authority: key.publicKey,
      })
      .rpc();
    const results = [];
    const cases =
      mode === "receipts" || mode === "split" || small
        ? []
        : wide
          ? [
              { name: "quorum", signers: wideQuorum },
              { name: "full-commit", signers: wide },
              { name: "below-quorum", signers: wideQuorum - 1 },
            ]
          : distribution === "equal"
            ? [
                { name: "quorum", signers: 67 },
                { name: "full-commit", signers: 100 },
                { name: "below-quorum", signers: 66 },
              ]
            : [
                { name: "quorum", signers: 21 },
                { name: "full-commit", signers: 100 },
                { name: "below-quorum", signers: 20 },
                { name: "tampered-root", signers: 21 },
              ];
    for (const [index, scenario] of cases.entries()) {
      const { signers, name } = scenario;
      const target = 3 + index * 2;
      const trusted = (
        await client.account.clientState.fetch(clientState)
      ).latestHeight.revisionHeight.toNumber();
      const { value, packet } = await fixture({
        target,
        trusted,
        signers,
        distribution,
        validators,
      });
      if (name === "tampered-root") {
        const offset = packet.bytes.indexOf(Buffer.from(packet.consensus.root));
        assert.ok(offset >= 0);
        packet.bytes[offset] ^= 1;
      }
      await save(join(root, `fixture-${name}.json`), {
        ...object(value),
        header: packet.bytes.toString("hex"),
      });
      const chunks = Math.ceil(packet.bytes.length / 900);
      const preverify = signers;
      console.log(
        JSON.stringify({
          event: "case-start",
          name,
          signers,
          bytes: packet.bytes.length,
          chunks,
          preverify,
          allPreverifiedAccounts: 10 + chunks + 1,
        }),
      );
      const result = await uploadHeader({
        wallet,
        packet,
        target,
        trusted,
        record: (value) =>
          appendFile(
            join(root, "transactions.jsonl"),
            JSON.stringify(value) + "\n",
            { mode: 0o600 },
          ),
      });
      results.push({ name, signers, ...result });
      await save(join(root, "results.json"), { manifest, results });
      if (name === "below-quorum" || name === "tampered-root")
        assert.notEqual(
          result.assembly.error,
          null,
          `${name} must never authenticate`,
        );
      // Wide runs are measurements: a commit may exceed compute or accounts.
      else if (!wide)
        assert.equal(result.assembly.error, null, `${name} regressed`);
      console.log(
        JSON.stringify({
          event: "case-result",
          signers,
          error: result.assembly.error,
          compute: result.assembly.compute,
          costs: result.costs,
        }),
      );
    }
    if (mode === "expiry")
      await save(
        join(root, "expiry-tests.json"),
        await testExpiry(wallet, initial.packet, root),
      );
    if (mode === "misbehaviour") {
      const other = await fixture({
        target: 3,
        trusted: 1,
        signers: 3,
        validators: 4,
        distribution: "equal",
        rootTag: "conflicting fork",
      });
      await save(
        join(root, "misbehaviour-tests.json"),
        await testMisbehaviour(wallet, initial.packet, other.packet),
      );
    }
    if (mode === "recovery") {
      await save(
        join(root, "recovery-tests.json"),
        await testRecovery(
          wallet,
          root,
          async (target, trusted) =>
            (
              await fixture({
                target,
                trusted,
                signers: 3,
                validators: 4,
                distribution: "equal",
              })
            ).packet,
        ),
      );
    }
    if (mode === "latest")
      await save(
        join(root, "latest-height-tests.json"),
        await testLatestHeight(
          wallet,
          (value) =>
            appendFile(
              join(root, "transactions.jsonl"),
              JSON.stringify(value) + "\n",
              { mode: 0o600 },
            ),
          async (target, trusted) =>
            (
              await fixture({
                target,
                trusted,
                signers: 3,
                validators: 4,
                distribution: "equal",
              })
            ).packet,
        ),
      );
    if (mode === "prune")
      await save(
        join(root, "prune-tests.json"),
        await testPrune(
          wallet,
          (value) =>
            appendFile(
              join(root, "transactions.jsonl"),
              JSON.stringify(value) + "\n",
              { mode: 0o600 },
            ),
          async (target, trusted) =>
            (
              await fixture({
                target,
                trusted,
                signers: 3,
                validators: 4,
                distribution: "equal",
              })
            ).packet,
        ),
      );
    if (mode === "open") {
      const conflicting = await fixture({
        target: 3,
        trusted: 1,
        signers: 3,
        validators: 4,
        distribution: "equal",
        rootTag: "conflicting fork",
      });
      await save(
        join(root, "open-relaying-tests.json"),
        await testOpenRelaying(
          wallet,
          (value) =>
            appendFile(
              join(root, "transactions.jsonl"),
              JSON.stringify(value) + "\n",
              { mode: 0o600 },
            ),
          async (target, trusted) =>
            (
              await fixture({
                target,
                trusted,
                signers: 3,
                validators: 4,
                distribution: "equal",
              })
            ).packet,
          (signer) =>
            testMisbehaviour(signer, initial.packet, conflicting.packet),
        ),
      );
    }
    if (mode === "split")
      await save(
        join(root, "split-tests.json"),
        await testBatchSplit(
          wallet,
          (value) =>
            appendFile(
              join(root, "transactions.jsonl"),
              JSON.stringify(value) + "\n",
              { mode: 0o600 },
            ),
          (
            await fixture({
              target: 3,
              trusted: 1,
              signers: 100,
              distribution: "equal",
            })
          ).packet,
        ),
      );
    if (mode === "receipts") {
      console.log(
        "Testing batch writer authorization, malformed inputs and capacity...",
      );
      await save(
        join(root, "batch-tests.json"),
        await testSignatureBatch(wallet),
      );
    }
    const evidence =
      new Map([
        ["recovery", "recovery-tests.json"],
        ["receipts", "batch-tests.json"],
        ["misbehaviour", "misbehaviour-tests.json"],
        ["expiry", "expiry-tests.json"],
        ["latest", "latest-height-tests.json"],
        ["prune", "prune-tests.json"],
        ["split", "split-tests.json"],
        ["open", "open-relaying-tests.json"],
      ]).get(mode) ?? "results.json";
    console.log(`Run complete; evidence: ${root}/${evidence}`);
  } finally {
    child.kill("SIGTERM");
    let timer: NodeJS.Timeout | undefined;
    await Promise.race([
      stopped,
      new Promise<void>((resolve) => {
        timer = setTimeout(() => {
          child.kill("SIGKILL");
          resolve();
        }, 10_000);
      }),
    ]);
    if (timer) clearTimeout(timer);
    await stopped;
    process.removeListener("SIGINT", interrupt);
    process.removeListener("SIGTERM", interrupt);
  }
}
// web3.js can retain a reconnect timer after its owned RPC has stopped.
main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
