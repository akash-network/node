import assert from "node:assert/strict";
import { fromBech32, toBech32 } from "@cosmjs/encoding";
import { z } from "zod";
import type { Sink_ID } from "@akashnetwork/chain-sdk/private-types/akash.v1";

export const MAX_UINT64 = (1n << 64n) - 1n;
export const MAX_INT64 = (1n << 63n) - 1n;
export type SinkId = string & { readonly __brand: "SinkId" };

export function ownerBytes(owner: string): Uint8Array {
  const { prefix, data } = fromBech32(owner, 90);
  assert.ok(
    prefix === "akash" && data.length === 20 && data.some((byte) => byte !== 0),
    "owner must be a nonzero Akash account of 20 bytes",
  );
  assert.equal(toBech32(prefix, data), owner, "noncanonical owner");
  return data;
}
const component = z
  .union([z.bigint(), z.string().regex(/^[1-9][0-9]*$/)])
  .transform((value) => BigInt(value));
const identity = z
  .object({ owner: z.string(), height: component, sequence: component })
  .strict();

function isSinkId(value: unknown): value is SinkId {
  if (typeof value !== "string") return false;
  const parts = value.split("/");
  if (
    parts.length !== 3 ||
    !/^[1-9][0-9]*$/.test(parts[1]) ||
    !/^[1-9][0-9]*$/.test(parts[2])
  )
    return false;
  if (BigInt(parts[1]) > MAX_INT64 || BigInt(parts[2]) > MAX_UINT64)
    return false;
  try {
    ownerBytes(parts[0]);
    return true;
  } catch {
    return false;
  }
}
export function parseSinkId(value: unknown): SinkId {
  if (isSinkId(value)) return value;
  if (typeof value === "object" && value !== null) {
    const parsed = identity.parse(value);
    const text = `${parsed.owner}/${parsed.height}/${parsed.sequence}`;
    if (isSinkId(text)) return text;
  }
  throw new Error(
    "ID must be canonical owner/positive-int64-height/positive-uint64-sequence",
  );
}
export function sinkIdParts(id: SinkId): Sink_ID {
  const [owner, height, sequence] = id.split("/");
  return { owner, height: BigInt(height), sequence: BigInt(sequence) };
}
export function compareIds(a: SinkId, b: SinkId): number {
  const x = sinkIdParts(a),
    y = sinkIdParts(b);
  return x.height !== y.height
    ? x.height < y.height
      ? -1
      : 1
    : x.sequence !== y.sequence
      ? x.sequence < y.sequence
        ? -1
        : 1
      : Buffer.compare(ownerBytes(x.owner), ownerBytes(y.owner));
}
export function idSuffix(id: SinkId): Buffer {
  const value = sinkIdParts(id),
    bytes = Buffer.alloc(37);
  bytes[0] = 20;
  bytes.set(ownerBytes(value.owner), 1);
  bytes.writeBigUInt64BE(value.height, 21);
  bytes.writeBigUInt64BE(value.sequence, 29);
  return bytes;
}
// Native chronological index key. PageRequest.key is inclusive, so resumption
// uses the last existing key and drops that one row, even after an empty next_key.
export function chronologicalKey(id: SinkId): Uint8Array {
  const value = sinkIdParts(id),
    key = Buffer.alloc(37);
  key.writeBigUInt64BE(value.height, 0);
  key.writeBigUInt64BE(value.sequence, 8);
  key[16] = 20;
  key.set(ownerBytes(value.owner), 17);
  return key;
}
