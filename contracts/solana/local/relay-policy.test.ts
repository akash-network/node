import assert from "node:assert/strict";
import { test } from "node:test";
import { web3 } from "@coral-xyz/anchor";
import {
  ACCOUNT_SIZE,
  AccountLayout,
  TOKEN_2022_PROGRAM_ID,
  TOKEN_PROGRAM_ID,
} from "@solana/spl-token";
import {
  QueryOpenTransfersRequest,
  Sink_Status as Status,
  TransferState,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { BinaryWriter, WireType } from "@bufbuild/protobuf/wire";
import {
  AlertSchedule,
  RetrySchedule,
  SpendLedger,
  finalCheck,
  headerRefresh,
  idBatches,
  mintWait,
  nextCursor,
  recipientMints,
  relaySettings,
  selectCommitSignatures,
  unbackedSettlement,
  type CommitSignature,
  type SetMember,
} from "./relay-policy";
import { assemblyExhaustion } from "./header-upload";
import {
  decodeAppStateAccessManager,
  decodeBridgePin,
  lightClientProblem,
  relayerAccess,
} from "./light-client";
import {
  OPEN_TRANSFERS_PATH,
  decodeOpenTransfersResponse,
  encodeOpenTransfersRequest,
  openTransfers,
} from "./open-transfers";
import idl from "../target/idl/akt_solana.json";
import clientIdl from "../idls/ics07_tendermint.json";

test("status reads are batched from the cursor without a record limit", () => {
  assert.deepEqual(idBatches(5n, 4n), []);
  assert.deepEqual(idBatches(1n, 3n, 2), [[1n, 2n], [3n]]);
  const batches = idBatches(1n, 1001n);
  assert.equal(batches.length, 11);
  assert.ok(batches.every((batch) => batch.length <= 100));
  assert.equal(batches[0][0], 1n);
  assert.equal(batches.at(-1)?.at(-1), 1001n);
  assert.deepEqual(idBatches(1001n, 1001n), [[1001n]]);
});

test("cursor stops at the lowest unresolved record", () => {
  assert.equal(nextCursor([7n, 9n], 12n), 7n);
  assert.equal(nextCursor([], 12n), 13n);
  assert.equal(nextCursor([], 0n), 1n);
});

test("headers refresh only for a needed proof or an aging client", () => {
  const base = {
    latest: 100,
    target: 180,
    required: [] as number[],
    ageSeconds: 600,
    trustingPeriodSeconds: 86_400,
    refreshFraction: 1 / 3,
  };
  // Pending, voided and held records with Solana statuses add no heights.
  assert.equal(headerRefresh(base), undefined);
  assert.equal(headerRefresh({ ...base, required: [90, 100] }), undefined);
  assert.equal(headerRefresh({ ...base, required: [90, 101] }), "proof");
  // A record newer than the target waits for a later header.
  assert.equal(headerRefresh({ ...base, required: [181] }), undefined);
  assert.equal(headerRefresh({ ...base, ageSeconds: 28_799 }), undefined);
  assert.equal(headerRefresh({ ...base, ageSeconds: 28_800 }), "age");
  assert.equal(
    headerRefresh({ ...base, target: 100, required: [101], ageSeconds: 9e9 }),
    undefined,
  );
});

test("settings default and reject bad values", () => {
  assert.equal(relaySettings({}).refreshFraction, 1 / 5);
  assert.equal(relaySettings({}).retryMaxMs, 900_000);
  assert.equal(
    relaySettings({ AKT_RELAY_RETRY_MAX_SECONDS: "60" }).retryMaxMs,
    60_000,
  );
  for (const value of ["", "5", "1.5", "x"])
    assert.throws(() => relaySettings({ AKT_RELAY_RETRY_MAX_SECONDS: value }));
  assert.equal(
    relaySettings({ AKT_RELAY_REFRESH_FRACTION: "0.25" }).refreshFraction,
    0.25,
  );
  for (const value of ["", "0", "1", "-0.1", "abc"])
    assert.throws(() => relaySettings({ AKT_RELAY_REFRESH_FRACTION: value }));
  assert.equal(relaySettings({}).pruneIntervalMs, 600_000);
  assert.equal(
    relaySettings({ AKT_RELAY_PRUNE_INTERVAL_SECONDS: "60" }).pruneIntervalMs,
    60_000,
  );
  for (const value of ["", "59", "1.5", "x"])
    assert.throws(() =>
      relaySettings({ AKT_RELAY_PRUNE_INTERVAL_SECONDS: value }),
    );
});

test("failed transfers back off exponentially up to the ceiling", () => {
  const retries = new RetrySchedule(6000, 60_000);
  assert.ok(retries.ready(4n, 0));
  const delays = [1, 2, 3, 4, 5, 6].map(() => retries.failed(4n, 0).delayMs);
  assert.deepEqual(delays, [6000, 12_000, 24_000, 48_000, 60_000, 60_000]);
  assert.equal(retries.ready(4n, 59_999), false);
  assert.ok(retries.ready(4n, 60_000));
  // Other transfers keep their own schedule.
  assert.ok(retries.ready(5n, 0));
  retries.clear(4n);
  assert.equal(retries.failed(4n, 0).delayMs, 6000);
  retries.failed(9n, 0);
  retries.forgetBelow(5n);
  assert.ok(retries.ready(4n, 0));
  assert.equal(retries.ready(9n, 0), false);
});

test("recipient prediction mirrors mint_transfer", () => {
  const wallet = web3.Keypair.generate().publicKey;
  const mint = web3.Keypair.generate().publicKey;
  const authority = web3.PublicKey.findProgramAddressSync(
    [Buffer.from("mint_authority")],
    web3.Keypair.generate().publicKey,
  )[0];
  const empty = Buffer.alloc(0);
  const system = {
    owner: web3.SystemProgram.programId,
    executable: false,
    data: empty,
  };
  const base = {
    recipient: wallet,
    account: null,
    ata: null,
    mint,
    mintAuthority: authority,
    vaults: [] as web3.PublicKey[],
  };
  const mints = (input: Partial<Parameters<typeof recipientMints>[0]>) =>
    recipientMints({ ...base, ...input });
  assert.ok(mints({}));
  assert.ok(mints({ account: system }));
  for (const owner of [TOKEN_PROGRAM_ID, TOKEN_2022_PROGRAM_ID])
    assert.equal(
      mints({ account: { owner, executable: false, data: empty } }),
      false,
    );
  assert.equal(mints({ account: { ...system, executable: true } }), false);
  assert.equal(mints({ recipient: web3.PublicKey.default }), false);
  // An off-curve address voids unless it is an allowed vault.
  const vault = {
    owner: web3.Keypair.generate().publicKey,
    executable: false,
    data: empty,
  };
  assert.equal(mints({ recipient: authority, mintAuthority: wallet }), false);
  assert.equal(
    mints({ recipient: authority, account: vault, mintAuthority: wallet }),
    false,
  );
  assert.ok(
    mints({
      recipient: authority,
      account: vault,
      mintAuthority: wallet,
      vaults: [authority],
    }),
  );
  assert.equal(mints({ recipient: authority, vaults: [authority] }), false);
});

test("an existing ATA must be the recipient's initialized AKT account", () => {
  const wallet = web3.Keypair.generate().publicKey;
  const mint = web3.Keypair.generate().publicKey;
  const token = (owner: web3.PublicKey, accountMint = mint, state = 1) => {
    const data = Buffer.alloc(ACCOUNT_SIZE);
    AccountLayout.encode(
      {
        mint: accountMint,
        owner,
        amount: 0n,
        delegateOption: 0,
        delegate: web3.PublicKey.default,
        state,
        isNativeOption: 0,
        isNative: 0n,
        delegatedAmount: 0n,
        closeAuthorityOption: 0,
        closeAuthority: web3.PublicKey.default,
      },
      data,
    );
    return { owner: TOKEN_PROGRAM_ID, executable: false, data };
  };
  const mints = (ata: Parameters<typeof recipientMints>[0]["ata"]) =>
    recipientMints({
      recipient: wallet,
      account: null,
      ata,
      mint,
      mintAuthority: web3.Keypair.generate().publicKey,
      vaults: [],
    });
  const other = web3.Keypair.generate().publicKey;
  assert.ok(mints(null));
  assert.ok(mints(token(wallet)));
  // Reassigned owner, wrong mint, frozen and uninitialized accounts void.
  assert.equal(mints(token(other)), false);
  assert.equal(mints(token(wallet, other)), false);
  assert.equal(mints(token(wallet, mint, 2)), false);
  assert.equal(mints(token(wallet, mint, 0)), false);
  assert.equal(
    mints({ ...token(wallet), owner: TOKEN_2022_PROGRAM_ID }),
    false,
  );
  assert.equal(
    mints({ ...token(wallet), data: token(wallet).data.subarray(1) }),
    false,
  );
  const system = {
    owner: web3.SystemProgram.programId,
    executable: false,
  };
  assert.ok(mints({ ...system, data: Buffer.alloc(0) }));
  assert.equal(mints({ ...system, data: Buffer.alloc(1) }), false);
});

test("pause and caps wait without paying; invalid recipients still void", () => {
  const day = 20_000n;
  const now = day * 86_400n + 5n;
  const policy = {
    enabled: true,
    perTransferMax: 100n,
    dailyMintCap: 250n,
    mintDay: day,
    mintedToday: 100n,
  };
  assert.equal(mintWait(policy, now, 100n, true), undefined);
  assert.equal(mintWait(policy, now, 101n, true), "per-transfer-limit");
  assert.equal(mintWait(policy, now, 100n, true, 100n), "daily-limit");
  assert.equal(mintWait(policy, now, 50n, true, 100n), undefined);
  // A new UTC day starts from zero.
  assert.equal(
    mintWait({ ...policy, mintedToday: 250n }, now + 86_400n, 100n, true),
    undefined,
  );
  assert.equal(
    mintWait({ ...policy, mintedToday: 250n }, now, 1n, true),
    "daily-limit",
  );
  assert.equal(
    mintWait({ ...policy, mintDay: day + 1n }, now, 1n, true),
    "solana-clock-behind",
  );
  assert.equal(mintWait(policy, now, 1000n, false), undefined);
  assert.equal(
    mintWait({ ...policy, enabled: false }, now, 1n, true),
    "bridge-paused",
  );
  assert.equal(
    mintWait({ ...policy, enabled: false }, now, 1n, false),
    "bridge-paused",
  );
});

test("records voided or refunded on Akash without a Solana status never mint", () => {
  assert.equal(
    unbackedSettlement(Status.STATUS_VOIDED),
    "akash-voided-without-solana-status",
  );
  assert.equal(
    unbackedSettlement(Status.STATUS_REFUNDED),
    "akash-refunded-without-solana-status",
  );
  // Pending and minted records keep the normal relay path.
  assert.equal(unbackedSettlement(Status.STATUS_PENDING), undefined);
  assert.equal(unbackedSettlement(Status.STATUS_MINTED), undefined);
});

test("unbacked settlement alerts when first seen and while it persists", () => {
  const alerts = new AlertSchedule(900_000);
  assert.ok(alerts.due(3n, 0));
  assert.equal(alerts.due(3n, 3000), false);
  assert.equal(alerts.due(3n, 899_999), false);
  assert.ok(alerts.due(3n, 900_000));
  assert.ok(alerts.due(4n, 900_000));
  // A record that leaves the state alerts again when it returns.
  alerts.clear(3n);
  assert.ok(alerts.due(3n, 900_001));
});

test("the spend limit defaults above a normal relay and is configurable", () => {
  assert.equal(relaySettings({}).maxLamportsPerTransfer, 5_000_000);
  assert.equal(
    relaySettings({ AKT_RELAY_MAX_LAMPORTS_PER_TRANSFER: "9000000" })
      .maxLamportsPerTransfer,
    9_000_000,
  );
  for (const value of ["", "0", "-1", "1.5", "x"])
    assert.throws(() =>
      relaySettings({ AKT_RELAY_MAX_LAMPORTS_PER_TRANSFER: value }),
    );
});

test("the spend ledger stops paying past the limit and survives restarts", () => {
  const identity = { genesis: "G", programId: "P" };
  const ledger = new SpendLedger();
  ledger.attempt(7n);
  // A normal mint: fees plus status and ATA rent.
  ledger.charge(7n, 3_470_000);
  assert.equal(ledger.exhausted(7n, 5_000_000), false);
  for (let i = 0; i < 10; i++) ledger.charge(7n, 5_000);
  ledger.attempt(7n);
  assert.equal(ledger.spent(7n), 3_520_000);
  ledger.charge(7n, 1_480_000);
  // At the limit is still allowed; over it stops.
  assert.equal(ledger.exhausted(7n, 5_000_000), false);
  ledger.charge(7n, 1);
  assert.equal(ledger.exhausted(7n, 5_000_000), true);
  assert.equal(ledger.exhausted(8n, 5_000_000), false);
  assert.throws(() => ledger.charge(7n, 0.5));
  // A restart reads the same state back.
  const saved = JSON.parse(JSON.stringify(ledger.toJSON(identity)));
  assert.deepEqual(saved.transfers["7"], {
    spentLamports: 5_000_001,
    attempts: 2,
  });
  const restored = SpendLedger.parse(saved, identity);
  assert.equal(restored.exhausted(7n, 5_000_000), true);
  assert.equal(restored.entries.get(7n)?.attempts, 2);
  assert.equal(SpendLedger.parse(undefined, identity).entries.size, 0);
  for (const other of [
    { genesis: "X", programId: "P" },
    { genesis: "G", programId: "X" },
  ])
    assert.throws(() => SpendLedger.parse(saved, other), /another deployment/);
  for (const transfers of [
    { "0": { spentLamports: 1, attempts: 1 } },
    { "07": { spentLamports: 1, attempts: 1 } },
    { "7": { spentLamports: "1", attempts: 1 } },
    { "7": { spentLamports: 1, attempts: -1 } },
  ])
    assert.throws(() => SpendLedger.parse({ ...saved, transfers }, identity));
  // An operator reset, or resolution below the floor, clears entries.
  restored.charge(9n, 10);
  assert.equal(restored.reset(7n), true);
  assert.equal(restored.reset(7n), false);
  assert.equal(restored.exhausted(7n, 5_000_000), false);
  assert.equal(restored.forgetBelow(9n), false);
  assert.equal(restored.forgetBelow(10n), true);
  assert.equal(restored.entries.size, 0);
  restored.charge(3n, 1);
  assert.equal(restored.reset("all"), true);
  assert.equal(restored.reset("all"), false);
});

test("the final Akash check skips voids, refunds and changed void orders", () => {
  const pending = Status.STATUS_PENDING;
  assert.equal(
    finalCheck(false, { voidOrdered: false, status: pending }),
    undefined,
  );
  assert.equal(
    finalCheck(true, { voidOrdered: true, status: pending }),
    undefined,
  );
  // Governance may confirm a mint; Akash then backs it.
  assert.equal(
    finalCheck(false, { voidOrdered: false, status: Status.STATUS_MINTED }),
    undefined,
  );
  assert.equal(
    finalCheck(false, { voidOrdered: false, status: Status.STATUS_VOIDED }),
    "akash-voided-without-solana-status",
  );
  assert.equal(
    finalCheck(true, { voidOrdered: true, status: Status.STATUS_REFUNDED }),
    "akash-refunded-without-solana-status",
  );
  assert.equal(
    finalCheck(false, { voidOrdered: true, status: pending }),
    "void-order-changed",
  );
});

test("relayer access distinguishes members, open relaying and none", () => {
  const payer = web3.Keypair.generate().publicKey;
  const program = web3.Keypair.generate().publicKey;
  const other = web3.Keypair.generate().publicKey;
  const role = (roleId: number, members: web3.PublicKey[]) => ({
    roleId,
    members,
  });
  assert.equal(
    relayerAccess([role(1, [other, payer])], payer, program),
    "member",
  );
  assert.equal(relayerAccess([role(1, [program])], payer, program), "open");
  assert.equal(
    relayerAccess([role(1, [program, payer])], payer, program),
    "member",
  );
  // The admin role and other programs' markers do not count.
  assert.equal(
    relayerAccess([role(0, [payer, program])], payer, program),
    "none",
  );
  assert.equal(relayerAccess([role(1, [other])], payer, program), "none");
  assert.equal(relayerAccess([], payer, program), "none");
});

test("the bridge account names the light client and chain", () => {
  const program = web3.Keypair.generate().publicKey;
  const manager = web3.Keypair.generate().publicKey;
  const discriminator = (
    accounts: { name: string; discriminator: number[] }[],
    name: string,
  ) => Buffer.from(accounts.find((a) => a.name === name)!.discriminator);
  const chain = Buffer.from("akashnet-2");
  const length = Buffer.alloc(4);
  length.writeUInt32LE(chain.length);
  const bridge = Buffer.concat([
    discriminator(idl.accounts, "BridgeConfig"),
    program.toBuffer(),
    length,
    chain,
    Buffer.alloc(64),
  ]);
  const pin = decodeBridgePin(bridge);
  assert.ok(pin.program.equals(program));
  assert.equal(pin.chainId, "akashnet-2");
  const wrong = Buffer.from(bridge);
  wrong[0] ^= 1;
  assert.throws(() => decodeBridgePin(wrong), /not a bridge account/);
  const long = Buffer.from(bridge);
  long.writeUInt32LE(65, 40);
  assert.throws(() => decodeBridgePin(long), /chain ID/);
  const app = Buffer.concat([
    discriminator(clientIdl.accounts, "AppState"),
    manager.toBuffer(),
    Buffer.alloc(257),
  ]);
  assert.ok(decodeAppStateAccessManager(app).equals(manager));
  assert.throws(() => decodeAppStateAccessManager(bridge));

  const found = { program, chainId: "akashnet-2" };
  assert.equal(lightClientProblem(found, { chainId: "akashnet-2" }), undefined);
  assert.equal(
    lightClientProblem(found, { chainId: "akashnet-2", pin: program }),
    undefined,
  );
  assert.equal(
    lightClientProblem(found, {
      chainId: "akashnet-2",
      pin: web3.Keypair.generate().publicKey,
    })?.event,
    "light-client-mismatch",
  );
  assert.equal(
    lightClientProblem(found, { chainId: "akash-1", pin: program })?.event,
    "bridge-chain-mismatch",
  );
});

function transferState(id: bigint) {
  const record = Buffer.alloc(77);
  record[0] = 1;
  record.writeBigUInt64LE(id, 1);
  web3.Keypair.generate().publicKey.toBuffer().copy(record, 29);
  record.writeBigUInt64LE(1_000_000n, 61);
  record.writeBigUInt64LE(10n + id, 69);
  return TransferState.fromPartial({ record, voidOrdered: false });
}
function openPage(ids: bigint[], next: bigint) {
  const writer = new BinaryWriter();
  for (const id of ids)
    writer
      .tag(1, WireType.LengthDelimited)
      .bytes(TransferState.encode(transferState(id)).finish());
  if (next) writer.tag(2, WireType.Varint).uint64(next);
  // Unknown fields from a newer node are skipped.
  writer.tag(9, WireType.Varint).uint32(1);
  return writer.finish();
}

test("OpenTransfers request and response follow the x/sink wire format", () => {
  assert.deepEqual(encodeOpenTransfersRequest(0n, 0), new Uint8Array());
  assert.deepEqual(
    encodeOpenTransfersRequest(300n, 100),
    Uint8Array.from([0x08, 0xac, 0x02, 0x10, 0x64]),
  );
  const decoded = decodeOpenTransfersResponse(openPage([4n, 9n], 9n));
  assert.equal(decoded.nextStartAfter, 9n);
  assert.equal(decoded.transfers.length, 2);
  assert.equal(Buffer.from(decoded.transfers[1].record).readBigUInt64LE(1), 9n);
  assert.equal(
    decodeOpenTransfersResponse(openPage([], 0n)).nextStartAfter,
    0n,
  );
  // uint64 values above 2^63 - 1 stay unsigned both ways.
  const high = 18_446_744_073_709_551_614n;
  assert.equal(
    QueryOpenTransfersRequest.decode(
      encodeOpenTransfersRequest(high, 500),
    ).startAfter.toString(),
    high.toString(),
  );
  assert.equal(
    decodeOpenTransfersResponse(openPage([], high)).nextStartAfter,
    high,
  );
});

test("OpenTransfers pages at one height and falls back on old nodes", async () => {
  const replies: Record<string, Uint8Array> = {
    "0": openPage([2n, 5n], 5n),
    "5": openPage([7n], 0n),
  };
  const seen: { startAfter: string; height?: number }[] = [];
  const node = {
    async abciQuery(params: {
      path: string;
      data: Uint8Array;
      height?: number;
    }) {
      assert.equal(params.path, OPEN_TRANSFERS_PATH);
      const startAfter =
        params.data.length && params.data[0] === 0x08
          ? String(params.data[1])
          : "0";
      seen.push({ startAfter, height: params.height });
      return { code: 0, height: params.height, value: replies[startAfter] };
    },
  };
  const open = await openTransfers(node, 40, 2);
  assert.deepEqual(
    open?.map((entry) => entry.id),
    [2n, 5n, 7n],
  );
  assert.deepEqual(seen, [
    { startAfter: "0", height: 40 },
    { startAfter: "5", height: 40 },
  ]);
  const old = {
    async abciQuery() {
      return { code: 6, codespace: "sdk", value: new Uint8Array() };
    },
  };
  assert.equal(await openTransfers(old, 40), undefined);
  const failing = {
    async abciQuery() {
      return { code: 18, codespace: "sdk", value: new Uint8Array() };
    },
  };
  await assert.rejects(
    () => openTransfers(failing, 40),
    /OpenTransfers failed/,
  );
  const lagging = {
    async abciQuery() {
      return { code: 0, height: 39, value: openPage([1n], 0n) };
    },
  };
  await assert.rejects(() => openTransfers(lagging, 40), /height mismatch/);
  const unordered = {
    async abciQuery() {
      return { code: 0, height: 40, value: openPage([5n, 2n], 0n) };
    },
  };
  await assert.rejects(() => openTransfers(unordered, 40), /out of ID order/);
  const stuck = {
    async abciQuery() {
      return { code: 0, height: 40, value: openPage([], 3n) };
    },
  };
  await assert.rejects(() => openTransfers(stuck, 40), /did not advance/);
});

const address = (i: number) => i.toString(16).toUpperCase().padStart(40, "0");
const members = (powers: number[], offset = 0): SetMember[] =>
  powers.map((power, i) => ({
    address: address(i + offset),
    voting_power: String(power),
  }));
const commit = (count: number, signed: (i: number) => boolean) =>
  Array.from({ length: count }, (_, i): CommitSignature =>
    signed(i)
      ? {
          block_id_flag: 2,
          validator_address: address(i),
          timestamp: "2026-10-19T00:00:00Z",
          signature: "c2ln",
        }
      : {
          block_id_flag: 1,
          validator_address: "",
          timestamp: "0001-01-01T00:00:00Z",
          signature: null,
        },
  );
const kept = (result: ReturnType<typeof selectCommitSignatures>) => {
  assert.ok(!("unsupported" in result), JSON.stringify(result));
  return result.signatures.flatMap((sig, i) =>
    sig.block_id_flag === 2 ? [i] : [],
  );
};

test("commits within the signature limit are submitted unchanged", () => {
  const signatures = commit(100, () => true);
  const result = selectCommitSignatures({
    signatures,
    validators: members(Array(100).fill(10_000)),
    maxSignatures: 100,
    limit: 128,
  });
  assert.ok(!("unsupported" in result));
  assert.equal(result.signatures, signatures);
  assert.equal(result.kept, 100);
});

test("large equal commits keep a quorum plus margin and mark the rest absent", () => {
  const signatures = commit(150, () => true);
  signatures[149] = { ...signatures[149], block_id_flag: 3 }; // nil vote
  const result = selectCommitSignatures({
    signatures,
    validators: members(Array(150).fill(10_000)),
    maxSignatures: 100,
    limit: 128,
  });
  // More than 2/3 of 150 is 101; 1% more of the power is 102 signatures.
  assert.deepEqual(kept(result), [...Array(102).keys()]);
  assert.ok(!("unsupported" in result));
  assert.equal(result.present, 149);
  assert.equal(result.signatures.length, 150);
  assert.deepEqual(result.signatures[120], {
    block_id_flag: 1,
    validator_address: "",
    timestamp: "0001-01-01T00:00:00Z",
    signature: null,
  });
  // Exactly the strict quorum when the margin is out of reach.
  const quorum = selectCommitSignatures({
    signatures: commit(150, (i) => i < 101),
    validators: members(Array(150).fill(10_000)),
    maxSignatures: 100,
    limit: 128,
  });
  assert.equal(kept(quorum).length, 101);
});

test("weighted commits keep the highest-power signers first", () => {
  // 30 validators at 100,000 and 90 at 1,000, listed small first.
  const powers = [...Array(90).fill(1_000), ...Array(30).fill(100_000)];
  const result = selectCommitSignatures({
    signatures: commit(120, () => true),
    validators: members(powers),
    maxSignatures: 100,
    limit: 128,
  });
  // 21 of the large validators hold 2.1M of 3.09M, above 2/3 plus 1%.
  assert.deepEqual(
    kept(result),
    Array.from({ length: 21 }, (_, i) => 90 + i),
  );
});

test("skipping updates also keep enough trusted power", () => {
  const validators = members(Array(150).fill(10_000));
  // The trusted set shares only validators 100 to 149 and adds 50 others.
  const trusted = {
    validators: [
      ...members(Array(50).fill(10_000), 100),
      ...members(Array(50).fill(10_000), 1_000),
    ],
    numerator: 1n,
    denominator: 3n,
  };
  const result = selectCommitSignatures({
    signatures: commit(150, () => true),
    validators,
    trusted,
    maxSignatures: 100,
    limit: 128,
  });
  // More than 1/3 of 100 trusted validators, plus 1%: 35 shared signers,
  // which also count toward the 102 the new set needs.
  assert.deepEqual(kept(result), [
    ...Array.from({ length: 67 }, (_, i) => i),
    ...Array.from({ length: 35 }, (_, i) => 100 + i),
  ]);
  // Adjacent updates ignore the trusted set.
  assert.equal(
    kept(
      selectCommitSignatures({
        signatures: commit(150, () => true),
        validators,
        maxSignatures: 100,
        limit: 128,
      }),
    ).length,
    102,
  );
});

test("commits no subset can verify are unsupported", () => {
  const validators = members(Array(160).fill(10_000));
  // 106 of 160 is not more than 2/3.
  const short = selectCommitSignatures({
    signatures: commit(160, (i) => i < 106),
    validators,
    maxSignatures: 100,
    limit: 128,
  });
  assert.match(
    "unsupported" in short ? short.unsupported : "",
    /1060000 of 1600000 voting power/,
  );
  // No shared signer with the trusted set.
  const disjoint = selectCommitSignatures({
    signatures: commit(160, () => true),
    validators,
    trusted: {
      validators: members(Array(10).fill(10_000), 5_000),
      numerator: 1n,
      denominator: 3n,
    },
    maxSignatures: 100,
    limit: 128,
  });
  assert.match(
    "unsupported" in disjoint ? disjoint.unsupported : "",
    /0 of 100000 trusted voting power/,
  );
  // 200 equal validators need 134 signatures, more than one update holds.
  const wide = selectCommitSignatures({
    signatures: commit(200, () => true),
    validators: members(Array(200).fill(10_000)),
    maxSignatures: 100,
    limit: 128,
  });
  assert.match(
    "unsupported" in wide ? wide.unsupported : "",
    /needs 134 of 200 signatures; at most 128/,
  );
});

test("assembly out of compute or heap is unsupported, not retried", () => {
  const program = web3.Keypair.generate().publicKey;
  const id = program.toBase58();
  const failed = (line: string) => ({
    error: { InstructionError: [2, "ProgramFailedToComplete"] },
    logs: [`Program ${id} consumed 1399644 of 1399700 compute units`, line],
  });
  assert.equal(
    assemblyExhaustion(
      failed(`Program ${id} failed: exceeded CUs meter at BPF instruction`),
      program,
    ),
    "assembly exhausted the compute budget",
  );
  assert.equal(
    assemblyExhaustion(
      failed(
        `Program ${id} failed: Access violation in heap section at address 0x300043c90 of size 64`,
      ),
      program,
    ),
    "assembly exhausted the heap",
  );
  // Rejected headers and other programs' failures stay ordinary errors.
  assert.equal(
    assemblyExhaustion(
      failed(`Program ${id} failed: custom program error: 0x177a`),
      program,
    ),
    undefined,
  );
  const other = web3.Keypair.generate().publicKey.toBase58();
  assert.equal(
    assemblyExhaustion(
      failed(`Program ${other} failed: exceeded CUs meter at BPF instruction`),
      program,
    ),
    undefined,
  );
  assert.equal(
    assemblyExhaustion({ error: null, logs: [] }, program),
    undefined,
  );
});

test("the header signature limit is configurable within one batch", () => {
  assert.equal(relaySettings({}).maxHeaderSignatures, 100);
  assert.equal(
    relaySettings({ AKT_RELAY_MAX_HEADER_SIGNATURES: "120" })
      .maxHeaderSignatures,
    120,
  );
  for (const value of ["0", "129", "1.5", "x"])
    assert.throws(() =>
      relaySettings({ AKT_RELAY_MAX_HEADER_SIGNATURES: value }),
    );
});
