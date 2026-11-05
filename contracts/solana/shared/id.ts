import assert from "node:assert/strict";
import { fromBech32, toBech32 } from "@cosmjs/encoding";
import { z } from "zod";
import type { Sink_ID } from "@akashnetwork/chain-sdk/private-types/akash.v1";

export const MAX_UINT64 = (1n << 64n) - 1n;
export const MAX_INT64 = (1n << 63n) - 1n;
export const MAX_OWNER_BYTES = 255;
export const OWNER_BECH32_LIMIT =
  5 + 1 + Math.ceil((MAX_OWNER_BYTES * 8) / 5) + 6;
export type SinkId = string & { readonly __brand: "SinkId" };

export function ownerBytes(owner: string): Uint8Array {
  const { prefix, data } = fromBech32(owner, OWNER_BECH32_LIMIT);
  assert.ok(
    prefix === "akash" && data.length > 0 && data.length <= MAX_OWNER_BYTES,
    "owner must be an Akash account of 1 to 255 bytes",
  );
  assert.equal(
    toBech32(prefix, data, OWNER_BECH32_LIMIT),
    owner,
    "noncanonical owner",
  );
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
      : ownerBytes(x.owner).length - ownerBytes(y.owner).length ||
        Buffer.compare(ownerBytes(x.owner), ownerBytes(y.owner));
}
export function idSuffix(id: SinkId): Buffer {
  const value = sinkIdParts(id),
    owner = ownerBytes(value.owner),
    height = Buffer.alloc(8),
    sequence = Buffer.alloc(8);
  height.writeBigUInt64BE(value.height);
  sequence.writeBigUInt64BE(value.sequence);
  return Buffer.concat([Buffer.from([owner.length]), owner, height, sequence]);
}
