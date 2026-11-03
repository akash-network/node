import assert from "node:assert/strict";
import {
  Sink_Status as Status,
  type TransferState,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import type { AkashClient } from "../reporter/akash";
import {
  parseSinkId,
  type SinkId,
  chronologicalKey,
  compareIds,
} from "../shared/id";
export type TransferEntry = { id: SinkId; transfer: TransferState };
export type Page = { transfers: TransferState[]; nextKey: Uint8Array };
export async function collectPages(
  read: (key: Uint8Array) => Promise<Page>,
): Promise<TransferEntry[]> {
  const result: TransferEntry[] = [],
    seen = new Set<string>();
  let key: Uint8Array = new Uint8Array();
  for (;;) {
    const page = await read(key);
    for (const transfer of page.transfers) {
      const id = parseSinkId(transfer.id);
      assert.ok(!seen.has(id), "duplicate transfer in paginated query");
      seen.add(id);
      result.push({ id, transfer });
    }
    if (page.nextKey.length === 0) return result;
    assert.ok(
      page.transfers.length > 0 && Buffer.compare(key, page.nextKey) < 0,
      "pagination cursor did not advance",
    );
    key = page.nextKey;
  }
}
// Disputes first, then refund candidates: their veto and payment deadlines
// are the most urgent work. A held transfer keeps its dispute position.
export async function openTransfers(
  akash: Pick<AkashClient, "transfers" | "disputes">,
  height: number,
) {
  const entries = new Map<SinkId, TransferEntry>();
  for (const entry of await collectPages((key) =>
    akash.disputes(height, key, 500),
  ))
    entries.set(entry.id, entry);
  for (const status of [
    Status.STATUS_REFUND_PENDING,
    Status.STATUS_VOIDED,
    Status.STATUS_TRANSFER_PENDING,
  ]) {
    for (const entry of await collectPages((key) =>
      akash.transfers(height, { key, status, limit: 500 }),
    ))
      entries.set(entry.id, entry);
  }
  return [...entries.values()];
}
export async function discoverTransfers(
  akash: Pick<AkashClient, "transfers">,
  height: number,
  after?: SinkId,
  limit = 500,
) {
  const page = await akash.transfers(height, {
    key: after ? chronologicalKey(after) : undefined,
    limit,
  });
  let cursor = after;
  const entries: TransferEntry[] = [];
  for (const [index, transfer] of page.transfers.entries()) {
    const id = parseSinkId(transfer.id);
    if (after && index === 0) {
      assert.equal(id, after, "chronological checkpoint missing from query");
      continue;
    }
    assert.ok(
      !cursor || compareIds(id, cursor) > 0,
      "history is not chronological",
    );
    entries.push({ id, transfer });
    cursor = id;
  }
  if (after)
    assert.ok(
      page.transfers.length > 0,
      "chronological checkpoint disappeared",
    );
  return { entries, cursor, hasMore: page.nextKey.length > 0 };
}
