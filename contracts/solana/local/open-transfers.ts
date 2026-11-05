import assert from "node:assert/strict";
import {
  Sink_Status as Status,
  type TransferState,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import type { AkashClient } from "../reporter/akash";
import { parseSinkId, type SinkId, compareIds } from "../shared/id";
export type TransferEntry = { id: SinkId; transfer: TransferState };
export type Page = { transfers: TransferState[]; nextKey: Uint8Array };
export type DiscoveryCursor =
  | { kind: "next"; key: Uint8Array; lastId: SinkId }
  | { kind: "tail"; key: Uint8Array; lastId: SinkId };
export async function collectPages(
  read: (key: Uint8Array) => Promise<Page>,
): Promise<TransferEntry[]> {
  const result: TransferEntry[] = [],
    seen = new Set<string>(),
    cursors = new Set<string>([""]);
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
    const next = Buffer.from(page.nextKey).toString("base64");
    assert.ok(
      page.transfers.length > 0 && !cursors.has(next),
      "pagination cursor did not advance",
    );
    cursors.add(next);
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
  after?: DiscoveryCursor,
  limit = 500,
) {
  const page = await akash.transfers(height, {
    key: after?.key,
    limit,
  });
  assert.ok(
    after?.kind !== "next" || page.transfers.length > 0,
    "chronological checkpoint disappeared",
  );
  let lastId = after?.lastId;
  let previous: SinkId | undefined;
  let checkpointFound = after?.kind !== "tail";
  const entries: TransferEntry[] = [];
  for (const transfer of page.transfers) {
    const id = parseSinkId(transfer.id);
    assert.ok(
      !previous || compareIds(id, previous) > 0,
      "history is not chronological",
    );
    previous = id;
    if (after?.kind === "tail" && compareIds(id, after.lastId) <= 0) {
      if (id === after.lastId) checkpointFound = true;
      continue;
    }
    assert.ok(checkpointFound, "chronological checkpoint disappeared");
    assert.ok(
      !lastId || compareIds(id, lastId) > 0,
      "history is not chronological",
    );
    entries.push({ id, transfer });
    lastId = id;
  }
  assert.ok(checkpointFound, "chronological checkpoint disappeared");
  const hasMore = page.nextKey.length > 0;
  let cursor: DiscoveryCursor | undefined;
  if (hasMore) {
    assert.ok(
      lastId &&
        page.transfers.length > 0 &&
        !Buffer.from(page.nextKey).equals(after?.key ?? new Uint8Array()),
      "pagination cursor did not advance",
    );
    cursor = { kind: "next", key: page.nextKey, lastId };
  } else if (lastId) {
    // Replay only the last page on the next poll. The server owns its cursor;
    // the last ID distinguishes already-discovered rows from later deposits.
    cursor = { kind: "tail", key: after?.key ?? new Uint8Array(), lastId };
  }
  return { entries, cursor, hasMore };
}
