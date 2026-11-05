import { CommitmentProof } from "cosmjs-types/cosmos/ics23/v1/proofs";
import { MerkleProof } from "cosmjs-types/ibc/core/commitment/v1/commitment";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { toBech32 } from "@cosmjs/encoding";
import { BN, web3 } from "@coral-xyz/anchor";
import {
  type TransferState,
  RecordData,
  SettlementData,
  Sink_Status as Status,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import {
  idSuffix,
  MAX_UINT64,
  MAX_INT64,
  OWNER_BECH32_LIMIT,
  ownerBytes,
  parseSinkId,
  sinkIdParts,
  type SinkId,
} from "./id";
export const TRANSFER_KEY_PREFIX = Buffer.from([0x02, 0x00]);
export const SETTLEMENT_KEY_PREFIX = Buffer.from([0x03, 0x01]);
export const MAX_PROOF_SIZE = 16 * 1024;
export const PROOF_HEADER_SIZE = 80;
export function transferKey(id: SinkId): Uint8Array {
  return Buffer.concat([TRANSFER_KEY_PREFIX, idSuffix(id)]);
}
export function settlementKey(id: SinkId, status: Status): Uint8Array {
  assert.ok(
    Number.isInteger(status) &&
      status >= Status.STATUS_TRANSFER_PENDING &&
      status <= Status.STATUS_REFUNDED,
    "invalid settlement status",
  );
  return Buffer.concat([
    SETTLEMENT_KEY_PREFIX,
    Buffer.from([status]),
    idSuffix(id),
  ]);
}
export function proofNonce(key: Uint8Array): Buffer {
  return createHash("sha256").update(key).digest();
}
export function anchorId(id: SinkId) {
  const value = sinkIdParts(id);
  return {
    owner: Buffer.from(ownerBytes(value.owner)),
    height: new BN(value.height.toString()),
    sequence: new BN(value.sequence.toString()),
  };
}
export function statusAddress(program: web3.PublicKey, id: SinkId) {
  const value = anchorId(id);
  return web3.PublicKey.findProgramAddressSync(
    [
      Buffer.from("transfer"),
      proofNonce(value.owner),
      value.height.toArrayLike(Buffer, "le", 8),
      value.sequence.toArrayLike(Buffer, "le", 8),
    ],
    program,
  )[0];
}
export function proofAddress(
  program: web3.PublicKey,
  payer: web3.PublicKey,
  key: Uint8Array,
) {
  return web3.PublicKey.findProgramAddressSync(
    [Buffer.from("proof"), payer.toBuffer(), proofNonce(key)],
    program,
  )[0];
}
export function coinAmount(
  coin: RecordData["amount"],
  positive = false,
): bigint {
  assert.ok(
    coin && coin.denom === "uakt" && /^(0|[1-9][0-9]*)$/.test(coin.amount),
    "invalid uakt Coin",
  );
  const amount = BigInt(coin.amount);
  assert.ok(!positive || amount > 0n, "amount must be positive");
  return amount;
}
export function recordData(data: RecordData | undefined) {
  assert.ok(data, "missing record data");
  const recipient = new web3.PublicKey(data.dest),
    amount = coinAmount(data.amount, true);
  assert.equal(recipient.toBase58(), data.dest, "noncanonical recipient");
  assert.ok(amount <= MAX_UINT64, "transfer amount exceeds uint64");
  return { recipient, amount };
}
export function decodeRecord(transfer: TransferState) {
  const id = parseSinkId(transfer.id);
  return { id, height: sinkIdParts(id).height, ...recordData(transfer.data) };
}
export function encodeRecord(data: RecordData | undefined): Buffer {
  recordData(data);
  assert.ok(data);
  return Buffer.from(RecordData.encode(data).finish());
}
export function decodeRecordData(raw: Uint8Array) {
  const data = RecordData.decode(raw);
  assert.deepEqual(
    Buffer.from(raw),
    encodeRecord(data),
    "noncanonical RecordData",
  );
  return recordData(data);
}

export function authorizedSettlement(
  transfer: TransferState,
  snapshotHeight: bigint,
) {
  const id = parseSinkId(transfer.id);
  const settlement = transfer.settlement;
  const authorization = settlement?.voidAuthorization;
  assert.ok(
    settlement && authorization,
    "missing permanent void authorization",
  );
  assert.ok(
    snapshotHeight > 0n && snapshotHeight <= MAX_INT64,
    "invalid snapshot height",
  );
  assert.ok(
    authorization.requestedHeight >= sinkIdParts(id).height &&
      authorization.requestedHeight <= snapshotHeight &&
      authorization.requestedRound > 0n &&
      authorization.requestedRound <= settlement.round,
    "invalid void authorization height or round",
  );
  return {
    key: settlementKey(id, transfer.status),
    value: Buffer.from(SettlementData.encode(settlement).finish()),
    authorization,
  };
}

export function settlementProofValue(
  transfer: TransferState,
  snapshotHeight: bigint,
  value: Uint8Array,
) {
  const expected = authorizedSettlement(transfer, snapshotHeight);
  const decoded = SettlementData.decode(value);
  assert.deepEqual(
    Buffer.from(SettlementData.encode(decoded).finish()),
    Buffer.from(value),
    "noncanonical SettlementData",
  );
  assert.deepEqual(
    Buffer.from(value),
    expected.value,
    "settlement proof differs from snapshot",
  );
  return Buffer.from(value);
}
export function matchesAnchorId(
  value: ReturnType<typeof anchorId>,
  id: SinkId,
): boolean {
  const expected = anchorId(id);
  return (
    Buffer.from(value.owner).equals(Buffer.from(expected.owner)) &&
    value.height.eq(expected.height) &&
    value.sequence.eq(expected.sequence)
  );
}

export function idFromTransferKey(key: Uint8Array): SinkId {
  const bytes = Buffer.from(key);
  assert.ok(
    bytes.subarray(0, TRANSFER_KEY_PREFIX.length).equals(TRANSFER_KEY_PREFIX),
    "invalid transfer key",
  );
  let offset = TRANSFER_KEY_PREFIX.length;
  const length = bytes[offset++];
  assert.ok(
    length > 0 && bytes.length === offset + length + 8 + 8,
    "invalid transfer key length",
  );
  const owner = bytes.subarray(offset, offset + length);
  offset += length;
  const height = bytes.readBigUInt64BE(offset);
  offset += 8;
  const sequence = bytes.readBigUInt64BE(offset);
  return parseSinkId({
    owner: toBech32("akash", owner, OWNER_BECH32_LIMIT),
    height,
    sequence,
  });
}

export function encodeMembershipProof(input: {
  key: Uint8Array;
  value: Uint8Array;
  ops: readonly { type: string; key: Uint8Array; data: Uint8Array }[];
}): Buffer {
  assert.equal(input.ops.length, 2, "membership proof needs both store levels");
  assert.equal(input.ops[0].type, "ics23:iavl", "unexpected IAVL proof type");
  assert.equal(
    input.ops[1].type,
    "ics23:simple",
    "unexpected multistore proof type",
  );
  const proofs = input.ops.map((op) => CommitmentProof.decode(op.data));
  const [iavl, store] = proofs;
  assert.ok(
    iavl.exist && store.exist,
    "membership proof must contain two existence proofs",
  );
  assert.ok(
    proofs.every(
      (proof) => !proof.nonexist && !proof.batch && !proof.compressed,
    ),
    "ambiguous membership proof",
  );
  assert.deepEqual(
    Buffer.from(iavl.exist.key),
    Buffer.from(input.key),
    "proof key mismatch",
  );
  assert.deepEqual(
    Buffer.from(iavl.exist.value),
    Buffer.from(input.value),
    "proof value mismatch",
  );
  assert.deepEqual(
    Buffer.from(store.exist.key),
    Buffer.from("sink"),
    "wrong proof store",
  );
  const bytes = Buffer.from(MerkleProof.encode({ proofs }).finish());
  assert.ok(
    bytes.length > 0 && bytes.length <= MAX_PROOF_SIZE,
    "proof exceeds upload bound",
  );
  return bytes;
}
