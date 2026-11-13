import {
  Sink_Status as Status,
  type TransferState,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { AnchorProvider, BN, Program, Wallet, web3 } from "@coral-xyz/anchor";
import {
  ASSOCIATED_TOKEN_PROGRAM_ID,
  TOKEN_PROGRAM_ID,
  getAssociatedTokenAddressSync,
} from "@solana/spl-token";
import { z } from "zod";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import {
  appendFile,
  readFile,
  readdir,
  rename,
  unlink,
} from "node:fs/promises";
import { basename, dirname, join, resolve } from "node:path";
import { parseArgs } from "node:util";
import idl from "../../../.cache/solana/target/idl/akt.json";
import type { Akt } from "../../../.cache/solana/target/types/akt";
import { AkashClient } from "../reporter/akash";
import { decodeRecord } from "../reporter/solana";
import { parseSinkId, type SinkId } from "../shared/id";
import {
  anchorId,
  encodeRecord,
  authorizedSettlement,
  settlementProofValue,
  encodeMembershipProof,
  PROOF_HEADER_SIZE,
  matchesAnchorId,
  proofAddress,
  proofNonce,
  statusAddress,
} from "../shared/proof";
import { transferKey } from "../shared/sink";
import {
  newProofJob,
  parseProofJob,
  sameProof,
  type ProofJob,
} from "./proof-job";
import { assertClientActive } from "./client-status";
import {
  CLIENT_ID,
  hex,
  object,
  pda,
  programData,
  provider,
  save,
  sleep,
} from "./common";
import { pruneExpiredConsensusStates } from "./consensus-prune";
import { atomicJson, optionalJson, withUploadLock } from "./durable";
import {
  HeaderUnsupported,
  SIGNATURE_BATCH_CAPACITY,
  decodePacket,
  uploadHeader,
} from "./header-upload";
import {
  LOCAL_LIGHT_CLIENT,
  accessProgram,
  clientProgram,
  discoverLightClient,
  lightClientAccounts,
  lightClientProblem,
  readRelayerAccess,
  type LightClient,
  type RelayerAccess,
} from "./light-client";
import { reclaimLookups } from "./lookup-cleanup";
import {
  discoverTransfers,
  openTransfers,
  type DiscoveryCursor,
} from "./open-transfers";
import {
  loadPayer,
  localRelayConfig,
  parseRelayConfig,
  type RelayConfig,
} from "./relay-config";
import {
  AlertSchedule,
  RetrySchedule,
  STATUS_BATCH,
  SpendLedger,
  finalCheck,
  headerRefresh,
  mintWait,
  recipientMints,
  relayOrder,
  selectCommitSignatures,
  unbackedMint,
  type CommitSignature,
  type SetMember,
} from "./relay-policy";
import {
  resumeTransaction,
  sendTransaction,
  type TransactionResult,
} from "./transactions";

type AkashNode = Pick<
  RelayConfig,
  "akashRpc" | "akashChainId" | "headerEncoder" | "maxHeaderSignatures"
>;

function decimal(value: unknown) {
  assert.ok(
    typeof value === "string" && /^(0|[1-9][0-9]*)$/.test(value),
    "decimal string required",
  );
  return BigInt(value);
}
async function rpc(node: AkashNode, method: string, params: object) {
  const reply = await fetch(node.akashRpc, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params }),
    signal: AbortSignal.timeout(15_000),
  });
  assert.ok(reply.ok, `Akash RPC HTTP ${reply.status}`);
  const body = object(await reply.json());
  assert.ok(!body.error, `Akash RPC ${method} failed`);
  return object(body.result);
}
async function validators(node: AkashNode, height: number) {
  const values: unknown[] = [];
  for (let page = 1; page <= 10; page++) {
    const result = await rpc(node, "validators", {
      height: String(height),
      page: String(page),
      per_page: "100",
    });
    assert.equal(result.block_height, String(height));
    assert.ok(Array.isArray(result.validators));
    values.push(...result.validators);
    if (BigInt(values.length) === decimal(result.total)) return values;
  }
  throw new Error("validator pagination exceeded local bound");
}
async function headerPacket(
  node: AkashNode,
  trusted: number,
  target: number,
  revision: number,
  trustLevel?: { numerator: bigint; denominator: bigint },
) {
  const commit = await rpc(node, "commit", { height: String(target) });
  assert.equal(commit.canonical, true, "noncanonical Akash commit");
  const signedHeader = object(commit.signed_header);
  const signed = object(signedHeader.commit);
  const input = {
    signed_header: signedHeader,
    validators: await validators(node, target),
    trusted_validators: await validators(node, trusted + 1),
    trusted_height: trusted,
    revision,
  };
  // Large commits exceed compute or heap at assembly. Submit only the
  // signatures the verifier needs; the rest become Absent.
  assert.ok(Array.isArray(signed.signatures), "commit has no signatures");
  const subset = selectCommitSignatures({
    signatures: signed.signatures as CommitSignature[],
    validators: input.validators as SetMember[],
    // Adjacent updates check only the new set.
    trusted:
      target > trusted + 1 && trustLevel
        ? { validators: input.trusted_validators as SetMember[], ...trustLevel }
        : undefined,
    maxSignatures: node.maxHeaderSignatures,
    limit: SIGNATURE_BATCH_CAPACITY,
  });
  if ("unsupported" in subset) throw new HeaderUnsupported(subset.unsupported);
  if (subset.kept < subset.present) {
    signedHeader.commit = { ...signed, signatures: subset.signatures };
    console.log(
      JSON.stringify({
        event: "header-signatures-trimmed",
        target,
        present: subset.present,
        kept: subset.kept,
      }),
    );
  }
  const raw = await new Promise<string>((resolve, reject) => {
    const child = spawn(node.headerEncoder, [], {
      stdio: ["pipe", "pipe", "pipe"],
    });
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
      code === 0
        ? resolve(stdout)
        : reject(new Error(`header encoding failed: ${stderr}`)),
    );
    child.stdin.end(JSON.stringify(input));
  });
  const packet = object(JSON.parse(raw));
  assert.equal(packet.chainId, node.akashChainId);
  const consensus = object(packet.consensus);
  assert.equal(decimal(consensus.height), BigInt(target));
  assert.ok(Array.isArray(packet.signatures));
  return {
    bytes: hex(packet.header),
    consensus: {
      timestamp: new BN(decimal(consensus.timestamp).toString()),
      root: Array.from(hex(consensus.root, 32)),
      nextValidatorsHash: Array.from(hex(consensus.nextValidatorsHash, 32)),
    },
    blockHash: hex(consensus.blockHash, 32).toString("hex"),
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

// The spend limit stopped a transfer; the tick alerts instead of retrying.
class BudgetExhausted extends Error {}

// Without options this is the local or team stack: settings derive from
// local/common.ts and AKT_RELAY_* variables, and the payer is the stack key.
export async function relayContext(options?: {
  config: RelayConfig;
  wallet: AnchorProvider;
}) {
  const settings = options?.config ?? localRelayConfig(process.env);
  const wallet = options?.wallet ?? (await provider());
  const chainId = settings.akashChainId;
  const programId = settings.programId;
  const genesis = await wallet.connection.getGenesisHash();
  if (settings.solanaGenesisHash !== undefined)
    assert.equal(
      genesis,
      settings.solanaGenesisHash,
      "Solana RPC serves another cluster: genesis hash differs from solanaGenesisHash",
    );
  const program = new Program<Akt>(
    { ...idl, address: programId.toBase58() },
    wallet,
  );
  const akash = new AkashClient(
    settings.akashRpc,
    chainId,
    settings.maxAgeSeconds,
  );
  const at = (seed: string, suffix?: Buffer) => pda(seed, programId, suffix);
  const statusAt = (id: SinkId) => statusAddress(programId, id);
  const proofBufferAt = (key: Uint8Array) =>
    proofAddress(programId, wallet.publicKey, key);
  const config = at("config"),
    bridge = at("bridge"),
    mint = at("mint"),
    mintAuthority = at("mint_authority");
  const admin = {
    program: programId,
    programData: programData(programId),
    authority: wallet.publicKey,
  };
  const compute = () => [
    web3.ComputeBudgetProgram.setComputeUnitLimit({ units: 1_400_000 }),
    web3.ComputeBudgetProgram.requestHeapFrame({ bytes: 256 * 1024 }),
  ];
  const message = (error: unknown) =>
    error instanceof Error ? error.message : String(error);
  const log = (value: object) => console.log(JSON.stringify(value));
  const alert = (value: object) => console.error(JSON.stringify(value));

  async function timed<T>(
    stage: string,
    action: () => Promise<T>,
    id?: SinkId,
  ): Promise<T> {
    const started = performance.now();
    let completed = false;
    try {
      const result = await action();
      completed = true;
      return result;
    } finally {
      log({
        event: "relay-stage-duration",
        stage,
        id,
        elapsedMs: Math.round(performance.now() - started),
        completed,
      });
    }
  }

  // Failed transfers back off exponentially; each attempt can pay for several
  // proof transactions. Header failures back off separately and never block
  // relays that the latest verified height can already prove.
  const headerRetries = new RetrySchedule<bigint>(6000, 120_000);
  // An unsupported header fails the same way until the validator set or its
  // votes change, so it waits the longest transfer backoff.
  const unsupported = new RetrySchedule<bigint>(
    settings.retryMaxMs,
    settings.retryMaxMs,
  );
  let prunedAt = 0;

  // Governance can re-pin the bridge to another light client at any time.
  // Every pass re-reads the bridge account and follows it, unless the
  // operator pinned a program, in which case a different one stops paying.
  type Followed = LightClient &
    ReturnType<typeof lightClientAccounts> & {
      client: ReturnType<typeof clientProgram>;
    };
  const describe = (found: LightClient): Followed => ({
    ...found,
    ...lightClientAccounts(found),
    client: clientProgram(wallet, found.program),
  });
  let followed: Followed | undefined;
  const followAlerts = new AlertSchedule<bigint>(15 * 60_000);
  async function follow() {
    const found = await discoverLightClient(wallet.connection, programId);
    const problem = lightClientProblem(found, {
      chainId,
      pin: settings.lightClientProgram,
    });
    if (problem) {
      if (followAlerts.due(0n, Date.now()))
        alert({
          event: problem.event,
          lightClient: found.program.toBase58(),
          bridgeChainId: found.chainId,
          pinned: settings.lightClientProgram?.toBase58(),
          chainId,
          action: "stopped",
        });
      throw new Error(problem.message);
    }
    followAlerts.clear(0n);
    if (!followed)
      log({
        event: "light-client",
        program: found.program.toBase58(),
        accessManager: found.accessManager.toBase58(),
        chainId: found.chainId,
        pinned: settings.lightClientProgram !== undefined,
      });
    else if (!followed.program.equals(found.program)) {
      alert({
        event: "light-client-changed",
        previous: followed.program.toBase58(),
        current: found.program.toBase58(),
        accessManager: found.accessManager.toBase58(),
        slot: found.slot,
      });
      headerRetries.clear(0n);
      unsupported.clear(0n);
      prunedAt = 0;
    }
    if (
      !followed ||
      !followed.program.equals(found.program) ||
      !followed.accessManager.equals(found.accessManager)
    )
      followed = describe(found);
    return followed;
  }

  // Members update and prune. Open relaying lets any payer update but not
  // prune. Without either, the relayer proves only at verified heights.
  let access: RelayerAccess | undefined;
  async function relayerAccess(lc: Followed) {
    const current = await readRelayerAccess(wallet, lc);
    if (current !== access)
      (current === "none" ? alert : log)({
        event: "relayer-access",
        access: current,
        payer: wallet.publicKey.toBase58(),
        lightClient: lc.program.toBase58(),
        updates: current !== "none",
        prunes: current === "member",
      });
    access = current;
    return current;
  }

  // Only a fresh local network is bootstrapped, with the fixed local IDs.
  async function bootstrap() {
    assert.equal(options, undefined, "only the local stack bootstraps");
    const local = {
      ...LOCAL_LIGHT_CLIENT,
      ...lightClientAccounts(LOCAL_LIGHT_CLIENT),
    };
    const client = clientProgram(wallet, local.program);
    const manager = accessProgram(wallet, local.accessManager);
    const { clientState, appState, consensusAt } = local;
    const accessManager = local.accessManagerState;
    if (!(await manager.account.accessManager.fetchNullable(accessManager))) {
      await manager.methods
        .initialize(wallet.publicKey)
        .accountsStrict({
          accessManager,
          payer: wallet.publicKey,
          systemProgram: web3.SystemProgram.programId,
          instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
          programData: programData(local.accessManager),
          authority: wallet.publicKey,
        })
        .rpc();
      await manager.methods
        .grantRole(new BN(1), wallet.publicKey)
        .accountsStrict({
          accessManager,
          admin: wallet.publicKey,
          instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
        })
        .rpc();
    }
    if (!(await program.account.launchConfig.fetchNullable(config))) {
      await program.methods
        .initialize()
        .accountsStrict({
          ...admin,
          config,
          mint,
          mintAuthority,
          tokenProgram: TOKEN_PROGRAM_ID,
          systemProgram: web3.SystemProgram.programId,
        })
        .rpc();
    }
    if (!(await client.account.clientState.fetchNullable(clientState))) {
      let state = await akash.state();
      while (state.height < 4) {
        await sleep(500);
        state = await akash.state();
      }
      // Comet stores the canonical commit once the following block arrives.
      const checkpointHeight = state.height - 1;
      const packet = await headerPacket(
        settings,
        checkpointHeight - 1,
        checkpointHeight,
        1,
      );
      await client.methods
        .initialize(
          {
            chainId,
            trustLevelNumerator: new BN(1),
            trustLevelDenominator: new BN(3),
            trustingPeriod: new BN(86400),
            unbondingPeriod: new BN(1814400),
            maxClockDrift: new BN(10),
            frozenHeight: {
              revisionNumber: new BN(0),
              revisionHeight: new BN(0),
            },
            latestHeight: {
              revisionNumber: new BN(1),
              revisionHeight: new BN(checkpointHeight),
            },
          },
          packet.consensus,
          local.accessManager,
        )
        .accountsStrict({
          clientStateAccount: clientState,
          consensusStateStore: consensusAt(BigInt(checkpointHeight)),
          appState,
          payer: wallet.publicKey,
          systemProgram: web3.SystemProgram.programId,
          programData: programData(local.program),
          authority: wallet.publicKey,
        })
        .rpc();
      await save(join(settings.stateDir, "checkpoint.json"), {
        chainId,
        height: checkpointHeight,
        blockHash: packet.blockHash,
        root: Buffer.from(packet.consensus.root).toString("hex"),
      });
      log({
        event: "trusted-local-checkpoint",
        height: checkpointHeight,
        blockHash: packet.blockHash,
      });
    }
    const current = await client.account.clientState.fetch(clientState);
    assert.equal(current.chainId, chainId);
    const newBridge =
      !(await program.account.bridgeConfig.fetchNullable(bridge));
    if (newBridge) {
      // A retained test-validator ledger may start behind wall time after an
      // outage. Wait for its real clock to accept the existing checkpoint;
      // never change the checkpoint or relax the program's drift check.
      const consensus = await client.account.consensusStateStore.fetch(
        consensusAt(BigInt(current.latestHeight.revisionHeight.toString())),
      );
      const timestamp = BigInt(consensus.consensusState.timestamp.toString());
      const drift = BigInt(current.maxClockDrift.toString()) * 1_000_000_000n;
      const deadline = Date.now() + 540_000;
      let announced = false;
      for (;;) {
        const clock = await wallet.connection.getAccountInfo(
          web3.SYSVAR_CLOCK_PUBKEY,
          "confirmed",
        );
        assert.ok(
          clock && clock.data.length === 40,
          "Solana clock unavailable",
        );
        const now = clock.data.readBigInt64LE(32) * 1_000_000_000n;
        if (timestamp <= now + drift) break;
        assert.ok(
          Date.now() < deadline,
          "Solana clock has not reached the existing checkpoint; leave the validator running and retry",
        );
        if (!announced) {
          console.log(
            "Waiting for the retained Solana clock to reach the existing checkpoint.",
          );
          announced = true;
        }
        await sleep(1000);
      }
    }
    if (newBridge)
      await program.methods
        .configureBridge(chainId, {
          // 1,000,000 AKT per transfer and 10,000,000 AKT per UTC day.
          perTransferMax: new BN(1_000_000_000_000),
          dailyMintCap: new BN(10_000_000_000_000),
          vaults: [],
        })
        .accountsStrict({
          admin,
          config,
          bridge,
          lightClientProgram: local.program,
          clientState,
          latestConsensus: consensusAt(
            BigInt(current.latestHeight.revisionHeight.toString()),
          ),
          systemProgram: web3.SystemProgram.programId,
        })
        .rpc();
    // Restarting services must preserve an operator's pause. Auto-enable only
    // the bridge configured by this bootstrap invocation.
    if (newBridge)
      await program.methods
        .setBridgeEnabled(true)
        .accountsStrict({ admin, config, bridge })
        .rpc();
    log({
      event: "bridge-state",
      enabled: (await program.account.launchConfig.fetch(config)).bridgeEnabled,
      mint: mint.toBase58(),
      height: current.latestHeight.revisionHeight.toString(),
    });
  }

  const headerDirectory = join(
    settings.stateDir,
    "header-uploads-v2",
    genesis,
    programId.toBase58(),
  );
  const activeHeader = join(headerDirectory, "active.json");
  const payerLock = join(headerDirectory, "payer.lock");
  async function resumeHeader() {
    if ((await optionalJson(activeHeader)) === undefined) return;
    const lc = await follow();
    await withUploadLock(payerLock, async () => {
      const pending = await optionalJson(activeHeader);
      if (pending !== undefined) await finishHeader(object(pending), lc);
    });
  }
  async function update(target: number, tamper = false) {
    const lc = await follow();
    await withUploadLock(payerLock, async () => {
      const pending = await optionalJson(activeHeader);
      if (pending !== undefined) await finishHeader(object(pending), lc);
      const current = await lc.client.account.clientState.fetch(lc.clientState);
      const trusted = current.latestHeight.revisionHeight.toNumber();
      if (target <= trusted) return;
      await assertClientActive(wallet, lc.client, chainId);
      const packet = await headerPacket(
        settings,
        trusted,
        target,
        current.latestHeight.revisionNumber.toNumber(),
        {
          numerator: BigInt(current.trustLevelNumerator.toString()),
          denominator: BigInt(current.trustLevelDenominator.toString()),
        },
      );
      if (tamper) {
        const offset = packet.bytes.indexOf(Buffer.from(packet.consensus.root));
        assert.ok(offset >= 0);
        packet.bytes[offset] ^= 1;
      }
      const job = {
        chainId,
        genesis,
        client: lc.program.toBase58(),
        target,
        trusted,
        packet: {
          chainId,
          header: packet.bytes.toString("hex"),
          consensus: {
            timestamp: packet.consensus.timestamp.toString(),
            root: Buffer.from(packet.consensus.root).toString("hex"),
            nextValidatorsHash: Buffer.from(
              packet.consensus.nextValidatorsHash,
            ).toString("hex"),
          },
          signatures: packet.signatures.map((sig) => ({
            hash: Buffer.from(sig.signatureHash).toString("hex"),
            pubkey: Buffer.from(sig.pubkey).toString("hex"),
            message: sig.msg.toString("hex"),
            signature: Buffer.from(sig.signature).toString("hex"),
          })),
        },
      };
      await atomicJson(activeHeader, job);
      await finishHeader(job, lc);
    });
  }
  async function finishHeader(job: Record<string, unknown>, lc: Followed) {
    assert.equal(job.chainId, chainId);
    assert.equal(job.genesis, genesis);
    assert.ok(
      typeof job.target === "number" &&
        Number.isSafeInteger(job.target) &&
        job.target > 0,
    );
    assert.ok(
      typeof job.trusted === "number" &&
        Number.isSafeInteger(job.trusted) &&
        job.trusted > 0 &&
        job.trusted < job.target,
    );
    const owner = job.client;
    assert.ok(typeof owner === "string");
    if (owner !== lc.program.toBase58()) {
      // Governance re-pinned during this upload. Its temporary accounts stay
      // under the old program; keep the job for inspection.
      const saved = join(
        headerDirectory,
        `abandoned-${job.target}-${Date.now()}.json`,
      );
      await rename(activeHeader, saved);
      alert({
        event: "header-upload-abandoned",
        client: owner,
        current: lc.program.toBase58(),
        target: job.target,
        saved,
      });
      return;
    }
    const result = await uploadHeader({
      wallet,
      packet: decodePacket(job.packet, chainId),
      target: job.target,
      trusted: job.trusted,
      journalDirectory: headerDirectory,
      lightClient: lc,
      beforeUpload: () => assertClientActive(wallet, lc.client, chainId),
      record: async (value) => {
        await appendFile(
          join(headerDirectory, "transactions.jsonl"),
          JSON.stringify(value) + "\n",
          { mode: 0o600 },
        );
      },
    });
    // A confirmed rejection is terminal for this exact header packet, and
    // the transport has reclaimed it. An unknown RPC outcome retains it.
    await unlink(activeHeader);
    const failure = result.assembly.error;
    if (
      typeof failure === "object" &&
      failure !== null &&
      "unsupportedHeader" in failure
    )
      throw new HeaderUnsupported(String(failure.unsupportedHeader));
    assert.equal(
      failure,
      null,
      `UpdateClientFailed: ${JSON.stringify(failure)}`,
    );
    log({
      event: "authenticated-header",
      trusted: job.trusted,
      height: job.target,
      signatures: result.commitSignatures,
      bytes: result.headerBytes,
      computeUnits: result.assembly.compute,
      signature: result.assembly.signature,
    });
  }
  const ensureClientActive = async () =>
    assertClientActive(wallet, (await follow()).client, chainId);
  // Unix seconds from the Clock sysvar, the time the programs check.
  async function solanaTime() {
    const clock = await wallet.connection.getAccountInfo(
      web3.SYSVAR_CLOCK_PUBKEY,
    );
    assert.ok(clock && clock.data.length === 40, "Solana clock unavailable");
    return clock.data.readBigInt64LE(32);
  }

  // Lamports spent per transfer survive restarts. The file name carries the
  // deployment, so a rebuilt ledger starts a separate one.
  const identity = { genesis, programId: programId.toBase58() };
  const ledgerPath = join(
    settings.stateDir,
    `relay-budget-v2-${genesis}-${programId.toBase58()}.json`,
  );
  const ledgerLock = join(settings.stateDir, "relay-budget.lock");
  const readLedger = async () =>
    SpendLedger.parse(await optionalJson(ledgerPath), identity);
  // Read-modify-write under a lock, so an operator reset is never lost.
  async function updateLedger(change: (ledger: SpendLedger) => boolean) {
    return withUploadLock(
      ledgerLock,
      async () => {
        const ledger = await readLedger();
        if (change(ledger))
          await atomicJson(ledgerPath, ledger.toJSON(identity));
        return ledger;
      },
      10_000,
    );
  }
  async function resetBudget(id: SinkId | "all") {
    const removed = await withUploadLock(payerLock, async () => {
      for (const pending of (await readLedger()).entries.keys())
        if (id === "all" || pending === id) await resumePayment(pending);
      return updateLedger((ledger) => ledger.reset(id));
    });
    log({ event: "relay-budget-reset", id: id.toString(), path: ledgerPath });
    return removed;
  }
  const jobDirectory = join(
    settings.stateDir,
    "relay-jobs-v2",
    genesis,
    programId.toBase58(),
  );
  const jobPath = (id: SinkId) =>
    join(jobDirectory, `${proofNonce(transferKey(id)).toString("hex")}.json`);
  const proofIdentity = {
    ...identity,
    chainId,
    payer: wallet.publicKey.toBase58(),
  };
  const proofJobPath = (id: SinkId) => `${jobPath(id)}.proof`;
  async function readProofJob(id: SinkId) {
    const saved = await optionalJson(proofJobPath(id));
    return saved === undefined
      ? undefined
      : parseProofJob(saved, proofIdentity, id);
  }
  async function proofJobIds() {
    let files: string[];
    try {
      files = await readdir(jobDirectory);
    } catch (error) {
      if (error instanceof Error && "code" in error && error.code === "ENOENT")
        return [];
      throw error;
    }
    const ids: SinkId[] = [];
    for (const file of files.filter((name) => name.endsWith(".json.proof"))) {
      const saved = await optionalJson(join(jobDirectory, file));
      if (saved === undefined) continue;
      const id = parseSinkId(object(saved).id);
      parseProofJob(saved, proofIdentity, id);
      assert.equal(
        file,
        basename(proofJobPath(id)),
        "proof journal filename mismatch",
      );
      ids.push(id);
    }
    return ids;
  }
  async function chargeResult(id: SinkId, result: TransactionResult) {
    const liquidity = ["proof-create", "proof-grow", "proof-close"].includes(
      result.stage,
    );
    if (liquidity || result.stage === "resolve")
      assert.ok(
        Number.isSafeInteger(result.payerLamports),
        "missing payer balance change",
      );
    const payerLamports = result.payerLamports ?? result.fee;
    const spent = result.stage === "resolve" ? payerLamports : result.fee;
    const bufferChange = liquidity ? payerLamports - result.fee : 0;
    await updateLedger((ledger) => {
      ledger.charge(id, spent, result.signature, bufferChange);
      return true;
    });
    log({
      event: "relay-payment",
      id,
      stage: result.stage,
      signature: result.signature,
      fee: result.fee,
      payerLamports,
      bufferChangeLamports: bufferChange,
      spentLamports: spent,
    });
  }
  async function resumePayment(id: SinkId) {
    await resumeTransaction(wallet, jobPath(id), (result) =>
      chargeResult(id, result),
    );
  }
  // Each transaction is simulated before it is sent, so a failing one costs
  // nothing. Fees always count; rent counts for the resolving transaction.
  // Closing a proof buffer only returns rent and is allowed past the limit.
  async function paid(
    id: SinkId,
    stage: string,
    instructions: web3.TransactionInstruction[],
    reclaims = false,
  ) {
    await resumePayment(id);
    const max = settings.maxLamportsPerTransfer;
    if (!reclaims && (await readLedger()).exhausted(id, max))
      throw new BudgetExhausted(`transfer ${id} is over its spend limit`);
    return timed(
      stage,
      async () => {
        const result = await sendTransaction({
          wallet,
          stage,
          instructions,
          preflight: true,
          pendingPath: jobPath(id),
          observe: (result) => chargeResult(id, result),
        });
        assert.equal(
          result.error,
          null,
          `${stage} failed: ${JSON.stringify(result.error)} (${result.signature})`,
        );
        return result.signature;
      },
      id,
    );
  }
  async function closeProofBuffer(id: SinkId) {
    const job = await readProofJob(id);
    if (!job) return;
    const proofBuffer = proofBufferAt(Buffer.from(job.key, "hex"));
    const account =
      await program.account.proofBuffer.fetchNullable(proofBuffer);
    if (account) {
      assert.ok(
        account.submitter.equals(wallet.publicKey),
        "proof buffer submitter mismatch",
      );
      assert.deepEqual(
        Buffer.from(account.nonce),
        Buffer.from(job.nonce, "hex"),
        "proof buffer nonce mismatch",
      );
      await paid(
        id,
        "proof-close",
        [
          await program.methods
            .closeProofBuffer()
            .accountsStrict({ submitter: wallet.publicKey, proofBuffer })
            .instruction(),
        ],
        true,
      );
    }
    await unlink(proofJobPath(id));
  }

  async function finishProofBuffers(id: SinkId) {
    await closeProofBuffer(id);
    await updateLedger((ledger) => {
      ledger.complete(id);
      return true;
    });
  }

  async function uploadProof(id: SinkId, wanted: ProofJob) {
    let job = await readProofJob(id);
    if (job && !sameProof(job, wanted)) {
      await closeProofBuffer(id);
      job = undefined;
    }
    if (!job) {
      job = wanted;
      await atomicJson(proofJobPath(id), job);
    }
    const proof = Buffer.from(job.proof, "hex");
    const proofBuffer = proofBufferAt(Buffer.from(job.key, "hex"));
    async function progress() {
      const info = await wallet.connection.getAccountInfo(
        proofBuffer,
        "finalized",
      );
      if (!info) return undefined;
      assert.ok(info.owner.equals(programId), "proof buffer owner mismatch");
      const account = program.coder.accounts.decode<
        Awaited<ReturnType<typeof program.account.proofBuffer.fetch>>
      >("proofBuffer", info.data);
      assert.ok(
        account.submitter.equals(wallet.publicKey),
        "proof buffer submitter mismatch",
      );
      assert.deepEqual(
        Buffer.from(account.nonce),
        Buffer.from(wanted.nonce, "hex"),
        "proof buffer nonce mismatch",
      );
      assert.equal(
        account.expectedLen,
        proof.length,
        "proof buffer length mismatch",
      );
      assert.deepEqual(
        Buffer.from(account.data),
        proof.subarray(0, account.data.length),
        "proof upload prefix mismatch",
      );
      assert.ok(
        account.data.length <= proof.length,
        "proof buffer exceeds expected length",
      );
      const updated = {
        ...wanted,
        uploaded: account.data.length,
        allocated: info.data.length,
        lamports: info.lamports,
      };
      await atomicJson(proofJobPath(id), updated);
      return updated;
    }
    let state = await progress();
    if (!state) {
      await paid(id, "proof-create", [
        await program.methods
          .createProofBuffer(
            Array.from(proofNonce(Buffer.from(job.key, "hex"))),
            proof.length,
          )
          .accountsStrict({
            submitter: wallet.publicKey,
            proofBuffer,
            systemProgram: web3.SystemProgram.programId,
          })
          .instruction(),
      ]);
      state = await progress();
      assert.ok(state, "created proof buffer missing");
    }
    while (state.allocated < PROOF_HEADER_SIZE + proof.length) {
      const previous = state.allocated;
      await paid(id, "proof-grow", [
        await program.methods
          .growProofBuffer()
          .accountsStrict({
            submitter: wallet.publicKey,
            proofBuffer,
            systemProgram: web3.SystemProgram.programId,
          })
          .instruction(),
      ]);
      state = await progress();
      assert.ok(
        state && state.allocated > previous,
        "proof buffer did not grow",
      );
    }
    while (state.uploaded < proof.length) {
      const offset = state.uploaded;
      // Two 450-byte appends fit in one transaction below the 1232-byte limit.
      const chunkSize = 450;
      const end = Math.min(offset + 2 * chunkSize, proof.length);
      const instructions: web3.TransactionInstruction[] = [];
      for (let start = offset; start < end; start += chunkSize) {
        instructions.push(
          await program.methods
            .appendProof(
              start,
              proof.subarray(start, Math.min(start + chunkSize, end)),
            )
            .accountsStrict({ submitter: wallet.publicKey, proofBuffer })
            .instruction(),
        );
      }
      await paid(id, "proof-append", instructions);
      state = await progress();
      assert.ok(
        state && state.uploaded > offset,
        "proof upload did not advance",
      );
    }
    log({
      event: "proof-buffer-ready",
      id,
      key: job.key,
      nonce: job.nonce,
      proofHeight: job.proofHeight,
      bytes: proof.length,
      allocatedBytes: state.allocated,
      refundableLamports: state.lamports,
    });
    return proofBuffer;
  }
  async function recoverPayments() {
    await withUploadLock(payerLock, async () => {
      const ledger = await readLedger();
      const saved = new Set(await proofJobIds());
      for (const id of new Set([...ledger.entries.keys(), ...saved])) {
        if (ledger.entries.get(id)?.completed && !saved.has(id)) continue;
        await resumePayment(id);
        const status = await program.account.transferStatus.fetchNullable(
          statusAt(id),
        );
        if (status) {
          assert.ok(matchesAnchorId(status.id, id), "status identity mismatch");
          await finishProofBuffers(id);
          continue;
        }
        const job = await readProofJob(id);
        if (job) {
          const state = await akash.state();
          const transfer = await akash.transfer(id, state.height);
          const expected = transfer.settlement?.voidAuthorization
            ? authorizedSettlement(transfer, BigInt(state.height))
            : { key: transferKey(id), value: encodeRecord(transfer.data) };
          const lc = await follow();
          const consensus = await wallet.connection.getAccountInfo(
            lc.consensusAt(BigInt(job.proofHeight)),
            "finalized",
          );
          if (
            !consensus ||
            !consensus.owner.equals(lc.program) ||
            job.key !== Buffer.from(expected.key).toString("hex") ||
            job.valueHash !== proofNonce(expected.value).toString("hex") ||
            job.lightClient !== lc.program.toBase58() ||
            unbackedMint(transfer)
          )
            await closeProofBuffer(id);
        }
      }
    });
  }
  async function prepareProof(id: SinkId, height: number) {
    const transfer = await timed(
      "transfer-query",
      () => akash.transfer(id, height - 1),
      id,
    );
    const record = decodeRecord(transfer);
    const authorization = transfer.settlement?.voidAuthorization;
    const key = Buffer.from(
      authorization
        ? authorizedSettlement(transfer, BigInt(height - 1)).key
        : transferKey(id),
    );
    const reply = await timed(
      "proof-query",
      () =>
        akash.comet.abciQuery({
          path: "/store/sink/key",
          data: key,
          height: height - 1,
          prove: true,
        }),
      id,
    );
    assert.ok(
      reply.code === 0 &&
        reply.height === height - 1 &&
        reply.proof?.ops.length === 2,
      "missing committed sink proof",
    );
    assert.deepEqual(Buffer.from(reply.key), key);
    assert.ok(record.height < BigInt(height), "proof predates identity");
    const value = authorization
      ? settlementProofValue(transfer, BigInt(height - 1), reply.value)
      : encodeRecord(transfer.data);
    assert.deepEqual(
      Buffer.from(reply.value),
      value,
      "record proof differs from snapshot",
    );
    const proof = encodeMembershipProof({ key, value, ops: reply.proof.ops });
    return { transfer, record, authorization, key, value, proof };
  }
  type PreparedProof = Awaited<ReturnType<typeof prepareProof>>;

  async function relayWith(
    lc: Followed,
    id: SinkId,
    height: number,
    prepared?: PromiseSettledResult<PreparedProof>,
  ): Promise<"resolved" | "already-resolved" | "skipped"> {
    await resumePayment(id);
    const status = await wallet.connection.getAccountInfo(statusAt(id));
    if (status && status.owner.equals(programId) && status.data.length > 8) {
      assert.ok(
        matchesAnchorId(
          (await program.account.transferStatus.fetch(statusAt(id))).id,
          id,
        ),
        "status identity mismatch",
      );
      await finishProofBuffers(id);
      return "already-resolved";
    }
    if ((await readLedger()).exhausted(id, settings.maxLamportsPerTransfer))
      return "skipped";
    if (prepared?.status === "rejected") throw prepared.reason;
    const { transfer, record, authorization, key, value, proof } =
      prepared?.status === "fulfilled"
        ? prepared.value
        : await prepareProof(id, height);
    // Last Akash read before paying: a void order or refund since planning
    // means minting would leave the refund as unbacked supply.
    const latest = await timed("state-query", () => akash.state(), id);
    const now = await timed(
      "transfer-query",
      () => akash.transfer(id, latest.height),
      id,
    );
    assert.ok(now.settlement, "missing sink settlement");
    const changed = finalCheck(transfer, now);
    if (changed) {
      alert({
        event: "relay-final-check",
        id: id.toString(),
        reason: changed,
        akashStatus: Status[now.status],
        akashHeight: latest.height,
        action: "skipped",
      });
      return "skipped";
    }
    await updateLedger((ledger) => {
      ledger.attempt(id);
      return true;
    });
    const proofBuffer = await uploadProof(
      id,
      newProofJob({
        ...proofIdentity,
        id,
        lightClient: lc.program.toBase58(),
        proofHeight: height,
        key,
        value,
        proof,
      }),
    );
    // Upload can span many blocks. Repeat the state and outcome guard immediately before resolution.
    const finalState = await timed("state-query", () => akash.state(), id);
    const finalTransfer = await timed(
      "transfer-query",
      () => akash.transfer(id, finalState.height),
      id,
    );
    if (finalCheck(transfer, finalTransfer)) {
      await closeProofBuffer(id);
      return "skipped";
    }
    const resolved = await program.account.transferStatus.fetchNullable(
      statusAt(id),
    );
    if (resolved) {
      assert.ok(matchesAnchorId(resolved.id, id), "status identity mismatch");
      await finishProofBuffers(id);
      return "already-resolved";
    }
    const latestClient = await lc.client.account.clientState.fetch(
      lc.clientState,
    );
    const shared = {
      payer: wallet.publicKey,
      config,
      bridge,
      proofBuffer,
      status: statusAt(id),
      lightClientProgram: lc.program,
      clientState: lc.clientState,
      latestConsensus: lc.consensusAt(
        BigInt(latestClient.latestHeight.revisionHeight.toString()),
      ),
      proofConsensus: lc.consensusAt(BigInt(height)),
      systemProgram: web3.SystemProgram.programId,
    };
    const resolution = authorization
      ? await program.methods
          .voidTransfer(anchorId(id), transfer.status, new BN(height))
          .accountsStrict(shared)
          .instruction()
      : await program.methods
          .mintTransfer(
            anchorId(id),
            record.recipient,
            new BN(record.amount.toString()),
            new BN(height),
          )
          .accountsStrict({
            ...shared,
            mint,
            mintAuthority,
            recipient: record.recipient,
            recipientTokenAccount: getAssociatedTokenAddressSync(
              mint,
              record.recipient,
              true,
            ),
            tokenProgram: TOKEN_PROGRAM_ID,
            associatedTokenProgram: ASSOCIATED_TOKEN_PROGRAM_ID,
          })
          .instruction();
    const signature = await paid(id, "resolve", [...compute(), resolution]);
    await closeProofBuffer(id);
    await updateLedger((ledger) => {
      ledger.complete(id);
      return true;
    });
    const ledger = await readLedger();
    log({
      event: "transfer-resolved",
      id: id.toString(),
      proofHeight: height,
      proofBytes: proof.length,
      signature,
      spentLamports: ledger.spent(id),
      attempts: ledger.entries.get(id)?.attempts ?? 1,
    });
    return "resolved";
  }
  const relay = async (id: SinkId, height: number) =>
    withUploadLock(payerLock, async () =>
      relayWith(await follow(), id, height),
    );

  async function withoutStatus(ids: SinkId[]) {
    const pending: SinkId[] = [];
    for (let i = 0; i < ids.length; i += STATUS_BATCH) {
      const batch = ids.slice(i, i + STATUS_BATCH);
      const infos = await wallet.connection.getMultipleAccountsInfo(
        batch.map(statusAt),
      );
      for (const [j, id] of batch.entries()) {
        const info = infos[j];
        if (!info || !info.owner.equals(programId) || info.data.length <= 8) {
          pending.push(id);
          continue;
        }
        const status = program.coder.accounts.decode<
          Awaited<ReturnType<typeof program.account.transferStatus.fetch>>
        >("transferStatus", info.data);
        assert.ok(matchesAnchorId(status.id, id), "status identity mismatch");
      }
    }
    return pending;
  }
  const discoveryPath = join(
    settings.stateDir,
    `relay-discovery-v3-${genesis}-${programId}.json`,
  );
  async function unresolved(state: Awaited<ReturnType<typeof akash.state>>) {
    const saved = await optionalJson(discoveryPath);
    let cursor: DiscoveryCursor | undefined;
    if (saved !== undefined) {
      const value = object(saved);
      assert.ok(
        value.version === 3 &&
          value.chainId === chainId &&
          value.genesis === genesis &&
          value.programId === programId.toBase58(),
        "unsupported discovery state; preserve it and use a fresh discovery file",
      );
      const checkpoint = z
        .object({
          key: z.string().base64(),
          lastId: z.string().transform(parseSinkId),
        })
        .strict()
        .nullable()
        .parse(value.cursor);
      if (checkpoint)
        cursor = {
          kind: "tail",
          key: Buffer.from(checkpoint.key, "base64"),
          lastId: checkpoint.lastId,
        };
    }
    const entries = new Map(
      (await openTransfers(akash, state.height)).map((entry) => [
        entry.id,
        entry.transfer,
      ]),
    );
    const unfinished = new Set(await proofJobIds());
    for (const [id, entry] of (await readLedger()).entries)
      if (!entry.completed) unfinished.add(id);
    for (const id of unfinished)
      if (!entries.has(id))
        entries.set(id, await akash.transfer(id, state.height));
    let discovered = true;
    try {
      for (;;) {
        const page = await discoverTransfers(akash, state.height, cursor);
        for (const entry of page.entries) entries.set(entry.id, entry.transfer);
        cursor = page.cursor;
        if (!page.hasMore) break;
      }
    } catch (error) {
      // Open transfers still relay; the saved checkpoint retries this history.
      discovered = false;
      alert({ event: "relay-discovery-failed", error: message(error) });
    }
    const pending = await withoutStatus([...entries.keys()]);
    if (discovered)
      await atomicJson(discoveryPath, {
        version: 3,
        chainId,
        ...identity,
        cursor: cursor
          ? {
              key: Buffer.from(cursor.key).toString("base64"),
              lastId: cursor.lastId,
            }
          : null,
      });
    return { pending, known: entries };
  }
  const waiting = new Map<SinkId, string>();
  const unbacked = new AlertSchedule(15 * 60_000);
  const exhausted = new AlertSchedule(15 * 60_000);
  const headerBlocked = new AlertSchedule<bigint>(15 * 60_000);
  // The lowest client height whose proof covers the record's current action.
  // A value above the target means no available header covers it yet.
  async function plan(
    id: SinkId,
    height: number,
    latest: number,
    known?: TransferState,
  ) {
    const transfer = known ?? (await akash.transfer(id, height));
    const record = decodeRecord(transfer);
    assert.ok(
      record.height < BigInt(Number.MAX_SAFE_INTEGER),
      "Akash RPC cannot represent this height exactly",
    );
    const created = Number(record.height);
    assert.ok(transfer.settlement, "missing sink settlement");
    const saved = await readProofJob(id);
    const planned = {
      id,
      record,
      authorization: transfer.settlement.voidAuthorization,
      akash: transfer.status,
      hold: transfer.settlement.hold,
      retained: saved !== undefined,
      unbacked: unbackedMint(transfer),
    };
    // Header H proves state committed at H - 1.
    let resumeHeight = saved?.proofHeight;
    let minimum = created + 1;
    if (saved) {
      const key = Buffer.from(saved.key, "hex");
      const checkpoint = await akash.comet.abciQuery({
        path: "/store/sink/key",
        data: key,
        height: saved.proofHeight - 1,
        prove: false,
      });
      assert.equal(checkpoint.code, 0, "proof checkpoint query failed");
      assert.equal(checkpoint.height, saved.proofHeight - 1);
      assert.deepEqual(Buffer.from(checkpoint.key), key);
      // IAVL's non-proof query reports a missing version separately from a missing key.
      if (checkpoint.log === "version does not exist") {
        assert.equal(checkpoint.codespace, "");
        assert.equal(checkpoint.value.length, 0);
        assert.equal(checkpoint.proof, undefined);
        resumeHeight = undefined;
        minimum = saved.proofHeight + 1;
      } else {
        assert.equal(checkpoint.log, "", "unrecognized checkpoint response");
        assert.equal(
          proofNonce(checkpoint.value).toString("hex"),
          saved.valueHash,
        );
      }
    }
    if (!transfer.settlement.voidAuthorization)
      return {
        ...planned,
        height: resumeHeight ?? minimum,
        resumeHeight,
      };
    // Prefer a retained upload while its exact settlement is still current.
    for (const candidate of new Set([resumeHeight, latest, height - 1])) {
      if (
        candidate === undefined ||
        candidate > height - 1 ||
        candidate < minimum
      )
        continue;
      const snapshot = await akash.transfer(id, candidate - 1);
      if (
        snapshot.settlement?.voidAuthorization &&
        !finalCheck(snapshot, transfer)
      )
        return {
          ...planned,
          height: candidate,
          resumeHeight: resumeHeight === candidate ? candidate : undefined,
        };
    }
    return {
      ...planned,
      height: Math.max(height, minimum),
      resumeHeight: undefined,
    };
  }
  // Check pause, caps and recipients before paying for a proof upload.
  async function payable(plans: Awaited<ReturnType<typeof plan>>[]) {
    const [configInfo, bridgeInfo, clock] =
      await wallet.connection.getMultipleAccountsInfo([
        config,
        bridge,
        web3.SYSVAR_CLOCK_PUBKEY,
      ]);
    assert.ok(configInfo && bridgeInfo, "bridge configuration unavailable");
    assert.ok(clock && clock.data.length === 40, "Solana clock unavailable");
    const launch = program.coder.accounts.decode<
      Awaited<ReturnType<typeof program.account.launchConfig.fetch>>
    >("launchConfig", configInfo.data);
    const configured = program.coder.accounts.decode<
      Awaited<ReturnType<typeof program.account.bridgeConfig.fetch>>
    >("bridgeConfig", bridgeInfo.data);
    const policy = {
      enabled: launch.bridgeEnabled,
      perTransferMax: BigInt(configured.policy.perTransferMax.toString()),
      dailyMintCap: BigInt(configured.policy.dailyMintCap.toString()),
      mintDay: BigInt(configured.mintDay.toString()),
      mintedToday: BigInt(configured.mintedToday.toString()),
    };
    const minting = plans.filter((entry) => !entry.authorization);
    const recipients = new Map<SinkId, boolean>();
    // Each record reads its recipient and ATA, two addresses per entry.
    for (let i = 0; i < minting.length; i += STATUS_BATCH / 2) {
      const batch = minting.slice(i, i + STATUS_BATCH / 2);
      const infos = await wallet.connection.getMultipleAccountsInfo(
        batch.flatMap((entry) => [
          entry.record.recipient,
          getAssociatedTokenAddressSync(mint, entry.record.recipient, true),
        ]),
      );
      for (const [j, entry] of batch.entries())
        recipients.set(
          entry.id,
          recipientMints({
            recipient: entry.record.recipient,
            account: infos[2 * j],
            ata: infos[2 * j + 1],
            mint,
            mintAuthority,
            vaults: configured.policy.vaults,
          }),
        );
    }
    const now = clock.data.readBigInt64LE(32);
    let reserved = 0n;
    const ready = [];
    for (const entry of plans) {
      const mints = recipients.get(entry.id) ?? false;
      const reason = entry.authorization
        ? undefined
        : mintWait(policy, now, entry.record.amount, mints, reserved);
      if (reason) {
        if (waiting.get(entry.id) !== reason)
          log({ event: "relay-wait", id: entry.id.toString(), reason });
        waiting.set(entry.id, reason);
        continue;
      }
      waiting.delete(entry.id);
      if (mints && !entry.authorization) reserved += entry.record.amount;
      ready.push(entry);
    }
    return ready;
  }
  async function relayFailed(id: SinkId, error: unknown) {
    let failures = 0,
      delayMs = 0;
    await updateLedger((ledger) => {
      ({ failures, delayMs } = ledger.failed(
        id,
        Date.now(),
        settings.retryMaxMs,
      ));
      return true;
    });
    alert({
      event: "relay-retry",
      id: id.toString(),
      attempt: failures,
      retrySeconds: delayMs / 1000,
      error: message(error),
    });
  }
  // Paying stopped at the limit. Alert when first seen and every 15 minutes;
  // close a proof buffer a failed attempt left, which only returns rent.
  async function overBudget(id: SinkId, ledger: SpendLedger) {
    if (!exhausted.due(id, Date.now())) return;
    alert({
      event: "relay-budget-exhausted",
      id: id.toString(),
      spentLamports: ledger.spent(id),
      maxLamportsPerTransfer: settings.maxLamportsPerTransfer,
      attempts: ledger.entries.get(id)?.attempts ?? 0,
      ledger: ledgerPath,
      action: "skipped",
    });
    try {
      await withUploadLock(payerLock, async () => {
        await closeProofBuffer(id);
      });
    } catch (error) {
      alert({
        event: "proof-buffer-close-failed",
        id: id.toString(),
        error: message(error),
      });
    }
  }
  // Only a completed update clears the header backoff. A rejected upload
  // leaves no job to resume, so resuming nothing must not reset the delay.
  async function headerStep(action: () => Promise<void>, target?: number) {
    const now = Date.now();
    if (!headerRetries.ready(0n, now) || !unsupported.ready(0n, now))
      return false;
    try {
      await action();
      unsupported.clear(0n);
      return true;
    } catch (error) {
      if (error instanceof HeaderUnsupported) {
        const { delayMs } = unsupported.failed(0n, Date.now());
        alert({
          event: "header-unsupported",
          target,
          reason: error.message,
          retrySeconds: delayMs / 1000,
        });
        return false;
      }
      const { failures, delayMs } = headerRetries.failed(0n, Date.now());
      alert({
        event: "header-retry",
        target,
        attempt: failures,
        retrySeconds: delayMs / 1000,
        error: message(error),
      });
      return false;
    }
  }
  // Each update leaves a consensus state holding rent. The verifier closes
  // only expired, non-latest states, so a scan every interval suffices.
  async function pruneConsensus(lc: Followed) {
    if (Date.now() - prunedAt < settings.pruneIntervalMs) return;
    prunedAt = Date.now();
    try {
      const result = await withUploadLock(payerLock, () =>
        pruneExpiredConsensusStates(wallet, { lightClient: lc }),
      );
      if (result.pruned || result.failed)
        log({ event: "consensus-pruned", ...result });
    } catch (error) {
      alert({
        event: "consensus-prune-failed",
        error: error instanceof Error ? error.message : "prune failed",
      });
    }
  }
  async function tick() {
    const lc = await follow();
    await recoverPayments();
    const allowed = await relayerAccess(lc);
    // Without update rights a resumed upload would only pay for failures.
    if (allowed !== "none") await headerStep(resumeHeader);
    try {
      await assertClientActive(wallet, lc.client, chainId);
    } catch (error) {
      if (allowed === "member") await pruneConsensus(lc);
      await reclaimLookups(wallet, headerDirectory);
      throw error;
    }
    const state = await timed("state-query", () => akash.state());
    const { pending, known } = await timed("discovery", () =>
      unresolved(state),
    );
    const ledger = await readLedger();
    const trusted = await lc.client.account.clientState.fetch(lc.clientState);
    let current = trusted.latestHeight.revisionHeight.toNumber();
    const target = state.height - 1;
    const plans = [];
    for (const id of pending) {
      if (!ledger.ready(id, Date.now())) continue;
      if (ledger.exhausted(id, settings.maxLamportsPerTransfer)) {
        await overBudget(id, ledger);
        continue;
      }
      exhausted.clear(id);
      try {
        const entry = await plan(id, state.height, current, known.get(id));
        if (entry.unbacked) {
          // Never mint over an Akash void or refund. Pay nothing; alert.
          if (unbacked.due(id, Date.now()))
            alert({
              event: "relay-unbacked-settlement",
              id: id.toString(),
              akashStatus: Status[entry.akash],
              reason: entry.unbacked,
              action: "skipped",
            });
          continue;
        }
        unbacked.clear(id);
        // A record newer than every available header waits; no update helps.
        if (entry.height <= target) plans.push(entry);
      } catch (error) {
        await relayFailed(id, error);
      }
    }
    const ready = await payable(plans);
    const consensus = await lc.client.account.consensusStateStore.fetch(
      lc.consensusAt(BigInt(current)),
    );
    const refresh = headerRefresh({
      latest: current,
      target,
      required: ready.map((entry) => entry.height),
      ageSeconds: Number(
        (await solanaTime()) -
          BigInt(consensus.consensusState.timestamp.toString()) /
            1_000_000_000n,
      ),
      trustingPeriodSeconds: trusted.trustingPeriod.toNumber(),
      refreshFraction: settings.refreshFraction,
    });
    if (refresh && allowed === "none") {
      if (headerBlocked.due(0n, Date.now()))
        alert({
          event: "relay-header-blocked",
          reason: refresh,
          payer: wallet.publicKey.toBase58(),
          lightClient: lc.program.toBase58(),
          trusted: current,
          target,
        });
    } else if (refresh) {
      headerBlocked.clear(0n);
      // One update to the newest canonical header serves every pending relay.
      const updated = await headerStep(async () => {
        log({
          event: "header-refresh",
          reason: refresh,
          trusted: current,
          target,
        });
        await timed("header-update", () => update(target));
      }, target);
      if (updated) headerRetries.clear(0n);
      current = (
        await lc.client.account.clientState.fetch(lc.clientState)
      ).latestHeight.revisionHeight.toNumber();
    }
    const covered = relayOrder(
      ready.filter((entry) => entry.height <= current),
      wallet.publicKey,
    );
    // Bound readonly proof preparation; paid work keeps its order and payer lock.
    for (let offset = 0; offset < covered.length; offset += 2) {
      const batch = covered.slice(offset, offset + 2);
      const prepared = await Promise.allSettled(
        batch.map((entry) =>
          prepareProof(entry.id, entry.resumeHeight ?? current),
        ),
      );
      for (const [index, entry] of batch.entries()) {
        try {
          await withUploadLock(payerLock, () =>
            relayWith(
              lc,
              entry.id,
              entry.resumeHeight ?? current,
              prepared[index],
            ),
          );
          await updateLedger((ledger) => {
            ledger.clearRetry(entry.id);
            return true;
          });
        } catch (error) {
          // The next pass alerts once and stops paying for it.
          if (error instanceof BudgetExhausted) continue;
          await relayFailed(entry.id, error);
        }
      }
    }
    // After relays have used the latest height, which is never pruned.
    if (allowed === "member") await pruneConsensus(lc);
    await reclaimLookups(wallet, headerDirectory);
  }
  return {
    bootstrap,
    relay,
    update,
    ensureClientActive,
    resumeHeader,
    tick: () => timed("tick", tick),
    resetBudget,
    lightClient: follow,
    settings,
    akash,
    wallet,
    // The last client followed; the local ID before the first pass.
    get client() {
      return followed?.client ?? clientProgram(wallet, CLIENT_ID);
    },
    program,
    close: () => akash.close(),
  };
}

// A relayer for an existing deployment, configured from a JSON file.
export async function configuredRelay(path: string) {
  const file = resolve(path);
  const config = parseRelayConfig(
    JSON.parse(await readFile(file, "utf8")),
    dirname(file),
  );
  const payer = await loadPayer(config.payerKeyFile);
  const connection = new web3.Connection(config.solanaRpc, {
    commitment: "confirmed",
    disableRetryOnRateLimit: true,
    fetch: (url, init) =>
      fetch(url, { ...init, signal: AbortSignal.timeout(30_000) }),
  });
  return relayContext({
    config,
    wallet: new AnchorProvider(connection, new Wallet(payer), {
      commitment: "confirmed",
      preflightCommitment: "confirmed",
    }),
  });
}

export async function runRelayer(args = process.argv.slice(2)) {
  const { values } = parseArgs({
    args,
    options: {
      bootstrap: { type: "boolean" },
      watch: { type: "boolean" },
      "check-invalid-header": { type: "boolean" },
      state: { type: "string" },
      config: { type: "string" },
      "reset-budget": { type: "string" },
    },
  });
  if (values.config)
    assert.ok(
      !values.bootstrap && !values["check-invalid-header"],
      "--bootstrap and --check-invalid-header are local stack tools",
    );
  const context = values.config
    ? await configuredRelay(values.config)
    : await relayContext();
  let stop = false;
  process.once("SIGINT", () => {
    stop = true;
  });
  process.once("SIGTERM", () => {
    stop = true;
  });
  try {
    const reset = values["reset-budget"];
    if (reset !== undefined) {
      await context.resetBudget(reset === "all" ? "all" : parseSinkId(reset));
      return;
    }
    if (values.bootstrap) {
      await context.bootstrap();
      return;
    }
    if (values["check-invalid-header"]) {
      const lc = await context.lightClient();
      const before = await lc.client.account.clientState.fetch(lc.clientState);
      const trusted = before.latestHeight.revisionHeight.toNumber();
      let target = (await context.akash.state()).height - 1;
      const deadline = Date.now() + 30_000;
      while (target <= trusted) {
        assert.ok(
          Date.now() < deadline,
          "Akash did not produce a newer canonical header",
        );
        await sleep(500);
        target = (await context.akash.state()).height - 1;
      }
      await assert.rejects(
        () => context.update(target, true),
        (error: unknown) =>
          error instanceof Error && /UpdateClientFailed/.test(error.message),
      );
      const after = await lc.client.account.clientState.fetch(lc.clientState);
      assert.equal(
        after.latestHeight.revisionHeight.toNumber(),
        trusted,
        "rejected header changed trusted height",
      );
      assert.equal(
        await lc.client.account.consensusStateStore.fetchNullable(
          lc.consensusAt(BigInt(target)),
        ),
        null,
        "rejected header left a trusted consensus state",
      );
      await context.update(target);
      console.log(
        "Altered app root rejected; authentic signed header accepted.",
      );
      return;
    }
    console.log(
      JSON.stringify({
        event: "relay-start",
        payer: context.wallet.publicKey.toBase58(),
        programId: context.settings.programId.toBase58(),
        akashChainId: context.settings.akashChainId,
        lightClientPin: context.settings.lightClientProgram?.toBase58(),
        maxLamportsPerTransfer: context.settings.maxLamportsPerTransfer,
        stateDir: context.settings.stateDir,
      }),
    );
    do {
      try {
        await context.tick();
      } catch (error) {
        console.error(error);
        if (!values.watch) throw error;
      }
      if (values.watch && !stop) await sleep(context.settings.pollMs);
    } while (values.watch && !stop);
  } finally {
    context.close();
  }
}
// Also imported by the local scenario runner.
if (require.main === module)
  void runRelayer().catch((error: unknown) => {
    console.error(error);
    process.exitCode = 1;
  });
