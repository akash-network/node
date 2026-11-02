import { web3 } from "@coral-xyz/anchor";
import {
  ACCOUNT_SIZE,
  AccountLayout,
  AccountState,
  TOKEN_2022_PROGRAM_ID,
  TOKEN_PROGRAM_ID,
} from "@solana/spl-token";
import { Sink_Status as Status } from "@akashnetwork/chain-sdk/private-types/akash.v1";

// Relayer decisions without RPC access, so they can be tested directly.

// getMultipleAccounts accepts at most 100 addresses per request.
export const STATUS_BATCH = 100;

export function idBatches(from: bigint, to: bigint, size = STATUS_BATCH) {
  const batches: bigint[][] = [];
  for (let start = from; start <= to; start += BigInt(size)) {
    const batch: bigint[] = [];
    for (let id = start; id <= to && id < start + BigInt(size); id++)
      batch.push(id);
    batches.push(batch);
  }
  return batches;
}

// Solana statuses are permanent. The cursor is the lowest scanned ID without
// one, or the next unissued ID once every scanned record is resolved.
export function nextCursor(unresolved: bigint[], last: bigint) {
  return unresolved.length ? unresolved[0] : last + 1n;
}

// An idle client is refreshed after this share of its trusting period, below
// the monitor's warning at 25%.
export const DEFAULT_REFRESH_FRACTION = 1 / 5;

export function relaySettings(env: Record<string, string | undefined>) {
  const refreshFraction = Number(
    env.AKT_RELAY_REFRESH_FRACTION ?? DEFAULT_REFRESH_FRACTION,
  );
  if (!(refreshFraction > 0 && refreshFraction < 1))
    throw new Error("AKT_RELAY_REFRESH_FRACTION must be between 0 and 1");
  const retryMaxSeconds = Number(env.AKT_RELAY_RETRY_MAX_SECONDS ?? 900);
  if (!(Number.isSafeInteger(retryMaxSeconds) && retryMaxSeconds >= 6))
    throw new Error("AKT_RELAY_RETRY_MAX_SECONDS must be an integer >= 6");
  const pruneSeconds = Number(env.AKT_RELAY_PRUNE_INTERVAL_SECONDS ?? 600);
  if (!(Number.isSafeInteger(pruneSeconds) && pruneSeconds >= 60))
    throw new Error(
      "AKT_RELAY_PRUNE_INTERVAL_SECONDS must be an integer >= 60",
    );
  const maxLamports = Number(
    env.AKT_RELAY_MAX_LAMPORTS_PER_TRANSFER ??
      DEFAULT_MAX_LAMPORTS_PER_TRANSFER,
  );
  if (!(Number.isSafeInteger(maxLamports) && maxLamports > 0))
    throw new Error(
      "AKT_RELAY_MAX_LAMPORTS_PER_TRANSFER must be a positive integer",
    );
  const maxSignatures = Number(
    env.AKT_RELAY_MAX_HEADER_SIGNATURES ?? DEFAULT_MAX_HEADER_SIGNATURES,
  );
  if (!(
    Number.isSafeInteger(maxSignatures) &&
    maxSignatures > 0 &&
    maxSignatures <= 128
  ))
    throw new Error(
      "AKT_RELAY_MAX_HEADER_SIGNATURES must be an integer from 1 to 128",
    );
  return {
    refreshFraction,
    retryMaxMs: retryMaxSeconds * 1000,
    pruneIntervalMs: pruneSeconds * 1000,
    maxLamportsPerTransfer: maxLamports,
    maxHeaderSignatures: maxSignatures,
  };
}

// A normal relay pays about 3.5M lamports: the status account and recipient
// ATA rent plus a few fees. The default leaves room for many failed attempts.
export const DEFAULT_MAX_LAMPORTS_PER_TRANSFER = 5_000_000;

// Exponential delay after each failure, capped. A success clears the entry.
export class RetrySchedule {
  private readonly entries = new Map<
    bigint,
    { failures: number; retryAt: number }
  >();
  constructor(
    private readonly baseMs: number,
    private readonly maxMs: number,
  ) {}
  ready(id: bigint, now: number) {
    const entry = this.entries.get(id);
    return !entry || now >= entry.retryAt;
  }
  failed(id: bigint, now: number) {
    const failures = (this.entries.get(id)?.failures ?? 0) + 1;
    const delayMs = Math.min(
      this.maxMs,
      this.baseMs * 2 ** Math.min(failures - 1, 30),
    );
    this.entries.set(id, { failures, retryAt: now + delayMs });
    return { failures, delayMs };
  }
  clear(id: bigint) {
    this.entries.delete(id);
  }
  forgetBelow(id: bigint) {
    for (const key of this.entries.keys())
      if (key < id) this.entries.delete(key);
  }
}

type Account = { owner: web3.PublicKey; executable: boolean; data: Buffer };

// Mirrors the recipient checks in mint_transfer. A missing account reaches
// the program as an empty system-owned account.
export function recipientMints(input: {
  recipient: web3.PublicKey;
  account: Account | null;
  ata: Account | null;
  mint: web3.PublicKey;
  mintAuthority: web3.PublicKey;
  vaults: web3.PublicKey[];
}) {
  const { recipient, account, ata } = input;
  const owner = account?.owner ?? web3.SystemProgram.programId;
  const forbidden =
    account?.executable === true ||
    recipient.equals(web3.PublicKey.default) ||
    recipient.equals(input.mintAuthority) ||
    owner.equals(TOKEN_PROGRAM_ID) ||
    owner.equals(TOKEN_2022_PROGRAM_ID);
  const wallet =
    owner.equals(web3.SystemProgram.programId) &&
    web3.PublicKey.isOnCurve(recipient.toBytes());
  return (
    !forbidden &&
    (wallet || input.vaults.some((vault) => vault.equals(recipient))) &&
    ataUsable(ata, input.mint, recipient)
  );
}

// Mirrors ata_usable: a reassigned or foreign ATA voids the transfer.
function ataUsable(
  ata: Account | null,
  mint: web3.PublicKey,
  recipient: web3.PublicKey,
) {
  if (!ata || ata.owner.equals(web3.SystemProgram.programId))
    return !ata || ata.data.length === 0;
  if (!ata.owner.equals(TOKEN_PROGRAM_ID) || ata.data.length !== ACCOUNT_SIZE)
    return false;
  const token = AccountLayout.decode(ata.data);
  return (
    token.mint.equals(mint) &&
    token.owner.equals(recipient) &&
    token.state === AccountState.Initialized
  );
}

// In the honest flow an Akash void or refund follows a Solana void. With no
// Solana status, that state comes from false void reports or a governance
// confirmation, and minting would leave the refund as unbacked supply.
export function unbackedSettlement(akash: Status) {
  if (akash === Status.STATUS_VOIDED)
    return "akash-voided-without-solana-status";
  if (akash === Status.STATUS_REFUNDED)
    return "akash-refunded-without-solana-status";
  return undefined;
}

// Alert when a state is first seen and again every repeatMs while it lasts.
export class AlertSchedule {
  private readonly last = new Map<bigint, number>();
  constructor(private readonly repeatMs: number) {}
  due(id: bigint, now: number) {
    const at = this.last.get(id);
    if (at !== undefined && now - at < this.repeatMs) return false;
    this.last.set(id, now);
    return true;
  }
  clear(id: bigint) {
    this.last.delete(id);
  }
}

export interface MintPolicyView {
  enabled: boolean;
  perTransferMax: bigint;
  dailyMintCap: bigint;
  mintDay: bigint;
  mintedToday: bigint;
}

// Why mint_transfer would fail for a retryable reason, mirroring
// BridgeConfig::next_mint_total in state.rs and instructions/mint_transfer.rs.
// The relayer waits for these without paying. Invalid recipients still need
// the bridge enabled but void without using the caps. `reserved` counts
// earlier mints planned for the same pass.
export function mintWait(
  policy: MintPolicyView,
  nowSeconds: bigint,
  amount: bigint,
  mints: boolean,
  reserved = 0n,
) {
  if (!policy.enabled) return "bridge-paused";
  if (!mints) return undefined;
  const day = nowSeconds / 86_400n;
  if (day < policy.mintDay) return "solana-clock-behind";
  if (amount > policy.perTransferMax) return "per-transfer-limit";
  const used = (day === policy.mintDay ? policy.mintedToday : 0n) + reserved;
  if (used + amount > policy.dailyMintCap) return "daily-limit";
  return undefined;
}

// Each update stores a rent-holding consensus state. Update only when a
// record needs a newer proof height that the target header provides, or when
// the client has used the configured share of its trusting period.
export function headerRefresh(input: {
  latest: number;
  target: number;
  required: number[];
  ageSeconds: number;
  trustingPeriodSeconds: number;
  refreshFraction: number;
}): "proof" | "age" | undefined {
  if (input.target <= input.latest) return undefined;
  if (
    input.required.some(
      (height) => height > input.latest && height <= input.target,
    )
  )
    return "proof";
  if (input.ageSeconds >= input.trustingPeriodSeconds * input.refreshFraction)
    return "age";
  return undefined;
}

// The last read before the first paid transaction. Skip when Akash now shows
// a void or refund, or when a void order appeared or vanished after planning.
export function finalCheck(
  plannedVoidOrdered: boolean,
  latest: { voidOrdered: boolean; status: Status },
) {
  return (
    unbackedSettlement(latest.status) ??
    (latest.voidOrdered !== plannedVoidOrdered
      ? "void-order-changed"
      : undefined)
  );
}

export interface SpendEntry {
  spentLamports: number;
  attempts: number;
}

// Lamports paid per transfer: every fee, plus the rent the resolving
// transaction locks (status account, recipient ATA). Proof-buffer rent is
// returned when the buffer closes, so it is not spent. Past the limit the
// relayer stops paying for that transfer until an operator resets it.
export class SpendLedger {
  readonly entries: Map<bigint, SpendEntry>;
  constructor(entries: Iterable<[bigint, SpendEntry]> = []) {
    this.entries = new Map(entries);
  }
  spent(id: bigint) {
    return this.entries.get(id)?.spentLamports ?? 0;
  }
  exhausted(id: bigint, maxLamports: number) {
    return this.spent(id) > maxLamports;
  }
  attempt(id: bigint) {
    const entry = this.entries.get(id) ?? { spentLamports: 0, attempts: 0 };
    this.entries.set(id, { ...entry, attempts: entry.attempts + 1 });
  }
  charge(id: bigint, lamports: number) {
    if (!Number.isSafeInteger(lamports))
      throw new Error("spend must be an integer");
    const entry = this.entries.get(id) ?? { spentLamports: 0, attempts: 0 };
    this.entries.set(id, {
      ...entry,
      spentLamports: entry.spentLamports + lamports,
    });
  }
  // Resolved transfers are never paid for again.
  forgetBelow(id: bigint) {
    let changed = false;
    for (const key of this.entries.keys())
      if (key < id) changed = this.entries.delete(key) || changed;
    return changed;
  }
  reset(id: bigint | "all") {
    if (id !== "all") return this.entries.delete(id);
    const changed = this.entries.size > 0;
    this.entries.clear();
    return changed;
  }
  toJSON(identity: { genesis: string; programId: string }) {
    return {
      version: 1,
      ...identity,
      transfers: Object.fromEntries(
        [...this.entries]
          .sort(([a], [b]) => (a < b ? -1 : 1))
          .map(([id, entry]) => [id.toString(), entry]),
      ),
    };
  }
  static parse(
    value: unknown,
    identity: { genesis: string; programId: string },
  ) {
    if (value === undefined) return new SpendLedger();
    const saved = record(value);
    if (
      saved.version !== 1 ||
      saved.genesis !== identity.genesis ||
      saved.programId !== identity.programId
    )
      throw new Error("spend ledger belongs to another deployment");
    const entries: [bigint, SpendEntry][] = [];
    for (const [key, raw] of Object.entries(record(saved.transfers))) {
      const entry = record(raw);
      if (
        !/^[1-9][0-9]*$/.test(key) ||
        !Number.isSafeInteger(entry.spentLamports) ||
        !Number.isSafeInteger(entry.attempts) ||
        Number(entry.attempts) < 0
      )
        throw new Error(`invalid spend ledger entry ${key}`);
      entries.push([
        BigInt(key),
        {
          spentLamports: Number(entry.spentLamports),
          attempts: Number(entry.attempts),
        },
      ]);
    }
    return new SpendLedger(entries);
  }
}
function record(value: unknown): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value))
    throw new Error("spend ledger must be a JSON object");
  return Object.fromEntries(Object.entries(value));
}

// CometBFT /commit and /validators JSON entries.
export type CommitSignature = {
  block_id_flag: number;
  validator_address: string;
  timestamp: string;
  signature: string | null;
};
export type SetMember = { address: string; voting_power: string };
const FLAG_ABSENT = 1;
const FLAG_COMMIT = 2;
// The form CometBFT gives an omitted vote.
export const ABSENT_SIGNATURE: CommitSignature = {
  block_id_flag: FLAG_ABSENT,
  validator_address: "",
  timestamp: "0001-01-01T00:00:00Z",
  signature: null,
};
// Commits above this many signatures exceeded compute or heap at assembly.
export const DEFAULT_MAX_HEADER_SIGNATURES = 100;

// Above maxSignatures present votes, keep the fewest highest-power signers
// the verifier still accepts and mark the rest Absent. It needs more than 2/3
// of the new set's power and, when the update skips heights, more than the
// trust level of the trusted set's power. Each threshold gets marginBps of its
// set's power on top when the signers allow it.
export function selectCommitSignatures(input: {
  signatures: CommitSignature[];
  validators: SetMember[];
  trusted?: {
    validators: SetMember[];
    numerator: bigint;
    denominator: bigint;
  };
  maxSignatures: number;
  limit: number;
  marginBps?: number;
}):
  | { signatures: CommitSignature[]; present: number; kept: number }
  | { unsupported: string } {
  const { signatures } = input;
  const present = signatures.filter(
    (sig) => sig.block_id_flag === FLAG_COMMIT,
  ).length;
  if (present <= input.maxSignatures)
    return { signatures, present, kept: present };
  const powers = (set: SetMember[]) =>
    new Map(
      set.map((member) => [
        member.address.toUpperCase(),
        BigInt(member.voting_power),
      ]),
    );
  const total = (power: Map<string, bigint>) =>
    [...power.values()].reduce((sum, value) => sum + value, 0n);
  const fresh = powers(input.validators);
  const freshTotal = total(fresh);
  const trusted = input.trusted && powers(input.trusted.validators);
  const trustedTotal = trusted ? total(trusted) : 0n;
  const level = input.trusted ?? { numerator: 1n, denominator: 3n };
  // tallied / all > numerator / denominator + margin / 10,000
  const passes = (
    tallied: bigint,
    all: bigint,
    numerator: bigint,
    denominator: bigint,
    margin: bigint,
  ) =>
    tallied * denominator * 10_000n >
    all * (numerator * 10_000n + margin * denominator);
  const seen = new Set<string>();
  const candidates: { index: number; address: string }[] = [];
  for (const [index, sig] of signatures.entries()) {
    const address = sig.validator_address.toUpperCase();
    if (sig.block_id_flag !== FLAG_COMMIT || !fresh.has(address)) continue;
    if (seen.has(address)) continue;
    seen.add(address);
    candidates.push({ index, address });
  }
  const pick = (margin: bigint) => {
    const kept = new Set<number>();
    let freshTally = 0n,
      trustedTally = 0n;
    const add = (entry: { index: number; address: string }) => {
      kept.add(entry.index);
      freshTally += fresh.get(entry.address) ?? 0n;
      trustedTally += trusted?.get(entry.address) ?? 0n;
    };
    const byPower = (power: Map<string, bigint>) =>
      candidates
        .filter((entry) => !kept.has(entry.index))
        .sort((a, b) => {
          const difference =
            (power.get(b.address) ?? 0n) - (power.get(a.address) ?? 0n);
          return difference ? (difference > 0n ? 1 : -1) : a.index - b.index;
        });
    // Trusted signers first: most also sign for the new set, so they count
    // toward both thresholds.
    if (trusted)
      for (const entry of byPower(trusted)) {
        if (
          passes(
            trustedTally,
            trustedTotal,
            level.numerator,
            level.denominator,
            margin,
          ) ||
          !trusted.has(entry.address)
        )
          break;
        add(entry);
      }
    for (const entry of byPower(fresh)) {
      if (passes(freshTally, freshTotal, 2n, 3n, margin)) break;
      add(entry);
    }
    return { kept, freshTally, trustedTally };
  };
  const strict = (choice: ReturnType<typeof pick>) =>
    passes(choice.freshTally, freshTotal, 2n, 3n, 0n) &&
    (!trusted ||
      passes(
        choice.trustedTally,
        trustedTotal,
        level.numerator,
        level.denominator,
        0n,
      ));
  // With the margin if the signers allow it and it fits, else the fewest
  // that pass.
  let choice = pick(BigInt(input.marginBps ?? 100));
  if (!strict(choice) || choice.kept.size > input.limit) choice = pick(0n);
  if (!passes(choice.freshTally, freshTotal, 2n, 3n, 0n))
    return {
      unsupported: `signers hold ${choice.freshTally} of ${freshTotal} voting power; the verifier needs more than 2/3`,
    };
  if (!strict(choice))
    return {
      unsupported: `signers hold ${choice.trustedTally} of ${trustedTotal} trusted voting power; the verifier needs more than ${level.numerator}/${level.denominator}`,
    };
  if (choice.kept.size > input.limit)
    return {
      unsupported: `a quorum needs ${choice.kept.size} of ${present} signatures; at most ${input.limit} fit one header update`,
    };
  return {
    signatures: signatures.map((sig, index) =>
      choice.kept.has(index) ? sig : ABSENT_SIGNATURE,
    ),
    present,
    kept: choice.kept.size,
  };
}
