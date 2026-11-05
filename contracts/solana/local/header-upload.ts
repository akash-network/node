import assert from "node:assert/strict";
import { join } from "node:path";
import { atomicJson, optionalJson } from "./durable";
import {
  sendTransaction,
  resumeTransaction,
  type TransactionResult,
} from "./transactions";
import { createHash } from "node:crypto";
import { AnchorProvider, BN, web3 } from "@coral-xyz/anchor";
import { LE64, pda, object, hex, sleep } from "./common";
import {
  LOCAL_LIGHT_CLIENT,
  clientProgram,
  type LightClientIds,
} from "./light-client";

export function decodePacket(value: unknown, chainId = "akash-scale-1") {
  const packet = object(value),
    consensus = object(packet.consensus);
  assert.equal(packet.chainId, chainId);
  assert.ok(
    typeof consensus.timestamp === "string" &&
      /^[0-9]+$/.test(consensus.timestamp),
  );
  assert.ok(Array.isArray(packet.signatures));
  return {
    bytes: hex(packet.header),
    consensus: {
      timestamp: new BN(consensus.timestamp),
      root: Array.from(hex(consensus.root, 32)),
      nextValidatorsHash: Array.from(hex(consensus.nextValidatorsHash, 32)),
    },
    signatures: packet.signatures.map((value: unknown) => {
      const sig = object(value);
      return {
        signatureHash: Array.from(hex(sig.hash, 32)),
        pubkey: Array.from(hex(sig.pubkey, 32)),
        msg: hex(sig.message),
        signature: Array.from(hex(sig.signature, 64)),
      };
    }),
  };
}

// The patched verifier reads at most 256 hashes from 128-hash batches.
export const SIGNATURE_BATCH_CAPACITY = 128;
export const MAX_SIGNATURE_BATCHES = 2;

// A header the verifier cannot process within Solana's limits. Sending the
// same commit again fails the same way.
export class HeaderUnsupported extends Error {}

// Assembly that ran out of compute or heap failed on the header's size, not
// on its validity or the network.
export function assemblyExhaustion(
  result: { error: unknown; logs: string[] },
  program: web3.PublicKey,
) {
  if (result.error === null) return undefined;
  const failure = `Program ${program.toBase58()} failed: `;
  for (const line of result.logs) {
    if (!line.startsWith(failure)) continue;
    if (/exceeded CUs meter|Computational budget exceeded/.test(line))
      return "assembly exhausted the compute budget";
    if (/heap section|out of memory/.test(line))
      return "assembly exhausted the heap";
  }
  return undefined;
}

// Shared transport, exercised by the 100-validator SBF benchmark and live relayer.
export async function uploadHeader({
  wallet,
  packet,
  target,
  trusted,
  record,
  journalDirectory,
  beforeUpload,
  signaturesPerBatch = SIGNATURE_BATCH_CAPACITY,
  lightClient = LOCAL_LIGHT_CLIENT,
}: {
  wallet: AnchorProvider;
  packet: ReturnType<typeof decodePacket>;
  target: number;
  trusted: number;
  record: (value: unknown) => Promise<void>;
  journalDirectory?: string;
  beforeUpload?: () => Promise<void>;
  // Tests lower this to exercise multi-batch uploads with small fixtures.
  signaturesPerBatch?: number;
  // The client the bridge pins; fresh local networks use the default.
  lightClient?: LightClientIds;
}) {
  const clientId = lightClient.program;
  const client = clientProgram(wallet, clientId);
  const clientState = pda("client", clientId),
    appState = pda("app_state", clientId);
  const accessManager = pda("access_manager", lightClient.accessManager);
  const chunkSize = 900,
    count = Math.ceil(packet.bytes.length / chunkSize);
  const preverify = packet.signatures.length;
  assert.ok(
    Number.isInteger(signaturesPerBatch) &&
      signaturesPerBatch > 0 &&
      signaturesPerBatch <= SIGNATURE_BATCH_CAPACITY,
  );
  const batchCount = Math.max(1, Math.ceil(preverify / signaturesPerBatch));
  // A packet beyond these limits never fits. Reject it before any on-chain
  // action and without throwing, so the caller can drop the job instead of
  // retrying the same packet forever.
  const unsupported = async (reason: string) => {
    await record({ event: "unsupported-header", target, reason });
    const assembly: TransactionResult = {
      stage: "unsupported",
      signature: "",
      bytes: 0,
      accounts: 0,
      fee: 0,
      compute: 0,
      error: { unsupportedHeader: reason },
      logs: [],
    };
    return {
      headerBytes: packet.bytes.length,
      chunks: count,
      commitSignatures: packet.signatures.length,
      preverified: 0,
      signaturesWithoutReceipt: packet.signatures.length,
      assembly,
      transactions: 0,
      maxTransactionBytes: 0,
      totalCompute: 0,
      costs: {
        fees: 0,
        temporaryRentReturned: 0,
        permanentRent: 0,
        lookupRentAwaitingCooldown: 0,
        balanceDelta: 0,
      },
      lookupTable: undefined,
    };
  };
  if (count < 1 || count > 255)
    return unsupported(
      `header is ${packet.bytes.length} bytes; the uploader supports 1 to 255 chunks of ${chunkSize} bytes`,
    );
  if (batchCount > MAX_SIGNATURE_BATCHES)
    return unsupported(
      `commit has ${preverify} signatures; the verifier accepts at most ${MAX_SIGNATURE_BATCHES * SIGNATURE_BATCH_CAPACITY} preverified signatures per header. Extend the ICS07 signature-batch patch before the validator set grows past this`,
    );
  const chunks = Array.from(
    { length: count },
    (_, i) =>
      web3.PublicKey.findProgramAddressSync(
        [
          Buffer.from("header_chunk"),
          wallet.publicKey.toBuffer(),
          LE64(BigInt(target)),
          Buffer.from([i]),
        ],
        clientId,
      )[0],
  );
  const session = createHash("sha256")
    .update("akt-header-upload-v1")
    .update(clientId.toBuffer())
    .update(wallet.publicKey.toBuffer())
    .update(packet.bytes)
    .digest();
  // Batch 0 keeps the original session, so journals from single-batch uploads
  // still resolve. Signature i goes to batch floor(i / signaturesPerBatch).
  const sessions = Array.from({ length: batchCount }, (_, k) =>
    k === 0
      ? session
      : createHash("sha256")
          .update("akt-header-upload-batch-v1")
          .update(session)
          .update(Buffer.from([k]))
          .digest(),
  );
  const sigs = sessions.map(
    (batchSession) =>
      web3.PublicKey.findProgramAddressSync(
        [Buffer.from("sig_batch"), wallet.publicKey.toBuffer(), batchSession],
        clientId,
      )[0],
  );
  const remaining = [...chunks, ...sigs].map((pubkey) => ({
    pubkey,
    isWritable: true,
    isSigner: false,
  }));
  const assembly = await client.methods
    .assembleAndUpdateClient(new BN(target), count, new BN(trusted))
    .accountsStrict({
      clientState,
      appState,
      accessManager,
      trustedConsensusState: pda(
        "consensus_state",
        clientId,
        LE64(BigInt(trusted)),
      ),
      newConsensusStateStore: pda(
        "consensus_state",
        clientId,
        LE64(BigInt(target)),
      ),
      submitter: wallet.publicKey,
      systemProgram: web3.SystemProgram.programId,
      instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
    })
    .remainingAccounts(remaining)
    .instruction();
  const compute = [
    web3.ComputeBudgetProgram.setComputeUnitLimit({ units: 1_400_000 }),
    web3.ComputeBudgetProgram.requestHeapFrame({ bytes: 256 * 1024 }),
  ];
  const addresses = [
    ...new Map(
      [
        ...assembly.keys.map((k) => k.pubkey),
        clientId,
        web3.ComputeBudgetProgram.programId,
      ].map((key) => [key.toBase58(), key]),
    ).values(),
  ];
  if (addresses.length > 64)
    return unsupported(
      `assembly needs ${addresses.length} accounts; conservative limit is 64`,
    );
  const before = await wallet.connection.getBalance(wallet.publicKey);
  assert.ok(
    journalDirectory !== undefined || Number.isSafeInteger(before),
    "use a separately funded payer with an exact JS integer balance",
  );
  const transactions: TransactionResult[] = [];
  let lookup: web3.AddressLookupTableAccount | undefined;
  let lookupAddress: web3.PublicKey | undefined;
  let temporaryRent = 0;
  const identity = {
    version: 1,
    genesis: await wallet.connection.getGenesisHash(),
    payer: wallet.publicKey.toBase58(),
    client: clientId.toBase58(),
    target,
    trusted,
    session: session.toString("hex"),
  };
  const journalPath = journalDirectory
    ? join(journalDirectory, `${identity.session}.json`)
    : undefined;
  const previous = journalPath ? await optionalJson(journalPath) : undefined;
  let lookupSlot: number | undefined;
  let terminal: TransactionResult | undefined;
  if (previous !== undefined) {
    const saved = object(previous);
    for (const [key, value] of Object.entries(identity))
      assert.equal(saved[key], value, `upload journal ${key} mismatch`);
    if (saved.assembly !== undefined) {
      const done = object(saved.assembly);
      assert.equal(done.stage, "assembly");
      assert.ok(
        typeof done.signature === "string" &&
          (done.error === null || typeof done.error === "object"),
      );
      terminal = {
        stage: "assembly",
        signature: done.signature,
        error: done.error,
        bytes: 0,
        accounts: 0,
        fee: 0,
        compute: 0,
        logs: [],
      };
    }
    if (saved.lookup !== undefined) {
      assert.ok(
        typeof saved.lookup === "string" &&
          typeof saved.lookupSlot === "number" &&
          Number.isSafeInteger(saved.lookupSlot),
      );
      lookupAddress = new web3.PublicKey(saved.lookup);
      lookupSlot = saved.lookupSlot;
    }
  }
  const persist = async () => {
    if (journalPath)
      await atomicJson(journalPath, {
        ...(previous === undefined ? {} : object(previous)),
        ...identity,
        lookup: lookupAddress?.toBase58(),
        lookupSlot,
        assembly: terminal,
      });
  };
  await persist();
  const targetAddress = pda("consensus_state", clientId, LE64(BigInt(target)));
  const pendingPath = journalDirectory
    ? join(journalDirectory, `${identity.session}.pending.json`)
    : undefined;
  async function observe(entry: TransactionResult) {
    transactions.push(entry);
    if (entry.stage === "assembly") {
      terminal = entry;
      await persist();
    }
    await record({ event: "transaction", ...entry });
  }
  if (pendingPath) await resumeTransaction(wallet, pendingPath, observe);
  const already = await client.account.consensusStateStore.fetchNullable(
    targetAddress,
    "finalized",
  );
  if (already)
    assert.deepEqual(
      already.consensusState,
      packet.consensus,
      "stored target differs from upload; refusing to skip verification",
    );
  async function send(
    stage: string,
    instructions: web3.TransactionInstruction[],
    useLookup = false,
  ) {
    const entry = await sendTransaction({
      wallet,
      stage,
      instructions,
      lookup: useLookup ? lookup : undefined,
      pendingPath,
      observe,
      submitted: (signature) =>
        record({ event: "submitted", stage, signature }),
    });
    if (stage !== "assembly")
      assert.equal(
        entry.error,
        null,
        `${stage} failed: ${JSON.stringify(entry)}`,
      );
    return entry;
  }
  let assemblyResult: TransactionResult | undefined;
  try {
    if (!already && !terminal) {
      await beforeUpload?.();
      if (!lookupAddress) {
        lookupSlot = await wallet.connection.getSlot("finalized");
        lookupAddress = web3.AddressLookupTableProgram.createLookupTable({
          authority: wallet.publicKey,
          payer: wallet.publicKey,
          recentSlot: lookupSlot,
        })[1];
        await persist(); // Address is durable before the create transaction.
      }
      assert.ok(lookupSlot !== undefined);
      let fetched =
        await wallet.connection.getAddressLookupTable(lookupAddress);
      if (!fetched.value) {
        // A never-created table can only reuse its slot while that slot is recent.
        if ((await wallet.connection.getSlot("finalized")) - lookupSlot > 512) {
          lookupSlot = await wallet.connection.getSlot("finalized");
          lookupAddress = web3.AddressLookupTableProgram.createLookupTable({
            authority: wallet.publicKey,
            payer: wallet.publicKey,
            recentSlot: lookupSlot,
          })[1];
          await persist();
        }
        await send("lookup-create", [
          web3.AddressLookupTableProgram.createLookupTable({
            authority: wallet.publicKey,
            payer: wallet.publicKey,
            recentSlot: lookupSlot,
          })[0],
        ]);
        fetched = await wallet.connection.getAddressLookupTable(lookupAddress);
      }
      assert.ok(fetched.value);
      assert.equal(
        fetched.value.state.deactivationSlot,
        18446744073709551615n,
        "upload lookup was deactivated before assembly",
      );
      assert.ok(fetched.value.state.authority?.equals(wallet.publicKey));
      const populated = fetched.value.state.addresses;
      assert.ok(populated.length <= addresses.length);
      populated.forEach((key, i) =>
        assert.ok(key.equals(addresses[i]), "lookup address mismatch"),
      );
      for (let i = populated.length; i < addresses.length; i += 20)
        await send("lookup-extend", [
          web3.AddressLookupTableProgram.extendLookupTable({
            lookupTable: lookupAddress,
            authority: wallet.publicKey,
            payer: wallet.publicKey,
            addresses: addresses.slice(i, i + 20),
          }),
        ]);
      lookup =
        (await wallet.connection.getAddressLookupTable(lookupAddress)).value ??
        undefined;
      assert.ok(lookup);
      const deadline = Date.now() + 15_000;
      while (
        (await wallet.connection.getSlot()) <= lookup.state.lastExtendedSlot
      ) {
        assert.ok(Date.now() < deadline, "lookup table did not warm up");
        await sleep(100);
      }
      for (const [k, signatureBatch] of sigs.entries()) {
        const batchInfo =
          await wallet.connection.getAccountInfo(signatureBatch);
        if (!batchInfo)
          await send("signature-batch-create", [
            await client.methods
              .initializeSignatureBatch(Array.from(sessions[k]))
              .accountsStrict({
                signatureBatch,
                appState,
                accessManager,
                submitter: wallet.publicKey,
                instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
                systemProgram: web3.SystemProgram.programId,
              })
              .instruction(),
          ]);
        else assert.ok(batchInfo.owner.equals(clientId), "foreign batch owner");
        const existingBatch =
          await client.account.signatureBatch.fetch(signatureBatch);
        assert.equal(existingBatch.version, 1);
        assert.ok(existingBatch.submitter.equals(wallet.publicKey));
        assert.deepEqual(existingBatch.session, Array.from(sessions[k]));
        const existingHashes = new Set(
          existingBatch.hashes.map((h) => Buffer.from(h).toString("hex")),
        );
        const end = Math.min(preverify, (k + 1) * signaturesPerBatch);
        for (let i = k * signaturesPerBatch; i < end; i++) {
          const signature = packet.signatures[i];
          if (
            existingHashes.has(
              Buffer.from(signature.signatureHash).toString("hex"),
            )
          )
            continue;
          const ix = await client.methods
            .preVerifySignatureBatch(signature)
            .accountsStrict({
              instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
              signatureBatch,
              appState,
              accessManager,
              submitter: wallet.publicKey,
            })
            .instruction();
          await send("signature", [
            web3.Ed25519Program.createInstructionWithPublicKey({
              publicKey: Uint8Array.from(signature.pubkey),
              message: signature.msg,
              signature: Uint8Array.from(signature.signature),
            }),
            ix,
          ]);
        }
      }
      for (let i = 0; i < count; i++) {
        const chunkData = packet.bytes.subarray(
          i * chunkSize,
          (i + 1) * chunkSize,
        );
        const existing = await wallet.connection.getAccountInfo(chunks[i]);
        if (existing) {
          assert.ok(existing.owner.equals(clientId), "foreign chunk owner");
          const decoded = await client.account.headerChunk.fetch(chunks[i]);
          assert.ok(decoded.submitter.equals(wallet.publicKey));
          assert.deepEqual(
            Buffer.from(decoded.chunkData),
            chunkData,
            "existing chunk differs from durable header",
          );
          continue;
        }
        const ix = await client.methods
          .uploadHeaderChunk({
            targetHeight: new BN(target),
            chunkIndex: i,
            chunkData: packet.bytes.subarray(
              i * chunkSize,
              (i + 1) * chunkSize,
            ),
          })
          .accountsStrict({
            chunk: chunks[i],
            clientState,
            appState,
            accessManager,
            submitter: wallet.publicKey,
            instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
            systemProgram: web3.SystemProgram.programId,
          })
          .instruction();
        await send("chunk", [ix], true);
      }
      const temporary = await wallet.connection.getMultipleAccountsInfo([
        ...chunks,
        ...sigs,
      ]);
      temporaryRent = temporary.reduce(
        (total, account) => total + (account?.lamports ?? 0),
        0,
      );
      assemblyResult = await send("assembly", [...compute, assembly], true);
    } else {
      assemblyResult =
        !already && terminal
          ? terminal
          : {
              stage: "already-authenticated",
              signature: "",
              bytes: 0,
              accounts: 0,
              fee: 0,
              compute: 0,
              error: null,
              logs: [],
            };
      await record({ event: "already-authenticated", target });
    }
    const exhausted = assemblyExhaustion(assemblyResult, clientId);
    if (exhausted) {
      await record({ event: "unsupported-header", target, reason: exhausted });
      assemblyResult = {
        ...assemblyResult,
        error: { unsupportedHeader: exhausted, cause: assemblyResult.error },
      };
    }
    const state = await client.account.clientState.fetch(
      clientState,
      "finalized",
    );
    const stored = await client.account.consensusStateStore.fetchNullable(
      pda("consensus_state", clientId, LE64(BigInt(target))),
      "finalized",
    );
    if (stored) {
      assert.ok(state.latestHeight.revisionHeight.toNumber() >= target);
      assert.deepEqual(
        stored.consensusState,
        packet.consensus,
        "authenticated target differs from requested header",
      );
      if (assemblyResult.error !== null) {
        await record({
          event: "authenticated-concurrently",
          target,
          localAssembly: assemblyResult.signature,
        });
        assemblyResult = {
          ...assemblyResult,
          stage: "authenticated-concurrently",
          error: null,
        };
      }
    } else {
      assert.notEqual(
        assemblyResult.error,
        null,
        "successful finalized assembly has no authenticated target",
      );
      if (!journalDirectory)
        assert.equal(
          state.latestHeight.revisionHeight.toNumber(),
          trusted,
          "failed update advanced trust",
        );
      assert.equal(stored, null, "failed update stored a consensus root");
    }
  } finally {
    // Unknown network outcomes retain their upload and journal. Restart reads
    // authenticated state first; it must not race a still-valid assembly tx.
    if (assemblyResult || !journalDirectory) {
      for (let i = 0; i < remaining.length; i += 20) {
        const ix = await client.methods
          .cleanupIncompleteUpload()
          .accountsStrict({ submitter: wallet.publicKey })
          .remainingAccounts(remaining.slice(i, i + 20))
          .instruction();
        await send("cleanup", [ix]);
      }
      assert.ok(
        (
          await wallet.connection.getMultipleAccountsInfo(
            [...chunks, ...sigs],
            "finalized",
          )
        ).every((a) => a === null),
        "temporary accounts remain",
      );
      if (
        lookupAddress &&
        (await wallet.connection.getAddressLookupTable(lookupAddress)).value
          ?.state.deactivationSlot === 18446744073709551615n
      )
        await send("lookup-deactivate", [
          web3.AddressLookupTableProgram.deactivateLookupTable({
            lookupTable: lookupAddress,
            authority: wallet.publicKey,
          }),
        ]);
    }
  }
  assert.ok(assemblyResult);
  const permanentRent =
    (
      await wallet.connection.getAccountInfo(
        pda("consensus_state", clientId, LE64(BigInt(target))),
      )
    )?.lamports ?? 0;
  const lookupRent =
    (lookupAddress
      ? (await wallet.connection.getAccountInfo(lookupAddress))?.lamports
      : 0) ?? 0;
  const fees = transactions.reduce((sum, tx) => sum + tx.fee, 0);
  const balanceDelta =
    before - (await wallet.connection.getBalance(wallet.publicKey));
  if (!journalDirectory)
    assert.equal(
      balanceDelta,
      fees + permanentRent + lookupRent,
      "isolated payer cost does not reconcile",
    );
  return {
    headerBytes: packet.bytes.length,
    chunks: count,
    commitSignatures: packet.signatures.length,
    preverified: preverify,
    signaturesWithoutReceipt: packet.signatures.length - preverify,
    assembly: assemblyResult,
    transactions: transactions.length,
    maxTransactionBytes: Math.max(...transactions.map((tx) => tx.bytes)),
    totalCompute: transactions.reduce((n, tx) => n + tx.compute, 0),
    costs: {
      fees,
      temporaryRentReturned: temporaryRent,
      permanentRent,
      lookupRentAwaitingCooldown: lookupRent,
      balanceDelta,
    },
    lookupTable: lookupAddress?.toBase58(),
  };
}
