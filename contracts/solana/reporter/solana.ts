import {
  Observation,
  Outcome,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import {
  Program,
  utils,
  web3,
  type IdlAccounts,
  type IdlEvents,
} from "@coral-xyz/anchor";
import { TOKEN_PROGRAM_ID, unpackMint } from "@solana/spl-token";
import assert from "node:assert/strict";
import { isDeepStrictEqual } from "node:util";
import { validateMint } from "./config";
import idl from "../../../.cache/solana/target/idl/akt.json";
import type { Akt } from "../../../.cache/solana/target/types/akt";
import { matchesAnchorId } from "../shared/proof";

import { decodeRecord, statusAddress } from "../shared/proof";
export { decodeRecord } from "../shared/proof";
export type Record = ReturnType<typeof decodeRecord>;
type TransferStatus = IdlAccounts<Akt>["transferStatus"];
type TransferResolved = IdlEvents<Akt>["transferResolved"];
type Executed = Pick<web3.VersionedTransactionResponse, "transaction" | "meta">;
// slotLimit is the highest slot another node could have finalized while this
// fresh node's finalized slot is contextSlot. Higher reported slots are false.
export type Observed =
  | { kind: "absent"; contextSlot: bigint; slotLimit?: bigint }
  | { kind: "resolved"; contextSlot: bigint; observation: Observation };

export type FinalizedObserved =
  | { kind: "absent"; contextSlot: bigint }
  | {
      kind: "resolved";
      contextSlot: bigint;
      address: web3.PublicKey;
      status: TransferStatus;
      outcome: Outcome;
      slot: bigint;
      proofHeight: bigint;
    };

export class SolanaUnavailableError extends Error {}

// The bridge account names a light client other than the configured pin.
export class LightClientMismatchError extends Error {
  constructor(
    readonly pinned: web3.PublicKey,
    readonly current: web3.PublicKey,
  ) {
    super(
      `bridge light client ${current.toBase58()} differs from pinned ${pinned.toBase58()}`,
    );
  }
}

export interface LightClientChange {
  previous: web3.PublicKey;
  current: web3.PublicKey;
  contextSlot: bigint;
}

async function rpcRead<T>(read: () => Promise<T>): Promise<T> {
  try {
    return await read();
  } catch {
    throw new SolanaUnavailableError("Solana node is temporarily unavailable");
  }
}

export interface SolanaConfig {
  genesisHash: string;
  programId: web3.PublicKey;
  mint: web3.PublicKey;
  // A pin stops observation when governance re-pins. Unset follows the bridge.
  lightClientProgram?: web3.PublicKey;
  akashChainId: string;
  maxAgeSeconds: number;
  maxSlotLead?: number;
}

// Solana targets 2.5 slots per second. Allow twice that rate across the
// freshness bound plus a minute of clock and finalization slack.
export function defaultSlotLead(maxAgeSeconds: number) {
  return (maxAgeSeconds + 60) * 5;
}

const KEY = "[1-9A-HJ-NP-Za-km-z]{32,44}";
const INVOKE = new RegExp(`^Program (${KEY}) invoke \\[([1-9][0-9]*)\\]$`);
const EXIT = new RegExp(`^Program (${KEY}) (success|failed: .*)$`);
const DATA = "Program data: ";

// Anchor's EventParser ignores events emitted inside a CPI. Attribute each
// `Program data:` line to the executing frame from the invoke/exit lines
// instead. Unbalanced logs cannot be attributed and throw. A later instruction
// can exhaust the log limit; then only top-level AKT frames that already closed
// with success count.
export function programEvents<E>(
  logs: readonly string[],
  programId: web3.PublicKey,
  decode: (data: string) => E | null,
): E[] {
  const id = programId.toBase58();
  const stack: string[] = [];
  const closed: { root: string; events: E[] }[] = [];
  let events: E[] = [];
  for (const [index, line] of logs.entries()) {
    if (line === "Log truncated") {
      assert.equal(index, logs.length - 1, "inconsistent truncated logs");
      return closed
        .filter((frame) => frame.root === id)
        .flatMap((frame) => frame.events);
    }
    const invoke = INVOKE.exec(line);
    const exit = EXIT.exec(line);
    if (invoke) {
      assert.equal(
        Number(invoke[2]),
        stack.length + 1,
        "inconsistent program log stack",
      );
      stack.push(invoke[1]);
    } else if (exit) {
      // A successful transaction cannot contain a failed invocation.
      assert.ok(
        exit[2] === "success" && stack.pop() === exit[1],
        "inconsistent program log stack",
      );
      if (stack.length === 0) {
        closed.push({ root: exit[1], events });
        events = [];
      }
    } else if (line.startsWith(DATA) && stack.at(-1) === id) {
      const event = decode(line.slice(DATA.length));
      if (event) events.push(event);
    }
  }
  assert.equal(stack.length, 0, "inconsistent program log stack");
  return closed.flatMap((frame) => frame.events);
}

export function safeSlot(slot: bigint): number {
  assert.ok(
    slot >= 0n && slot <= BigInt(Number.MAX_SAFE_INTEGER),
    "unsafe Solana slot",
  );
  return Number(slot);
}

// The owned RPC node is a trust boundary. Finalized commitment cannot make an
// untrusted RPC truthful. No public-provider fallback is configured here.
export class SolanaObserver {
  private readonly program: Program<Akt>;
  private readonly slotLead: bigint;
  private followed?: { program: web3.PublicKey; contextSlot: bigint };
  private readonly changes: LightClientChange[] = [];
  constructor(
    private readonly rpc: web3.Connection,
    private readonly config: SolanaConfig,
  ) {
    const lead = config.maxSlotLead ?? defaultSlotLead(config.maxAgeSeconds);
    assert.ok(Number.isSafeInteger(lead) && lead > 0, "invalid maxSlotLead");
    this.slotLead = BigInt(lead);
    this.program = new Program<Akt>(
      { ...idl, address: config.programId.toBase58() },
      { connection: rpc },
    );
    validateMint(config);
  }

  private pda(seed: string, suffix?: Buffer) {
    const seeds: Buffer[] = [Buffer.from(seed)];
    if (suffix) seeds.push(suffix);
    return web3.PublicKey.findProgramAddressSync(
      seeds,
      this.config.programId,
    )[0];
  }

  private async finalizedAccounts(address?: web3.PublicKey) {
    assert.equal(
      await rpcRead(() => this.rpc.getGenesisHash()),
      this.config.genesisHash,
      "wrong Solana cluster",
    );
    const snapshot = await rpcRead(() =>
      this.rpc.getMultipleAccountsInfoAndContext(
        [
          this.config.programId,
          this.pda("config"),
          this.pda("bridge"),
          this.config.mint,
          ...(address ? [address] : []),
        ],
        { commitment: "finalized" },
      ),
    );
    assert.ok(
      Number.isSafeInteger(snapshot.context.slot) && snapshot.context.slot > 0,
      "Solana node has no finalized positive slot",
    );
    const contextSlot = BigInt(snapshot.context.slot);
    const time = await rpcRead(() =>
      this.rpc.getBlockTime(snapshot.context.slot),
    );
    const age = time === null ? Infinity : Date.now() / 1000 - time;
    assert.ok(
      age >= -10 && age <= this.config.maxAgeSeconds,
      "stale Solana finalized state",
    );
    const [
      executable,
      launchAccount,
      bridgeAccount,
      mintAccount,
      statusAccount,
    ] = snapshot.value;
    assert.ok(executable?.executable, "mint program is not executable");
    for (const account of [launchAccount, bridgeAccount]) {
      assert.ok(
        account &&
          !account.executable &&
          account.owner.equals(this.config.programId),
        "wrong configuration owner",
      );
    }
    assert.ok(launchAccount && bridgeAccount && mintAccount);
    const launch = this.program.coder.accounts.decode<
      IdlAccounts<Akt>["launchConfig"]
    >("launchConfig", launchAccount.data);
    const bridge = this.program.coder.accounts.decode<
      IdlAccounts<Akt>["bridgeConfig"]
    >("bridgeConfig", bridgeAccount.data);
    assert.equal(launch.version, 2, "unsupported launch configuration");
    assert.ok(launch.mint.equals(this.config.mint), "wrong configured mint");
    assert.equal(
      bridge.chainId,
      this.config.akashChainId,
      "wrong bridge origin chain",
    );
    this.checkLightClient(bridge.lightClientProgram, contextSlot);
    const mint = unpackMint(this.config.mint, mintAccount, TOKEN_PROGRAM_ID);
    assert.ok(
      mint.isInitialized &&
        mint.decimals === 6 &&
        mint.freezeAuthority === null,
      "wrong AKT mint configuration",
    );
    assert.ok(
      mint.mintAuthority?.equals(this.pda("mint_authority")),
      "wrong mint authority",
    );
    return {
      contextSlot,
      statusAccount,
      bridgeEnabled: launch.bridgeEnabled,
      lightClientProgram: bridge.lightClientProgram,
    };
  }

  // Governance can re-pin the bridge at any time. A configured pin fails
  // closed on a different program. Otherwise record each change in slot order.
  private checkLightClient(current: web3.PublicKey, contextSlot: bigint) {
    const pinned = this.config.lightClientProgram;
    if (pinned) {
      if (!current.equals(pinned))
        throw new LightClientMismatchError(pinned, current);
      return;
    }
    const previous = this.followed;
    if (previous && contextSlot < previous.contextSlot) return;
    if (previous && !previous.program.equals(current))
      this.changes.push({ previous: previous.program, current, contextSlot });
    this.followed = { program: current, contextSlot };
  }

  // Light-client changes seen since the last call, oldest first.
  takeLightClientChanges() {
    return this.changes.splice(0);
  }

  async deployment() {
    const { contextSlot, bridgeEnabled } = await this.finalizedAccounts();
    return { contextSlot, bridgeEnabled };
  }

  // Reads the finalized bridge account; throws on a pin mismatch.
  async lightClient() {
    return (await this.finalizedAccounts()).lightClientProgram;
  }

  private statusAddress(record: Record) {
    return statusAddress(this.config.programId, record.id);
  }

  async observeFinalized(record: Record): Promise<FinalizedObserved> {
    const address = this.statusAddress(record);
    const { contextSlot, statusAccount } =
      await this.finalizedAccounts(address);
    if (!statusAccount) return { kind: "absent", contextSlot };
    assert.ok(
      !statusAccount.executable &&
        statusAccount.owner.equals(this.config.programId),
      "wrong transfer status owner",
    );
    const status = this.program.coder.accounts.decode<TransferStatus>(
      "transferStatus",
      statusAccount.data,
    );
    assert.ok(
      matchesAnchorId(status.id, record.id),
      "wrong status transfer ID",
    );
    assert.ok(
      BigInt(status.proofHeight.toString()) > record.height,
      "proof predates deposit commitment",
    );
    const slot = BigInt(status.resolvedAtSlot.toString());
    assert.ok(slot > 0n && slot <= contextSlot, "status is not finalized");
    let outcome: Outcome;
    if ("minted" in status.outcome) {
      assert.equal(
        BigInt(status.outcome.minted.amount.toString()),
        record.amount,
        "mint amount differs from Akash deposit",
      );
      assert.ok(
        status.outcome.minted.recipient.equals(record.recipient),
        "mint recipient differs from Akash deposit",
      );
      outcome = Outcome.OUTCOME_MINTED;
    } else if ("voided" in status.outcome) {
      outcome = Outcome.OUTCOME_VOIDED;
    } else {
      throw new Error("unknown transfer outcome");
    }
    return {
      kind: "resolved",
      contextSlot,
      address,
      status,
      outcome,
      slot,
      proofHeight: BigInt(status.proofHeight.toString()),
    };
  }

  // `reported` holds the Akash votes and governance observation for this
  // transfer.
  async observe(
    record: Record,
    reported: readonly Observation[] = [],
    previousOwnObservation?: Observation,
  ): Promise<Observed> {
    const observed = await this.observeFinalized(record);
    if (observed.kind === "absent")
      return this.absent(record, observed.contextSlot, reported);
    const { contextSlot, address, status, outcome, slot } = observed;
    // The caller may supply only this reporter's authenticated, stored vote.
    // Reuse its evidence after rechecking the immutable finalized status, so
    // pruning old transaction logs does not break already-observed transfers.
    const previous = previousOwnObservation;
    const signature =
      previous?.outcome === outcome &&
      previous.slot === slot &&
      previous.transactionSignature.length === 64 &&
      previous.transactionSignature.some((byte) => byte !== 0)
        ? Uint8Array.from(previous.transactionSignature)
        : await this.resolutionSignature(address, status, safeSlot(slot));
    return {
      kind: "resolved",
      contextSlot,
      observation: Observation.fromPartial({
        outcome,
        slot,
        transactionSignature: signature,
      }),
    };
  }

  // A missing status can support a veto only when every plausible reported
  // slot is at or below this node's finalized slot. Slots beyond slotLimit
  // cannot be finalized and are left for the policy to exclude and alert on.
  private async absent(
    record: Record,
    contextSlot: bigint,
    reported: readonly Observation[],
  ): Promise<Observed> {
    const slotLimit = contextSlot + this.slotLead;
    const slots = reported.map((observation) => observation.slot);
    assert.ok(
      slots.every((slot) => slot <= contextSlot || slot > slotLimit),
      "Solana node is behind reported evidence; absence cannot authorize a veto",
    );
    // The status is permanent, so its absence already disproves evidence at or
    // below contextSlot. A finalized transaction that still shows it means this
    // node is inconsistent and must not support a veto.
    for (const observation of reported) {
      if (observation.slot > contextSlot) continue;
      assert.ok(
        !(await this.showsReported(record, observation)),
        "reported Solana transaction contradicts missing status",
      );
    }
    return { kind: "absent", contextSlot, slotLimit };
  }

  private async showsReported(record: Record, observation: Observation) {
    const kind =
      observation.outcome === Outcome.OUTCOME_MINTED
        ? "minted"
        : observation.outcome === Outcome.OUTCOME_VOIDED
          ? "voided"
          : undefined;
    if (!kind || observation.transactionSignature.length !== 64) return false;
    const signature = utils.bytes.bs58.encode(observation.transactionSignature);
    const tx = await rpcRead(() =>
      this.rpc.getTransaction(signature, {
        commitment: "finalized",
        maxSupportedTransactionVersion: 1,
      }),
    );
    return (
      tx !== null &&
      BigInt(tx.slot) === observation.slot &&
      tx.transaction.signatures[0] === signature &&
      this.resolves(
        tx,
        this.statusAddress(record),
        (event) =>
          matchesAnchorId(event.id, record.id) &&
          BigInt(event.proofHeight.toString()) > record.height &&
          BigInt(event.resolvedAtSlot.toString()) === BigInt(tx.slot) &&
          kind in event.outcome &&
          (!("minted" in event.outcome) ||
            (event.outcome.minted.recipient.equals(record.recipient) &&
              BigInt(event.outcome.minted.amount.toString()) ===
                record.amount)),
      )
    );
  }

  // A resolution must succeed, write the status account and emit a matching
  // TransferResolved event from an AKT program frame.
  private resolves(
    tx: Executed,
    address: web3.PublicKey,
    matches: (event: TransferResolved) => boolean,
  ) {
    if (!tx.meta || tx.meta.err !== null || !tx.meta.logMessages) return false;
    const { message } = tx.transaction;
    try {
      const keys = message.getAccountKeys({
        accountKeysFromLookups: tx.meta.loadedAddresses,
      });
      let writesStatus = false;
      for (let i = 0; i < keys.length; i++) {
        if (keys.get(i)?.equals(address) && message.isAccountWritable(i))
          writesStatus = true;
      }
      return (
        writesStatus &&
        programEvents(tx.meta.logMessages, this.config.programId, (data) =>
          this.program.coder.events.decode(data),
        ).some(
          (event) => event.name === "transferResolved" && matches(event.data),
        )
      );
    } catch {
      return false;
    }
  }

  private async resolutionSignature(
    address: web3.PublicKey,
    status: TransferStatus,
    slot: number,
  ) {
    // Any transaction can reference the status PDA and flood its signature
    // history. Search only the block at the permanent resolution slot. A
    // skipped, missing or pruned block stops a new report.
    // Mainnet blocks carry version 1 transactions (SIMD-0385), and a node
    // refuses the whole block to a client that accepts only older versions.
    // Accept exactly the newest version web3.js parses, so a future version
    // fails as missing evidence instead of as a parse error.
    const block = await this.rpc
      .getBlock(slot, {
        commitment: "finalized",
        maxSupportedTransactionVersion: 1,
        transactionDetails: "full",
        rewards: false,
      })
      .catch(() => null);
    if (!block)
      throw new Error(
        "resolution evidence unavailable: block missing from owned node history",
      );
    assert.equal(
      BigInt(status.resolvedAtSlot.toString()),
      BigInt(slot),
      "resolution slot mismatch",
    );
    const found = block.transactions.filter((tx) =>
      this.resolves(
        tx,
        address,
        (event) =>
          Buffer.from(event.id.owner).equals(Buffer.from(status.id.owner)) &&
          event.id.height.eq(status.id.height) &&
          event.id.sequence.eq(status.id.sequence) &&
          event.proofHeight.eq(status.proofHeight) &&
          event.resolvedAtSlot.eq(status.resolvedAtSlot) &&
          (event.outcome.minted && status.outcome.minted
            ? event.outcome.minted.recipient.equals(
                status.outcome.minted.recipient,
              ) && event.outcome.minted.amount.eq(status.outcome.minted.amount)
            : Boolean(
                event.outcome.voided &&
                status.outcome.voided &&
                isDeepStrictEqual(
                  event.outcome.voided.reason,
                  status.outcome.voided.reason,
                ),
              )),
      ),
    );
    // The status account is created once, so one transaction can resolve it.
    assert.ok(found.length < 2, "multiple resolution transactions in block");
    if (!found[0])
      throw new Error("resolution evidence unavailable in its block");
    const signature = utils.bytes.bs58.decode(
      found[0].transaction.signatures[0],
    );
    assert.equal(signature.length, 64);
    assert.ok(signature.some((byte) => byte !== 0));
    return Uint8Array.from(signature);
  }
}
